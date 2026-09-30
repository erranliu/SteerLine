package steerline

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	"path/filepath"
	"strings"

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
	ToolReply     *workerToolReplyFact
}

type workerToolReplyFact struct {
	ThreadID string
	TurnID   string
	CallID   string
	Status   string
	Success  *bool
	Answers  []map[string][]string
}

func (f workerToolReplyFact) matchesAnswers(expected map[string][]string) bool {
	for _, answers := range f.Answers {
		if digestValue(answers) == digestValue(expected) {
			return true
		}
	}
	return false
}

func decodeWorkerFlowEvent(mode workerflow.Mode, ev WorkerEvent, execution *ExecutionRecord, inspectReply bool) (workerFlowEventFact, error) {
	fact := workerFlowEventFact{Source: ev.Source, SourceEventID: ev.SourceEventID, SourceCursor: ev.SourceCursor}
	if execution == nil {
		return fact, errors.New("worker event has no fixed execution")
	}
	if mode != workerflow.LocalCodex && mode != workerflow.SupervisedOrca {
		return fact, errors.New("native Orca messages require the fixed Dispatch adapter")
	}
	if mode == workerflow.SupervisedOrca && ev.AppServer != nil {
		return fact, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "supervised Orca event used the local app-server protocol"}
	}
	if ev.Session != "" {
		if !codexThreadID.MatchString(ev.Session) || execution.SessionID != "" && execution.SessionID != ev.Session {
			return fact, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "worker event changed the original Codex session"}
		}
		fact.SessionID = ev.Session
	}
	if mode != workerflow.LocalCodex {
		if ev.Activity.Kind == "turn" && ev.Activity.Status == "started" {
			fact.StartedTurnID = execution.PendingTurnID
			if fact.StartedTurnID == "" {
				fact.StartedTurnID = execution.CurrentTurnID
			}
		}
		return fact, nil
	}
	if ev.AppServer == nil {
		if ev.Activity.Kind == "turn" && ev.Activity.Status == "started" {
			return fact, errors.New("app-server turn start has no native evidence")
		}
		return fact, nil
	}
	switch ev.AppServer.Method {
	case "turn/started":
		var params struct {
			ThreadID string `json:"threadId"`
			Turn     struct {
				ID string `json:"id"`
			} `json:"turn"`
		}
		if json.Unmarshal(ev.AppServer.Params, &params) != nil || params.ThreadID != execution.SessionID || !codexThreadID.MatchString(params.Turn.ID) {
			return fact, errors.New("app-server turn start changed its fixed thread")
		}
		fact.StartedTurnID = params.Turn.ID
	case "item/tool/call":
		call, err := parseCodexAskCall(*ev.AppServer, execution.SessionID, execution.CurrentTurnID)
		if err != nil {
			return fact, err
		}
		call.Generation = execution.AppServerGeneration
		fact.Question = &call
	case "item/completed":
		if !inspectReply {
			break
		}
		var params struct {
			ThreadID string `json:"threadId"`
			TurnID   string `json:"turnId"`
			Item     struct {
				ID           string `json:"id"`
				Type         string `json:"type"`
				Status       string `json:"status"`
				Success      *bool  `json:"success"`
				ContentItems []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"contentItems"`
			} `json:"item"`
		}
		if json.Unmarshal(ev.AppServer.Params, &params) != nil || params.ThreadID != execution.SessionID || params.TurnID != execution.CurrentTurnID {
			return fact, errors.New("app-server tool completion changed native identity")
		}
		if params.Item.Type != "dynamicToolCall" {
			break
		}
		reply := workerToolReplyFact{ThreadID: params.ThreadID, TurnID: params.TurnID, CallID: params.Item.ID, Status: params.Item.Status, Success: params.Item.Success}
		for _, content := range params.Item.ContentItems {
			if content.Type != "inputText" {
				continue
			}
			var body struct {
				Answers map[string][]string `json:"answers"`
			}
			if json.Unmarshal([]byte(content.Text), &body) == nil {
				reply.Answers = append(reply.Answers, body.Answers)
			}
		}
		fact.ToolReply = &reply
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
	QuestionHint     bool
	PublishRequested bool
	LocalCodex       bool
}

func readWorkerTurnEvidence(fixed workerFlowContext, mode workerflow.Mode) (workerTurnEvidence, error) {
	run, execution, c := fixed.run, fixed.execution, fixed.config
	var fact workerTurnEvidence
	if run == nil || execution == nil {
		return fact, errors.New("worker turn has no fixed execution")
	}
	fact.LocalCodex = mode == workerflow.LocalCodex
	fact.TurnID = execution.CurrentTurnID
	if fact.TurnID == "" {
		fact.TurnID = execution.PendingTurnID
	}
	if fact.LocalCodex && !codexThreadID.MatchString(fact.TurnID) || !fact.LocalCodex && !stableExecutionID(fact.TurnID) {
		return fact, errors.New("The original worker exited without a saved turn identity")
	}
	fact.CommandID = fact.TurnID
	var outcome codexworker.Outcome
	var err error
	if fact.LocalCodex {
		fact.CommandID = execution.CurrentCommandID
		if fact.CommandID == "" {
			fact.CommandID = execution.PendingCommandID
		}
		if !stableExecutionID(fact.CommandID) {
			return fact, errors.New("Original app-server turn lacks a fixed command or native turn identity")
		}
		path := filepath.Join(c.RunDir, "runs", run.RunID, "turns", fact.CommandID, "appserver.jsonl")
		fact.SourcePath = path
		outcome, err = readAppServerTurnOutcome(path, execution.SessionID, fact.TurnID)
	} else if mode == workerflow.SupervisedOrca {
		path := filepath.Join(c.RunDir, "runs", run.RunID, "turns", fact.TurnID, "events.jsonl")
		fact.SourcePath = path
		outcome, err = codexworker.ReadExecOutcome(path)
	} else {
		return fact, errors.New("native Orca settlement requires its Dispatch adapter")
	}
	if err != nil {
		return fact, fmt.Errorf("original worker turn output is insufficient: %w", err)
	}
	if !outcome.Started || outcome.SessionID == "" || execution.SessionID != "" && execution.SessionID != outcome.SessionID {
		return fact, errors.New("The original worker exited, but its turn output is insufficient to determine the result")
	}
	fact.SessionID, fact.Started, fact.Completed = outcome.SessionID, outcome.Started, outcome.Completed
	fact.Succeeded = outcome.NativeStatus == "completed"
	fact.LastMessage, fact.Failure = outcome.LastMessage, outcome.Failure
	fact.PRCandidates = outcome.PRCandidates
	if fact.LocalCodex && !fact.Succeeded {
		fact.PRCandidates = nil
	}
	if mode == workerflow.SupervisedOrca {
		last := strings.TrimSpace(outcome.LastMessage)
		fact.QuestionHint = strings.HasSuffix(last, "?") || strings.HasSuffix(last, "？")
	}
	fact.PublishRequested = fact.LocalCodex && fact.Succeeded && localWorkerRequestedPublish(fact.LastMessage, run.RunID)
	return fact, nil
}
