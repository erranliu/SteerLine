package execution

import (
	contracts "github.com/erranliu/SteerLine/internal/steerline/contracts"
)

import (
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

func workerSessionContext(run *contracts.Run, record *executionmodel.ExecutionRecord) *executionbackend.SessionContext {
	if record == nil {
		return nil
	}
	sessionID, generation := executionSessionFacts(record)
	fixed := &executionbackend.SessionContext{
		ProjectID: record.ProjectIdentity, WorkflowRunID: record.WorkflowRunID,
		ExecutionID: record.ExecutionID, AttemptID: record.CurrentAttemptID,
		Agent: executionbackend.AgentSpec{Name: record.Agent}, Target: record.Target,
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
	executionbackend.TurnEvidence
	PublishRequested bool
}

// These facts describe original turn/question readiness, not backend identity.
type workerWaitFacts struct {
	PreserveUnresolvedTurn bool
	PreserveActiveStatus   bool
	ContextPrefix          string
	Options                []executionbackend.DecisionOption
}
