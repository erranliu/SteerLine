package execution

import (
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
	"github.com/erranliu/SteerLine/internal/execution/reliable"
)

func (s *Service) markWorkerRecoveryRequired(run *reconstructiontypes.RunView) {
	e := s.executionRecord(run)
	if !e.RecoveryRequired {
		e.RecoveryRequired, e.RecoveryState = true, "unverified"
		e.RecoveryVersion++
		e.RecoveryMarkedAt = ptr(now())
		e.RecoverySessionAvailable = nil
	}
}

func (s *Service) workerContinuationQualified(run *reconstructiontypes.RunView, evidence executionbackend.LifecycleRecoveryEvidence) bool {
	return reliable.CanContinueOriginal(evidence) && !s.hasUnknownWorkerOperation(run) && s.workflowRecord(run).Status != "reconciling"
}

func (s *Service) setWorkerRecoveryEvidence(run *reconstructiontypes.RunView, evidence executionbackend.LifecycleRecoveryEvidence) {
	completed := s.originalWorkerTurnEnded(run)
	w := s.workflowRecord(run)
	core := (w.StepID == executionmodel.StepImplement || w.StepID == executionmodel.StepReport) && w.PRURL == ""
	questionPending := run.Decision != nil && run.Decision.Request.QuestionRoute != nil &&
		(run.Decision.Status == "pending" || run.Decision.Status == "answer_saved")
	s.setWorkerRecoveryFacts(run, reliable.RecoveryFacts{WorkerLive: evidence.ResourceAvailable, WorkerAbsent: evidence.ProcessAbsent, WritersAbsent: evidence.WritersAbsent, Completed: completed,
		OutcomeUncertain: core && (completed || evidence.ProcessAbsent && evidence.WritersAbsent) && !s.workerContinuationQualified(run, evidence), QuestionPending: questionPending}, evidence.SessionAvailable)
}

// Actual worker evidence, not the declared service-stop behavior, determines
// recovery eligibility. Repeated identical evidence never invalidates consent.
func (s *Service) setWorkerRecoveryFacts(run *reconstructiontypes.RunView, facts reliable.RecoveryFacts, sessionAvailable bool) {
	e := s.executionRecord(run)
	value := reliable.RecoveryChange(reliable.RecoveryRevision{State: e.RecoveryState, SessionAvailable: e.RecoverySessionAvailable, Version: e.RecoveryVersion}, facts, sessionAvailable, s.hasUnknownWorkerOperation(run))
	e.RecoveryState, e.RecoverySessionAvailable, e.RecoveryVersion = value.State, value.SessionAvailable, value.Version
}

func settledOriginalWorkerTurn(e *executionmodel.ExecutionRecord) bool {
	return e.CurrentTurnID != "" && e.CurrentTurnID == e.LastStoppedTurnID && e.PendingCommandID == "" && e.PendingTurnID == ""
}

func (s *Service) originalWorkerTurnEnded(run *reconstructiontypes.RunView) bool {
	e := s.executionRecord(run)
	if settledOriginalWorkerTurn(e) {
		return true
	}
	// A queued next stage is not an unresolved native input. Its predecessor
	// can qualify continuation while the unsent stage remains behind consent.
	return e.PendingCommandID != "" && s.workerInputNotSubmitted(run, e.PendingCommandID) &&
		(e.CurrentTurnID == "" || e.CurrentTurnID == e.LastStoppedTurnID)
}

// Session preparation differs from model input. Explicit rejection is settled
// without acceptance; an unknown outcome is never treated as unsent.
func (s *Service) workerInputNotSubmitted(run *reconstructiontypes.RunView, command string) bool {
	e := s.executionRecord(run)
	if command == "" || stageInput(s.workflowRecord(run), command) == nil {
		return false
	}
	status := func(key string) string {
		if op := s.operationByKey(key + e.ExecutionID + ":" + command); op != nil {
			return op.Status
		}
		return ""
	}
	return reliable.InputNotSubmitted(status("worker-turn:"), status("worker-input:"), status("worker-session:"))
}

func (s *Service) stampPendingRecoveryQuestion(run *reconstructiontypes.RunView) {
	if run.Decision == nil || run.Decision.Status != "pending" || run.Decision.Request.QuestionRoute == nil {
		return
	}
	e, record := s.executionRecord(run), s.decisionRecord(run)
	if record != nil && (record.RecoveryVersion != e.RecoveryVersion || record.RecoveryState != e.RecoveryState) {
		record.RecoveryVersion, record.RecoveryState = e.RecoveryVersion, e.RecoveryState
		record.Version++
		s.markWorkerDecisionWrite(record.DecisionID)
	}
}
