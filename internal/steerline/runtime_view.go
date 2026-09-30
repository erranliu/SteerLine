package steerline

import (
	"encoding/json"
	"errors"
)

func runtimeCheckpoint(v Runtime) ServiceCheckpoint {
	var reserved *string
	if unclaimedReservation(&v) {
		reserved = v.CurrentTask
	}
	return ServiceCheckpoint{SchemaVersion: WorkerFlowRuntimeSchema, Revision: v.Revision,
		UpdatedAt: v.UpdatedAt, HeartbeatAt: v.HeartbeatAt, Service: v.Service,
		Repository: v.Repository, Operation: v.Control.Operation, Blockers: v.Control.Blockers, ReservedTask: reserved}
}

func runtimeFromCheckpoint(c ServiceCheckpoint) Runtime {
	return Runtime{SchemaVersion: c.SchemaVersion, Revision: c.Revision, UpdatedAt: c.UpdatedAt,
		HeartbeatAt: c.HeartbeatAt, Service: c.Service, Repository: c.Repository,
		CurrentTask: c.ReservedTask, SlotOccupied: c.ReservedTask != nil,
		Control: ControlState{Operation: c.Operation, Blockers: c.Blockers}}
}

// projectRuntime is the sole constructor of client Run views. It never writes
// domain records or uses a previously persisted view to fill missing fields.
func projectRuntime(v *Runtime) error {
	previous := make(map[string]*Run, len(v.Runs))
	for _, run := range v.Runs {
		if run != nil {
			previous[run.RunID] = run
		}
	}
	runs := make([]*Run, 0, len(v.WorkflowRuns))
	for _, task := range v.Tasks {
		workflow := latestNativeWorkflow(v.WorkflowRuns, task.TaskID)
		if workflow == nil {
			continue
		}
		if task.Status == "discarded" && workflow.Phase == "abandoned" {
			continue
		}
		execution := latestWorkflowExecution(v.Executions, workflow.WorkflowRunID)
		if execution == nil {
			return errors.New("workflow has no execution record")
		}
		view, err := projectRun(task, *workflow, *execution, v.Attempts, v.Decisions, v.Deliveries)
		if err != nil {
			return err
		}
		if old := previous[view.RunID]; old != nil {
			*old = view
			runs = append(runs, old)
		} else {
			runs = append(runs, &view)
		}
	}
	v.Runs = runs
	projectSlot(v)
	return nil
}

func projectRun(task TaskRecord, workflow WorkflowRunRecord, execution ExecutionRecord, attempts []StepAttemptRecord, decisions []DecisionRecord, deliveries []TaskDeliveryRecord) (Run, error) {
	if workflow.TaskID != task.TaskID || execution.WorkflowRunID != workflow.WorkflowRunID || workflow.SelectedResult != "" && workflow.SelectedResult != execution.ExecutionID {
		return Run{}, errors.New("task, workflow and selected execution identity differ")
	}
	if workflow.WorkflowVersion == WorkerFlowVersion {
		count := 0
		for _, attempt := range attempts {
			if attempt.ExecutionID == execution.ExecutionID {
				if attempt.StepID != "worker" || execution.CurrentStep != "worker" {
					return Run{}, errors.New("worker flow has an invalid step identity")
				}
				count++
			}
		}
		if count < 1 {
			return Run{}, errors.New("worker flow has no worker attempt identity")
		}
	}
	run := Run{TaskID: task.TaskID, DomainTaskID: task.TaskID, Title: task.Title, TaskStatus: task.Status,
		RecoveryRequired: execution.RecoveryRequired, RecoveryState: execution.RecoveryState, RecoveryVersion: execution.RecoveryVersion,
		RunID: workflow.WorkflowRunID, WorkflowVersion: workflow.WorkflowVersion, WorkflowConfigDigest: workflow.ConfigDigest,
		ControlRevision: workflow.ControlRevision, ExecutionID: execution.ExecutionID, Claimant: task.Claimant,
		Host: hostForExecution(execution.Backend, execution.Protocol), Backend: execution.Backend, Protocol: execution.Protocol,
		Agent: execution.Agent, Target: execution.Target, Process: execution.Process, WorkspaceIntent: execution.WorkspaceIntent,
		StartedAt: copyPtr(execution.TurnStartedAt), EndedAt: copyPtr(execution.TurnEndedAt), LogPath: copyPtr(execution.LogPath),
		LastEvent: copyPtr(execution.LastEvent), Summary: copyPtr(execution.Summary), Error: copyPtr(execution.Error), ManualStop: copyPtr(execution.ManualStop),
		FinishTaskStatus: nativeRuntimeFinishStatus(task, workflow), ExecutionStatus: executionViewStatus(execution),
		PR: PRState{State: nonEmpty(workflow.PRState, "unknown"), QueriedAt: copyPtr(workflow.PRQueriedAt),
			QueryError: copyPtr(workflow.PRQueryError), MergedAt: copyPtr(workflow.PRMergedAt), MergeCommit: copyPtr(workflow.PRMergeCommit)}}
	if attempt := latestCurrentNativeAttempt(attempts, &execution); attempt != nil {
		if workflow.WorkflowVersion == WorkerFlowVersion && execution.CurrentAttemptID != "" && execution.CurrentAttemptID != attempt.AttemptID {
			return Run{}, errors.New("worker flow current attempt pointer differs from latest worker generation")
		}
		run.AttemptID, run.DomainAttemptNumber = attempt.AttemptID, attempt.AttemptNumber
	}
	if execution.SessionID != "" && (execution.Process == nil || execution.Process.Kind != "orca-native") {
		run.SessionID = ptr(execution.SessionID)
	}
	refreshExecutionView(&run, &execution)
	if workflow.PRURL != "" {
		run.PR.URL = ptr(workflow.PRURL)
	}
	if execution.DecisionID != "" {
		for _, record := range decisions {
			if record.DecisionID == execution.DecisionID {
				if record.WorkflowRunID != workflow.WorkflowRunID || record.ExecutionID != execution.ExecutionID || record.AttemptID != run.AttemptID || record.Version == 0 {
					return Run{}, errors.New("current decision differs from its execution or attempt")
				}
				var err error
				run.Decision, err = decisionView(record)
				if err != nil {
					return Run{}, err
				}
				break
			}
		}
		if run.Decision == nil {
			return Run{}, errors.New("execution refers to a missing decision")
		}
	}
	if task.Status == "closed" {
		run.ExecutionStatus = "succeeded"
		for _, delivery := range deliveries {
			if delivery.TaskID == task.TaskID && delivery.SelectedWorkflowRunID == workflow.WorkflowRunID {
				run.PR = PRState{URL: ptr(delivery.PRURL), State: "merged", MergedAt: ptr(delivery.MergedAt), MergeCommit: ptr(delivery.MergeCommit)}
			}
		}
	}
	return run, nil
}

func executionViewStatus(e ExecutionRecord) string {
	switch e.Status {
	case "active":
		return "running"
	case "pending":
		return "pending"
	case "idle", "succeeded", "stopped":
		return nonEmpty(e.LastTurnOutcome, "succeeded")
	case "failed":
		return "failed"
	default:
		return "unknown"
	}
}

func refreshExecutionView(run *Run, e *ExecutionRecord) {
	run.Process, run.Target = copyPtr(e.Process), e.Target
	run.WorkspaceIntent = copyPtr(e.WorkspaceIntent)
	path, branch, base := e.Workspace.Worktree, e.Workspace.Branch, e.Workspace.Base
	if e.WorkspaceIntent != nil {
		if e.Workspace.Worktree == "" {
			branch, base = e.WorkspaceIntent.Branch, e.WorkspaceIntent.BaseCommit
		}
		if path == "" && e.Protocol != string(ProtocolOrcaSupervised) {
			path = e.WorkspaceIntent.Path
		}
		if branch == "" {
			branch = e.WorkspaceIntent.Branch
		}
		if base == "" {
			base = e.WorkspaceIntent.BaseCommit
		}
	}
	run.WorktreePath, run.WorktreeBranch, run.BaseCommit = nil, nil, nil
	if path != "" {
		run.WorktreePath = ptr(path)
	}
	if branch != "" {
		run.WorktreeBranch = ptr(branch)
	}
	if base != "" {
		run.BaseCommit = ptr(base)
	}
	if e.SessionID != "" && (e.Process == nil || e.Process.Kind != "orca-native") {
		run.SessionID = ptr(e.SessionID)
	}
}

func decisionView(record DecisionRecord) (*DecisionState, error) {
	var request DecisionRequest
	if err := json.Unmarshal(record.Request, &request); err != nil {
		return nil, err
	}
	decision := &DecisionState{ID: record.DecisionID, AttemptID: record.AttemptID, Version: record.Version, Request: request, Status: record.Status}
	if request.OrcaAsk != nil && request.OrcaAsk.MessageID != record.QuestionID {
		return nil, errors.New("persisted Orca question differs from its decision binding")
	}
	if len(record.Answer) > 0 {
		var answer DecisionAnswer
		if err := json.Unmarshal(record.Answer, &answer); err != nil {
			return nil, err
		}
		if answer.DecisionID != record.DecisionID || answer.RunID != record.WorkflowRunID || validateDecisionAnswer(&request, &answer) != nil {
			return nil, errors.New("saved answer differs from its fixed question or identity")
		}
		decision.OptionID, decision.Note, decision.Answers, decision.AnsweredBy = answer.OptionID, answer.Note, answer.Answers, answer.AnsweredBy
		decision.SourceAttemptID, decision.RecoveryVersion = answer.SourceAttemptID, answer.ExpectedRecoveryVersion
		if !answer.AnsweredAt.IsZero() {
			decision.AnsweredAt = &answer.AnsweredAt
		}
	}
	return decision, nil
}

func projectSlot(v *Runtime) {
	occupied := v.Control.Operation != nil || len(v.Control.Blockers) != 0
	var tasks []string
	for _, run := range v.Runs {
		if run.ExecutionStatus == "unknown" || run.ExecutionStatus == "pending" || run.ExecutionStatus == "running" || run.FinishTaskStatus == "waiting_decision" {
			occupied = true
			tasks = append(tasks, run.TaskID)
		}
	}
	if unclaimedReservation(v) {
		occupied = true
	} else {
		v.CurrentTask = nil
		if len(tasks) == 1 {
			v.CurrentTask = ptr(tasks[0])
		}
	}
	v.SlotOccupied = occupied
}

func unclaimedReservation(v *Runtime) bool {
	return v.SlotOccupied && v.CurrentTask != nil && latestNativeWorkflow(v.WorkflowRuns, *v.CurrentTask) == nil
}
func optionalString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
func copyPtr[T any](value *T) *T {
	if value == nil {
		return nil
	}
	return ptr(*value)
}
func (s *Service) workflowRecord(run *Run) *WorkflowRunRecord {
	workflow, _ := s.workflowRecords(run)
	return workflow
}

func (s *Service) refreshRun(run *Run) {
	if run == nil {
		return
	}
	workflow, execution := s.workflowRecords(run)
	task := nativeTaskRecord(s.State.Tasks, run.TaskID)
	if workflow == nil || execution == nil || task == nil {
		return
	}
	view, err := projectRun(*task, *workflow, *execution, s.State.Attempts, s.State.Decisions, s.State.Deliveries)
	if err != nil {
		s.setError(err)
		return
	}
	*run = view
}

func (s *Service) setRunDecision(run *Run, decision *DecisionState) {
	execution := s.executionRecord(run)
	if execution == nil {
		return
	}
	if decision == nil {
		execution.DecisionID = ""
		s.refreshRun(run)
		return
	}
	record := DecisionRecord{ProjectIdentity: s.Config.identity, DecisionID: decision.ID, Scope: "execution",
		WorkflowRunID: run.RunID, ExecutionID: run.ExecutionID, AttemptID: decision.AttemptID, QuestionID: decision.ID,
		Request: mustJSON(decision.Request), Status: decision.Status, Version: decision.Version, CreatedAt: now(), UpdatedAt: now()}
	if decision.Request.OrcaAsk != nil {
		record.QuestionID = decision.Request.OrcaAsk.MessageID
	}
	for i := range s.State.Decisions {
		if s.State.Decisions[i].DecisionID == record.DecisionID {
			record.CreatedAt = s.State.Decisions[i].CreatedAt
			s.State.Decisions[i] = record
			execution.DecisionID = record.DecisionID
			s.refreshRun(run)
			return
		}
	}
	s.State.Decisions = append(s.State.Decisions, record)
	execution.DecisionID = record.DecisionID
	s.refreshRun(run)
}

func (s *Service) decisionRecord(run *Run) *DecisionRecord {
	if run == nil || run.Decision == nil {
		return nil
	}
	for i := range s.State.Decisions {
		if s.State.Decisions[i].DecisionID == run.Decision.ID {
			return &s.State.Decisions[i]
		}
	}
	return nil
}
func (s *Service) setDecisionStatus(run *Run, status string) {
	record := s.decisionRecord(run)
	record.Status, record.Version, record.UpdatedAt = status, record.Version+1, now()
	s.refreshRun(run)
}
