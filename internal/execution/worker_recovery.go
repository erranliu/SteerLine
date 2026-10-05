package execution

import (
	"context"
	"encoding/json"
	"errors"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
	"os"
	"strings"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

// Operations are bound by their saved request and exact identity components,
// including host publication which is keyed by WorkflowRun rather than Execution.
func (s *Service) hasUnknownWorkerOperation(run *reconstructiontypes.RunView) bool {
	for _, op := range s.domain.Operations {
		if op.Kind == "worker_input_value" {
			continue
		} // This journal has no external outcome to reconcile.
		if op.Status != "outcome_unknown" && op.Status != "in_flight" {
			continue
		}
		var target struct {
			ExecutionID   string `json:"execution_id"`
			WorkflowRunID string `json:"workflow_run_id"`
			RunID         string `json:"run_id"`
		}
		if json.Unmarshal(op.Request, &target) == nil && (target.ExecutionID == run.ExecutionID || target.WorkflowRunID == run.RunID || target.RunID == run.RunID) {
			return true
		}
		for _, identity := range strings.Split(op.IdempotencyKey, ":") {
			if identity == run.ExecutionID || identity == run.RunID {
				return true
			}
		}
	}
	return false
}

// beginLocalRecoveryAttempt advances the worker generation in one service
// write. The prior process and session stay on the retired Attempt; its
// process receipt and execution binding stay at their original paths.
func (s *Service) beginLocalRecoveryAttempt(ctx context.Context, run *reconstructiontypes.RunView, mode string) error {
	if mode == "" {
		mode = "continue"
	}
	if mode != "continue" && mode != "new_session" {
		return errors.New("choose the original session or explicitly choose a new session")
	}
	lifecycle, fixed, err := s.lifecycleForRun(run)
	if err != nil {
		return err
	}
	workflow, execution, old := fixed.workflow, fixed.execution, fixed.attempt
	if !execution.RecoveryRequired ||
		(execution.RecoveryState != "interrupted" && execution.RecoveryState != "completed") ||
		execution.Process == nil || s.Config.ProcessHost.InspectProcess(execution.Process) != "process absent" {
		return errors.New("the original worker process and its descendants are not confirmed stopped")
	}
	if execution.Workspace.Worktree == "" || run.WorktreePath == nil || canonical(*run.WorktreePath) != canonical(execution.Workspace.Worktree) {
		return errors.New("the original worktree identity is not verified")
	}
	workspace, err := s.inspectWorkflowWorkspace(ctx, run)
	if err != nil || canonical(workspace.Worktree) != canonical(execution.Workspace.Worktree) || workspace.Branch != execution.Workspace.Branch {
		return errors.New("the original worktree and branch could not be reverified")
	}
	applyWorkflowWorkspaceEvidence(run, execution, workspace)
	previous, err := s.lifecycleSnapshot(run, fixed.workflow, execution, old, nil)
	if err != nil {
		return err
	}
	recovery, err := lifecycle.VerifyManualRecovery(ctx, previous)
	if err != nil {
		return err
	}
	if !recovery.ManagedProcess || !recovery.ProcessAbsent {
		return errors.New("the original process tree is not proven to be service managed")
	}
	if mode == "continue" && (!recovery.SessionAvailable || execution.SessionID == "") {
		return errors.New("the original session identity is unavailable; choose a new session")
	}
	if owned := s.ownedProcesses[run.RunID]; owned != nil {
		if owned.Identity() != *execution.Process {
			return errors.New("owned process identity changed before recovery")
		}
		owned.Close()
		delete(s.ownedProcesses, run.RunID)
	}
	if !s.Config.Resources.WorkerTreeAbsent(*execution.Process) {
		return errors.New("original worker descendants may still write to the worktree")
	}
	if s.hasUnknownWorkerOperation(run) {
		return errors.New("an earlier external operation has an unknown outcome; reconcile it before continuing")
	}
	originalSession := execution.SessionID
	old.WorkerProcess = copyPtr(execution.Process)
	old.Status, old.UpdatedAt = "stopped", now()
	if run.Decision != nil {
		s.setDecisionStatus(run, "applied")
		s.setRunDecision(run, nil)
	}
	created := now()
	newAttemptID := newID()
	s.domain.Attempts = append(s.domain.Attempts, executionmodel.StepAttemptRecord{
		ProjectIdentity: old.ProjectIdentity, AttemptID: newAttemptID, ExecutionID: old.ExecutionID,
		StepID: "worker", AttemptNumber: old.AttemptNumber + 1, Status: "planned",
		InputRevision: old.InputRevision, ContractDigest: old.ContractDigest,
		ExternalBinding: old.ExternalBinding, CreatedAt: created, UpdatedAt: created,
	})
	execution.Process = nil
	execution.CurrentAttemptID = newAttemptID
	execution.ImplementationData = nil
	execution.PolicyResources = nil
	execution.PendingCommandID, execution.CurrentCommandID = "", ""
	execution.PendingTurnID, execution.CurrentTurnID, execution.LastStoppedTurnID = "", "", ""
	execution.PendingQuestions = nil
	execution.SessionGeneration = ""
	execution.TurnStartedAt, execution.TurnEndedAt = nil, nil
	execution.LogPath = nil
	execution.Status, execution.LastTurnOutcome, execution.RecoveryState = "pending", "", "resuming"
	execution.RecoveryRequired = false
	if mode == "continue" {
		execution.Generation = newID()
		execution.SessionID = originalSession
	} else {
		execution.Generation = ""
		execution.SessionID = ""
	}
	execution.UpdatedAt = created
	s.refreshRun(run)
	if mode == "continue" {
		workflow, execution = s.workflowRecords(run)
		attempt := s.workflowAttempt(run, run.AttemptID)
		next, err := s.lifecycleSnapshot(run, workflow, execution, attempt, nil)
		if err != nil {
			return err
		}
		seed, err := lifecycle.BindRecovery(ctx, previous, next)
		if err != nil {
			return err
		}
		if err := putImplementationEnvelope(execution, seed); err != nil {
			return err
		}
	}
	return s.commitWorker(run)
}

// A saved answer from a previous service instance is historical input. A new
// Decision gives the user a distinct, first-wins authorization for this run.
func (s *Service) refreshSavedRecoveryDecision(run *reconstructiontypes.RunView, preserveNative bool, reason string) error {
	if run == nil || run.Decision == nil || run.Decision.Status != "answer_saved" {
		return nil
	}
	old := *run.Decision
	execution := s.executionRecord(run)
	if execution == nil {
		return errors.New("saved recovery answer lost its execution")
	}
	if old.SourceAttemptID == run.AttemptID && old.RecoveryVersion == execution.RecoveryVersion &&
		execution.RecoveryMarkedAt != nil && old.AnsweredAt != nil && old.AnsweredAt.After(*execution.RecoveryMarkedAt) {
		return nil // This answer already authorizes the current recovery revision.
	}
	for _, op := range s.domain.Operations {
		if op.IdempotencyKey != "worker-question-answer:"+execution.ExecutionID+":"+old.ID {
			continue
		}
		if op.Status == "confirmed" {
			s.setDecisionStatus(run, "reply_accepted")
			return s.commitWorker(run)
		}
		if op.Status == "in_flight" || op.Status == "accepted" || op.Status == "outcome_unknown" {
			execution.RecoveryState = "unverified"
			return s.waitWorkerFlow(run, "Original answer send has an unknown outcome; reconcile its native result before confirming again")
		}
	}
	oldInput := "Prior saved answer: " + workerFeedbackText(executionmodel.DecisionAnswer{OptionID: old.OptionID, Note: old.Note}, old.Request)
	if len(old.Answers) != 0 {
		oldInput += "\nNative answers: " + string(mustJSON(old.Answers))
	}
	s.setDecisionStatus(run, "superseded")
	if err := s.commitWorker(run); err != nil {
		return err
	}
	s.setRunDecision(run, nil)
	if preserveNative {
		request := old.Request
		request.Context = clip(reason+"\n\n"+oldInput+"\n\nPlease confirm or revise this answer for the live original request.", 7000)
		decision, err := WriteDecisionRequest(run, request)
		if err != nil {
			return err
		}
		s.setRunDecision(run, decision)
	} else {
		if err := s.waitWorkerFlow(run, reason+"\n\n"+oldInput); err != nil {
			return err
		}
	}
	if record := s.decisionRecord(run); record != nil {
		record.RecoveryOfDecision = old.ID
		s.markWorkerDecisionWrite(record.DecisionID)
		record.RecoveryVersion = execution.RecoveryVersion
		record.RecoveryState = execution.RecoveryState
	}
	return s.commitWorker(run)
}

func (s *Service) startManuallyRecoveredInitialWorker(ctx context.Context, run *reconstructiontypes.RunView, answer executionmodel.DecisionAnswer, commandID string) error {
	if answer.OptionID != "continue" || run.Decision == nil || run.Decision.Request.QuestionRoute != nil {
		return errors.New("initial local execution requires an explicit continue decision")
	}
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return err
	}
	execution := fixed.execution
	if !execution.RecoveryRequired || execution.Process != nil || execution.PendingCommandID != "" || execution.CurrentTurnID != "" ||
		execution.TurnNumber != 0 && run.DomainAttemptNumber <= 1 || run.WorkspaceIntent == nil {
		return errors.New("initial worker preparation changed during recovery")
	}
	if s.hasUnknownWorkerOperation(run) {
		return s.waitWorkerFlow(run, "An earlier external operation has an unknown outcome; reconcile it before continuing")
	}
	task := nativeTaskRecord(s.domain.Tasks, run.DomainTaskID)
	if task == nil {
		return errors.New("fixed task is unavailable")
	}
	contract, err := s.commandStore.TaskContract(ctx, fixed.config.identity, *task)
	if err != nil {
		return err
	}
	prompt, err := Prompt(*task, contract, run.RunID, fixed.config.PublicationPolicy)
	if err != nil {
		return err
	}
	prompt += "\n\nThe service reopened before the first worker turn. Inspect the current worktree before acting.\n\n" + workerFeedbackText(answer, run.Decision.Request)
	key := "worker-workspace:" + execution.ExecutionID
	var preparation *executionmodel.OperationRecord
	for i := range s.domain.Operations {
		if s.domain.Operations[i].IdempotencyKey == key {
			preparation = &s.domain.Operations[i]
			break
		}
	}
	if preparation != nil && preparation.Status != "confirmed" {
		return s.waitWorkerFlow(run, "Original workspace preparation has an unknown outcome; reconcile it before continuing")
	}
	if execution.Workspace.Worktree == "" {
		if preparation == nil {
			opID, err := s.startWorkerOperation(run, "prepare_workspace", key, struct {
				WorkflowRunID string                           `json:"workflow_run_id"`
				ExecutionID   string                           `json:"execution_id"`
				Workspace     executionbackend.WorkspaceIntent `json:"workspace"`
			}{run.RunID, execution.ExecutionID, *run.WorkspaceIntent})
			if err != nil {
				return err
			}
			if _, err := os.Lstat(run.WorkspaceIntent.Path); os.IsNotExist(err) {
				preparation, prepareErr := prepareWorktree(ctx, fixed.config, run)
				if prepareErr != nil {
					status := string(preparation.Status)
					if status != string(executionbackend.OperationRejected) {
						status = "outcome_unknown"
					}
					_ = s.changeWorkerOperation(run, opID, status, "", mustJSON(preparation), prepareErr.Error())
					return s.waitWorkerFlow(run, "Original workspace preparation needs reconciliation: "+prepareErr.Error())
				}
			} else if err != nil {
				return err
			}
			workspace, err := s.inspectWorkflowWorkspace(ctx, run)
			if err != nil {
				_ = s.changeWorkerOperation(run, opID, "outcome_unknown", "", nil, err.Error())
				return s.waitWorkerFlow(run, "Original workspace identity is unverified: "+err.Error())
			}
			applyWorkflowWorkspaceEvidence(run, execution, workspace)
			if err := s.changeWorkerOperation(run, opID, "confirmed", "", mustJSON(workspace), ""); err != nil {
				return err
			}
		} else {
			workspace, err := s.inspectWorkflowWorkspace(ctx, run)
			if err != nil {
				return s.waitWorkerFlow(run, "Prepared workspace identity is unverified: "+err.Error())
			}
			applyWorkflowWorkspaceEvidence(run, execution, workspace)
		}
	}
	workspace, err := s.inspectWorkflowWorkspace(ctx, run)
	if err != nil {
		return s.waitWorkerFlow(run, "Current worktree identity is unverified: "+err.Error())
	}
	applyWorkflowWorkspaceEvidence(run, execution, workspace)
	s.setDecisionStatus(run, "applied")
	s.setRunDecision(run, nil)
	execution.RecoveryRequired = false
	execution.RecoveryState = "resuming"
	if err := s.commitWorker(run); err != nil {
		return err
	}
	return s.startWorkerFlowTurn(ctx, run, prompt, commandID)
}

// Every service instance must revalidate unfinished executions before a saved
// answer or dispatch intent may write to an external worker. The gate is part
// of the Execution row, so an abrupt process exit has the same startup rule.
func (s *Service) adoptOrGateServiceExecutions() error {
	return s.markServiceRecoveryGateForPriorOwner()
}

func (s *Service) markServiceRecoveryGateForPriorOwner() error {
	for _, run := range s.runs {
		if run == nil || run.WorkflowVersion != executionmodel.Version || run.TaskStatus != "in_progress" {
			continue
		}
		workflow, execution := s.workflowRecords(run)
		if workflow == nil || execution == nil || workflow.Phase == "completed" || workflow.Phase == "abandoned" {
			continue
		}
		if execution.OwnerServiceInstanceID != s.State.Service.InstanceID {
			s.markOneServiceRecoveryGate(run)
			execution.OwnerServiceInstanceID = s.State.Service.InstanceID
			s.refreshRun(run)
			if err := s.commitWorker(run); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) markServiceRecoveryGate() error {
	for _, run := range s.runs {
		s.markOneServiceRecoveryGate(run)
		if run != nil && run.TaskStatus == "in_progress" {
			if err := s.commitWorker(run); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) markOneServiceRecoveryGate(run *reconstructiontypes.RunView) {
	if run == nil || run.WorkflowVersion != executionmodel.Version {
		return
	}
	task := nativeTaskRecord(s.domain.Tasks, run.DomainTaskID)
	workflow, execution := s.workflowRecords(run)
	if task == nil || workflow == nil || execution == nil || task.Status != "in_progress" || workflow.Phase == "completed" || workflow.Phase == "abandoned" {
		return
	}
	execution.RecoveryRequired = true
	execution.RecoveryVersion++
	execution.RecoveryState = "unverified"
	marked := now()
	execution.RecoveryMarkedAt = &marked
	execution.UpdatedAt = marked
	s.refreshRun(run)
}

func (s *Service) resumeConfiguredWork(ctx context.Context) error {
	if err := s.resumeWorkerFlow(ctx); err != nil {
		return err
	}
	for _, run := range s.runs {
		if run == nil || run.WorkflowVersion != executionmodel.Version {
			continue
		}
		workflow, execution := s.workflowRecords(run)
		if workflow == nil || execution == nil || workflow.Phase != "completed" {
			continue
		}
		var cleanupErr error
		lifecycle, _, resolveErr := s.lifecycleForRun(run)
		if resolveErr != nil {
			cleanupErr = resolveErr
		} else {
			cleanupErr = s.cleanupLifecycleResources(ctx, run, lifecycle)
		}
		if cleanupErr != nil {
			execution.LastEvent = ptr("Task delivery completed; owned execution cleanup needs reconciliation: " + cleanupErr.Error())
			s.refreshRun(run)
			if saveErr := s.commitWorker(run); saveErr != nil {
				return saveErr
			}
		}
	}
	return nil
}

func (s *Service) resumeWorkerFlow(ctx context.Context) error {
	if err := s.resumeAppServerWorkers(ctx); err != nil {
		return err
	}
	// An unavailable candidate must not prevent its siblings' saved work from
	// advancing. Keep each diagnostic while visiting every independent run.
	var resumeErr error
	for _, run := range s.runs {
		if run == nil || run.WorkflowVersion != executionmodel.Version {
			continue
		}
		if s.candidateBusinessClosed(run) {
			if err := s.resumeRetiredCandidate(ctx, run); err != nil {
				resumeErr = errors.Join(resumeErr, err)
			}
			continue
		}
		if s.workers[run.RunID] != nil {
			continue
		}
		task := nativeTaskRecord(s.domain.Tasks, run.DomainTaskID)
		workflow, execution := s.workflowRecords(run)
		if task == nil || workflow == nil || execution == nil || task.Status != "in_progress" || workflow.Phase == "completed" {
			continue
		}
		adapter, err := s.workerFlowRegistration(run)
		if err != nil {
			resumeErr = errors.Join(resumeErr, err)
			continue
		}
		_, err = s.resumeRegisteredWorkerFlow(ctx, task, run, adapter)
		if err != nil {
			resumeErr = errors.Join(resumeErr, err)
		}
	}
	return resumeErr
}
