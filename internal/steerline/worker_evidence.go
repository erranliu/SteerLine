package steerline

import (
	"errors"

	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/workerflow"
)

// Worker protocol adapters verify native evidence before the service changes
// business records. Empty fields mean that the source did not provide a fact.
type workerFlowEventFact struct {
	executionapi.EventFact
	Question *codexAskCall
}

// workerSessionContext converts persisted records to fixed, value-only input.
// The adapter cannot write WorkflowRun, Execution or Decision records through it.
func workerSessionContext(run *Run, record *ExecutionRecord) *executionapi.SessionContext {
	if record == nil {
		return nil
	}
	fixed := &executionapi.SessionContext{
		ProjectID: record.ProjectIdentity, WorkflowRunID: record.WorkflowRunID,
		ExecutionID: record.ExecutionID, AttemptID: record.CurrentAttemptID,
		Agent: executionapi.AgentSpec{Name: record.Agent}, Target: record.Target,
		SessionID: record.SessionID, Generation: record.AppServerGeneration,
		CurrentTurnID: record.CurrentTurnID, PendingTurnID: record.PendingTurnID,
		CurrentCommandID: record.CurrentCommandID, PendingCommandID: record.PendingCommandID,
	}
	if run != nil {
		fixed.TaskID = run.TaskID
	}
	return fixed
}

func decodeWorkerFlowEvent(mode workerflow.Mode, ev WorkerEvent, execution *executionapi.SessionContext, inspectReply bool) (workerFlowEventFact, error) {
	fact := workerFlowEventFact{EventFact: executionapi.EventFact{Source: ev.Source, SourceEventID: ev.SourceEventID, SourceCursor: ev.SourceCursor, SourceIdentity: ev.SourceIdentity, SourceSegment: ev.SourceSegment, SourceOffset: ev.SourceOffset, SourceLength: ev.SourceLength}}
	if execution == nil {
		return fact, errors.New("worker event has no fixed execution")
	}
	if mode != workerflow.LocalCodex {
		return fact, errors.New("Orca messages require their fixed Dispatch adapter")
	}
	if ev.Session != "" {
		if !codexThreadID.MatchString(ev.Session) || execution.SessionID != "" && execution.SessionID != ev.Session {
			return fact, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "worker event changed the original Codex session"}
		}
		fact.SessionID = ev.Session
	}
	if ev.AppServer == nil {
		if ev.Activity.Kind == "turn" && ev.Activity.Status == "started" {
			return fact, errors.New("app-server turn start has no native evidence")
		}
		return fact, nil
	}
	native, err := codexworker.DecodeEvent(*ev.AppServer, execution.SessionID, execution.CurrentTurnID, inspectReply)
	if err != nil {
		return fact, err
	}
	fact.StartedTurnID, fact.ToolReply = native.StartedTurnID, native.ToolReply
	if native.Question != nil {
		call, err := codexAskCallFromEnvelope(*native.Question)
		if err != nil {
			return fact, err
		}
		call.Generation = execution.Generation
		fact.Question = &call
	}
	return fact, nil
}

type workerTurnEvidence struct {
	executionapi.TurnEvidence
	PublishRequested bool
	LocalCodex       bool
}
