package steerline

import (
	"errors"

	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	"github.com/erranliu/SteerLine/internal/steerline/workerflow"
)

// Worker protocol adapters verify native evidence before the service changes
// business records. Empty fields mean that the source did not provide a fact.
type workerFlowEventFact struct {
	Source        string
	SourceEventID string
	SourceCursor  string
	SessionID     string
	StartedTurnID string
	Question      *codexAskCall
	ToolReply     *codexworker.ToolReplyFact
}

func decodeWorkerFlowEvent(mode workerflow.Mode, ev WorkerEvent, execution *ExecutionRecord, inspectReply bool) (workerFlowEventFact, error) {
	fact := workerFlowEventFact{Source: ev.Source, SourceEventID: ev.SourceEventID, SourceCursor: ev.SourceCursor}
	if execution == nil {
		return fact, errors.New("worker event has no fixed execution")
	}
	if mode != workerflow.LocalCodex {
		return fact, errors.New("Orca messages require their fixed Dispatch adapter")
	}
	if ev.Session != "" {
		if !codexThreadID.MatchString(ev.Session) || execution.SessionID != "" && execution.SessionID != ev.Session {
			return fact, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "worker event changed the original Codex session"}
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
		call.Generation = execution.AppServerGeneration
		fact.Question = &call
	}
	return fact, nil
}

type workerTurnEvidence struct {
	SourcePath       string
	TurnID           string
	CommandID        string
	SessionID        string
	Started          bool
	Completed        bool
	Succeeded        bool
	LastMessage      string
	Failure          string
	PRCandidates     []string
	PublishRequested bool
	LocalCodex       bool
}
