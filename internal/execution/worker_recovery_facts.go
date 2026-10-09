package execution

import (
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
)

// Actual worker evidence, not the declared service-stop behavior, determines
// recovery eligibility. Repeated identical evidence never invalidates consent.
func (s *Service) setWorkerRecoveryFacts(run *reconstructiontypes.RunView, facts executionmodel.RecoveryFacts, sessionAvailable bool) {
	e := s.executionRecord(run)
	facts.OutcomeUncertain = facts.OutcomeUncertain || s.hasUnknownWorkerOperation(run)
	state := string(executionmodel.ClassifyRecovery(facts))
	if e.RecoverySessionAvailable != nil && (e.RecoveryState != state || *e.RecoverySessionAvailable != sessionAvailable) {
		e.RecoveryVersion++
	}
	e.RecoveryState, e.RecoverySessionAvailable = state, ptr(sessionAvailable)
}

func settledOriginalWorkerTurn(e *executionmodel.ExecutionRecord) bool {
	return e.CurrentTurnID != "" && e.CurrentTurnID == e.LastStoppedTurnID && e.PendingCommandID == "" && e.PendingTurnID == ""
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
