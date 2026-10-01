package steerline

import executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"

// workerSessionContext copies the fixed execution identity into a value-only
// input for implementation codecs.
func workerSessionContext(run *Run, record *ExecutionRecord) *executionapi.SessionContext {
	if record == nil {
		return nil
	}
	sessionID, generation := executionSessionFacts(record)
	fixed := &executionapi.SessionContext{
		ProjectID: record.ProjectIdentity, WorkflowRunID: record.WorkflowRunID,
		ExecutionID: record.ExecutionID, AttemptID: record.CurrentAttemptID,
		Agent: executionapi.AgentSpec{Name: record.Agent}, Target: record.Target,
		SessionID: sessionID, Generation: generation,
		CurrentTurnID: record.CurrentTurnID, PendingTurnID: record.PendingTurnID,
		CurrentCommandID: record.CurrentCommandID, PendingCommandID: record.PendingCommandID,
	}
	if run != nil {
		fixed.TaskID = run.TaskID
	}
	return fixed
}

// Worker protocol adapters verify native evidence before the service changes
// business records. Empty fields mean that the source did not provide a fact.
type workerTurnEvidence struct {
	executionapi.TurnEvidence
	PublishRequested bool
}

// These facts describe original turn/question readiness, not backend identity.
type workerWaitFacts struct {
	PreserveUnresolvedTurn bool
	PreserveActiveStatus   bool
	ContextPrefix          string
	Options                []executionapi.DecisionOption
}
