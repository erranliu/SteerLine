package steerline

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	"github.com/erranliu/SteerLine/internal/steerline/workerflow"
)

func readWorkerTurnEvidence(fixed workerFlowContext, mode workerflow.Mode) (workerTurnEvidence, error) {
	run, c := fixed.run, fixed.config
	execution := workerSessionContext(run, fixed.execution)
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
