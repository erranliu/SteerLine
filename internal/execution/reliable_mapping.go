package execution

import (
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
	"github.com/erranliu/SteerLine/internal/execution/reliable"
)

// Application authorities become narrow constraints; reliable never reads a
// Task, business step, Decision record or mutable Service.
func (s *Service) workerContinuationConstraints(run *reconstructiontypes.RunView) reliable.ContinuationConstraints {
	e := s.executionRecord(run)
	return reliable.ContinuationConstraints{RecoveryRequired: e.RecoveryRequired, PriorOwner: e.OwnerServiceInstanceID != s.State.Service.InstanceID, QuestionPending: hasBlockingPendingQuestions(e), OutcomeUnknown: s.hasUnknownWorkerOperation(run), EnvironmentBlocked: e.Environment != nil && (!e.Environment.Ready || e.Environment.Blocker != nil), UnconfirmedAnswer: run.Decision != nil && run.Decision.Status == "answer_saved" && run.Decision.Request.InteractionKind != "async"}
}

func reliableBinding(e *executionmodel.ExecutionRecord) reliable.BindingSnapshot {
	value := reliable.BindingSnapshot{ProjectIdentity: e.ProjectIdentity, ExecutionID: e.ExecutionID, CurrentAttemptID: e.CurrentAttemptID, Generation: e.Generation, ConfigurationDigest: e.ConfigurationDigest, Target: e.Target, Workspace: reliable.Workspace(e.Workspace), Process: copyPtr(e.Process), SessionID: e.SessionID, SessionGeneration: e.SessionGeneration, PendingCommandID: e.PendingCommandID, PendingTurnID: e.PendingTurnID, CurrentTurnID: e.CurrentTurnID, ImplementationData: CloneImplementationData(e.ImplementationData)}
	if e.Environment != nil {
		value.Environment = &reliable.EnvironmentBinding{OriginalTerminal: copyPtr(e.Environment.OriginalTerminal), Runtime: e.Environment.Runtime, LaunchCommand: e.Environment.LaunchCommand, WriteRoots: append([]string(nil), e.Environment.WriteRoots...)}
	}
	if e.PolicyResources != nil {
		value.PolicyResources = &reliable.PolicyBinding{WriteRoots: append([]string(nil), e.PolicyResources.WriteRoots...)}
	}
	return value
}
func applyReliableBinding(e *executionmodel.ExecutionRecord, value reliable.BindingSnapshot) {
	e.Target, e.Workspace, e.Process = value.Target, executionmodel.WorkspaceEvidence(value.Workspace), value.Process
	e.SessionID, e.SessionGeneration, e.PendingTurnID = value.SessionID, value.SessionGeneration, value.PendingTurnID
	e.ImplementationData = value.ImplementationData
	if e.Environment != nil && value.Environment != nil {
		e.Environment.OriginalTerminal, e.Environment.Runtime = value.Environment.OriginalTerminal, value.Environment.Runtime
		e.Environment.LaunchCommand, e.Environment.WriteRoots = value.Environment.LaunchCommand, value.Environment.WriteRoots
	}
}

func reliableObservation(e *executionmodel.ExecutionRecord) reliable.ObservationIdentity {
	return reliable.ObservationIdentity{ExecutionID: e.ExecutionID, AttemptID: e.CurrentAttemptID, Generation: e.Generation, RecoveryVersion: e.RecoveryVersion, CommandID: e.CurrentCommandID, TurnID: e.CurrentTurnID, PendingCommandID: e.PendingCommandID, PendingTurnID: e.PendingTurnID}
}

func reliableTurn(e *executionmodel.ExecutionRecord) reliable.TurnState {
	return reliable.TurnState{SessionID: e.SessionID, PendingTurnID: e.PendingTurnID, CurrentTurnID: e.CurrentTurnID, LastStoppedTurnID: e.LastStoppedTurnID, Status: e.Status, Number: e.TurnNumber, UpdatedAt: e.UpdatedAt}
}
func applyReliableTurn(e *executionmodel.ExecutionRecord, value reliable.TurnState) {
	e.SessionID, e.PendingTurnID, e.CurrentTurnID = value.SessionID, value.PendingTurnID, value.CurrentTurnID
	e.LastStoppedTurnID, e.Status, e.TurnNumber, e.UpdatedAt = value.LastStoppedTurnID, value.Status, value.Number, value.UpdatedAt
}

func storedTurnEvidence(v executionmodel.StageResult) executionbackend.TurnEvidence {
	return executionbackend.TurnEvidence{CommandID: v.CommandID, TurnID: v.TurnID, SessionID: v.SessionID, Succeeded: v.Succeeded, NativeOutcome: v.NativeOutcome, LastMessage: v.Final, FinalVerified: v.FinalVerified, FinalSource: v.FinalSource, FinalItemID: v.FinalItemID, FinalNativeSessionID: v.FinalNativeSessionID, FinalNativeTurnID: v.FinalNativeTurnID, Failure: v.Failure, FinalDiagnostic: v.FinalDiagnostic}
}
