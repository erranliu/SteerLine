package steerline

import "github.com/erranliu/SteerLine/internal/steerline/projection"

func runtimeCheckpoint(v Runtime) ServiceCheckpoint { return projection.Checkpoint(v) }

// projectRuntime is composition for already-loaded rows, without reuse of client Run pointers.
func projectRuntime(v *Runtime) error {
	snapshot := DomainSnapshot{Tasks: v.Tasks, WorkflowRuns: v.WorkflowRuns, Executions: v.Executions, Attempts: v.Attempts, Decisions: v.Decisions, Operations: v.Operations, Deliveries: v.Deliveries}
	next, err := projection.Build(snapshot, projection.MetadataOf(*v))
	if err != nil {
		return err
	}
	*v = next
	return nil
}

func refreshExecutionView(run *Run, e *ExecutionRecord) { *run = projection.Execution(*run, *e) }

func projectSlot(v *Runtime) { v.SlotOccupied, v.CurrentTask = projection.Slot(*v) }

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
	task := nativeTaskRecord(s.domain.Tasks, run.TaskID)
	if workflow == nil || execution == nil || task == nil {
		return
	}
	view, err := projection.Run(*task, *workflow, *execution, s.domain.Attempts, s.domain.Decisions, s.domain.Deliveries)
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
	s.markWorkerDecisionWrite(decision.ID)
	record := DecisionRecord{ProjectIdentity: s.Config.identity, DecisionID: decision.ID, Scope: "execution",
		WorkflowRunID: run.RunID, ExecutionID: run.ExecutionID, AttemptID: decision.AttemptID, QuestionID: decision.ID,
		Request: mustJSON(decision.Request), Status: decision.Status, Version: decision.Version, CreatedAt: now(), UpdatedAt: now()}
	if decision.Request.QuestionRoute != nil {
		record.QuestionID = decision.Request.QuestionRoute.QuestionID
	}
	for i := range s.domain.Decisions {
		if s.domain.Decisions[i].DecisionID == record.DecisionID {
			record.CreatedAt = s.domain.Decisions[i].CreatedAt
			s.domain.Decisions[i] = record
			execution.DecisionID = record.DecisionID
			s.refreshRun(run)
			return
		}
	}
	s.domain.Decisions = append(s.domain.Decisions, record)
	execution.DecisionID = record.DecisionID
	s.refreshRun(run)
}

func (s *Service) decisionRecord(run *Run) *DecisionRecord {
	if run == nil || run.Decision == nil {
		return nil
	}
	for i := range s.domain.Decisions {
		if s.domain.Decisions[i].DecisionID == run.Decision.ID {
			return &s.domain.Decisions[i]
		}
	}
	return nil
}
func (s *Service) setDecisionStatus(run *Run, status string) {
	record := s.decisionRecord(run)
	record.Status, record.Version, record.UpdatedAt = status, record.Version+1, now()
	s.markWorkerDecisionWrite(record.DecisionID)
	s.refreshRun(run)
}

func (s *Service) markWorkerDecisionWrite(id string) {
	if s.workerDecisionWrites == nil {
		s.workerDecisionWrites = make(map[string]bool)
	}
	s.workerDecisionWrites[id] = true
}
