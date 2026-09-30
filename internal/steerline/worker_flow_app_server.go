package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/erranliu/SteerLine/internal/steerline/codexapp"
	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
)

func appServerAskDecision(run *Run, call codexAskCall) (*DecisionState, error) {
	return WriteDecisionRequest(run, codexAskDecisionRequest(call))
}

type appServerProcessReceipt struct {
	Endpoint   string          `json:"endpoint"`
	Generation string          `json:"generation"`
	Identity   ProcessIdentity `json:"identity"`
}

// Native start notifications can be replayed after recovery. Only the first
// confirmed start consumes a pending Command and accepts post-turn feedback.
func (s *Service) recordAppServerTurnStarted(run *Run, workflow *WorkflowRunRecord, execution *ExecutionRecord, turnID string) (string, error) {
	commandID := execution.PendingCommandID
	fresh := commandID != ""
	if !fresh && execution.CurrentTurnID == turnID && execution.LastStoppedTurnID != turnID {
		commandID = execution.CurrentCommandID
	}
	if !stableExecutionID(commandID) || !codexThreadID.MatchString(turnID) || fresh && execution.PendingTurnID != turnID {
		return "", errors.New("app-server turn start lacks its fixed command or native turn receipt")
	}
	var operation *OperationRecord
	for i := range s.State.Operations {
		if s.State.Operations[i].IdempotencyKey == "worker-turn:"+run.ExecutionID+":"+commandID {
			operation = &s.State.Operations[i]
			break
		}
	}
	if operation == nil {
		return "", errors.New("app-server native turn has no saved start operation")
	}
	if err := BeginWorkerTurn(workflow, execution, execution.SessionID, turnID); err != nil {
		return "", err
	}
	execution.CurrentCommandID, execution.PendingCommandID = commandID, ""
	if fresh {
		if attempt := s.workflowAttempt(run, run.AttemptID); attempt != nil {
			attempt.Status, attempt.UpdatedAt = "running", now()
		}
		if run.Decision != nil && run.Decision.Status == "answer_saved" && run.Decision.Request.NativeAsk == nil {
			s.setDecisionStatus(run, "reply_accepted")
		}
		execution.Status, execution.Error = "active", nil
		s.refreshRun(run)
		execution.TurnStartedAt, execution.TurnEndedAt = ptr(now()), nil
		s.refreshRun(run)
	}
	if operation.Status == "confirmed" {
		return operation.OperationID, nil
	}
	return operation.OperationID, TransitionOperation(operation, "confirmed", turnID, mustJSON(map[string]string{"session_id": execution.SessionID, "turn_id": turnID}), "")
}

func startAppServerWorkerTurn(ctx context.Context, c Config, run *Run, execution *ExecutionRecord, commandID, prompt string, persistThread func() error) (*Worker, string, string, error) {
	if run == nil || execution == nil || run.WorkflowVersion != WorkerFlowVersion || run.Protocol != string(ProtocolLocalCodexAppServer) || run.WorktreePath == nil || run.AttemptID == "" || !stableExecutionID(commandID) || prompt == "" || c.Codex.Executable == "" {
		return nil, "", "", errors.New("app-server worker turn lacks its fixed execution, workspace or command")
	}
	if run.ExecutionID != execution.ExecutionID || run.RunID != execution.WorkflowRunID || canonical(*run.WorktreePath) != canonical(execution.Workspace.Worktree) {
		return nil, "", "", errors.New("app-server execution/workspace identity mismatch")
	}
	paths, err := codexworker.PrepareTurn(c.RunDir, run.RunID, commandID, prompt, AtomicWrite)
	if err != nil {
		return nil, "", "", err
	}
	dir, journalPath, serverDir := paths.TurnDir, paths.JournalPath, paths.ServerDir
	var process *os.Process
	if execution.Process != nil {
		var receipt appServerProcessReceipt
		data, readErr := os.ReadFile(filepath.Join(serverDir, "process.json"))
		if readErr != nil || json.Unmarshal(data, &receipt) != nil || receipt.Identity != *execution.Process || receipt.Endpoint != execution.AppServerEndpoint || receipt.Generation != execution.AppServerGeneration {
			return nil, "", "", errors.New("saved app-server process receipt differs from execution")
		}
		if execution.AppServerEndpoint == "" || execution.AppServerGeneration == "" || inspectProcess(execution.Process) != "matching process still exists" {
			return nil, "", "", errors.New("original app-server process is not verifiably live")
		}
		var err error
		process, err = os.FindProcess(execution.Process.PID)
		if err != nil {
			return nil, "", "", err
		}
	} else {
		identity, launched, endpoint, err := codexworker.LaunchServer(codexworker.LaunchConfig{
			Executable: c.Codex.Executable, Args: c.Codex.Args, Profile: c.Profile,
			Workspace: *run.WorktreePath, ServerDir: serverDir,
		}, startDetachedProcess, func(base []string) []string {
			return unsetEnv(overrideEnv(base, map[string]string{"STEERLINE_WORKFLOW_RUN_ID": run.RunID, "STEERLINE_EXECUTION_ID": run.ExecutionID}), []string{"BEADS_*", "BD_*"})
		})
		if err != nil {
			return nil, "", "", err
		}
		generation := newID()
		receipt := appServerProcessReceipt{Endpoint: endpoint, Generation: generation, Identity: identity}
		if err := AtomicWrite(filepath.Join(serverDir, "process.json"), append(mustJSON(receipt), '\n')); err != nil {
			return nil, "", "", fmt.Errorf("app-server started but its process receipt is not durable: %w", err)
		}
		execution.Process = &identity
		execution.AppServerEndpoint = endpoint
		execution.AppServerGeneration = generation
		process = launched
	}
	client, err := connectWorkerAppServer(ctx, execution.AppServerEndpoint)
	if err != nil {
		return nil, "", "", err
	}
	closeClient := true
	defer func() {
		if closeClient {
			client.Close()
		}
	}()
	if execution.SessionID == "" {
		params, err := codexworker.ThreadParams(*run.WorktreePath, c.Sandbox, c.Model, []any{codexapp.AskDynamicTool()})
		if err != nil {
			return nil, "", "", err
		}
		response, err := codexworker.StartThread(ctx, client, params, *run.WorktreePath)
		if err != nil {
			return nil, "", "", err
		}
		if err := AtomicWrite(filepath.Join(serverDir, "thread.json"), append(mustJSON(response), '\n')); err != nil {
			return nil, "", "", err
		}
		execution.SessionID = response.Thread.ID
	} else {
		if err := codexworker.ResumeThread(ctx, client, execution.SessionID, *run.WorktreePath); err != nil {
			return nil, "", "", err
		}
	}
	binding := codexHistoryBinding{Version: 1, ProjectID: c.identity, TaskID: run.DomainTaskID, WorkflowID: run.RunID, ExecutionID: run.ExecutionID, AttemptID: run.AttemptID, Backend: BackendLocal, Protocol: ProtocolLocalCodexAppServer, TargetID: run.Target.ID, ExternalRef: conversationExternalRef(execution.Process), Workspace: *run.WorktreePath, SessionID: execution.SessionID, Command: codexReaderCommand(c), Home: codexEnvironmentHome()}
	if err := writeCodexHistoryBinding(localCodexHistoryBindingPath(dir, execution.SessionID), binding); err != nil {
		return nil, "", "", err
	}
	refreshExecutionView(run, execution)
	if persistThread == nil {
		return nil, "", "", errors.New("app-server thread cannot be sent without a durable execution receipt")
	}
	if err := persistThread(); err != nil {
		return nil, "", "", fmt.Errorf("app-server thread was created but its SQLite receipt is unverified: %w", err)
	}
	params, err := codexworker.TurnParams(ctx, codexworker.TurnParamsInput{
		Worktree: *run.WorktreePath, ThreadID: execution.SessionID, CommandID: commandID,
		Prompt: prompt, Sandbox: c.Sandbox, Model: c.Model, ReasoningEffort: c.ReasoningEffort,
	}, func(ctx context.Context, worktree string) ([]string, error) {
		return codexLinkedWorktreeWriteArgs(ctx, c, worktree)
	})
	if err != nil {
		return nil, "", "", err
	}
	turn, err := codexworker.StartTurn(ctx, client, params)
	if err != nil {
		return nil, "", "", err
	}
	if err := AtomicWrite(filepath.Join(dir, "turn-start.json"), append(mustJSON(turn), '\n')); err != nil {
		return nil, "", "", err
	}
	journal, err := os.OpenFile(journalPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, "", "", err
	}
	observeCtx, cancel := context.WithCancel(context.Background())
	worker := &Worker{proc: &durableWorkerProcess{identity: *execution.Process, process: process}, Events: make(chan WorkerEvent, 64), Done: make(chan error, 1), cancelObservation: cancel, observationDone: make(chan struct{})}
	go observeAppServerWorker(observeCtx, worker, client, journal, run.RunID, run.AttemptID, execution.SessionID, turn.Turn.ID, nil)
	closeClient = false
	return worker, journalPath, turn.Turn.ID, nil
}

func connectWorkerAppServer(ctx context.Context, endpoint string) (*codexapp.Client, error) {
	return codexworker.ConnectAppServer(ctx, endpoint)
}

func observeAppServerWorker(ctx context.Context, worker *Worker, client *codexapp.Client, journal *os.File, runID, attemptID, threadID, turnID string, replay []codexapp.Envelope) {
	defer close(worker.observationDone)
	defer close(worker.Events)
	defer client.Close()
	defer journal.Close()
	completed, err := codexworker.Observe(ctx, client, journal, threadID, turnID, replay, func(envelope codexapp.Envelope) error {
		_, err := parseCodexAskCall(envelope, threadID, turnID)
		return err
	}, func(observed codexworker.ObservationEvent) bool {
		event := WorkerEvent{
			RunID: runID, AttemptID: attemptID, Source: "codex-app-server", Session: threadID,
			Summary:  observed.Envelope.Method,
			Activity: ActivityEvent{Source: "codex", Kind: observed.Kind, Status: observed.Status, Title: observed.Title},
		}
		if observed.Forward {
			envelope := observed.Envelope
			event.AppServer = &envelope
		}
		return deliverDurableWorkerEvent(ctx, worker, event)
	})
	if err != nil || completed {
		worker.Done <- err
	}
}

func readAppServerTurnOutcome(path, threadID, turnID string) (codexworker.Outcome, error) {
	return codexworker.ReadAppServerOutcome(path, threadID, turnID)
}

// reattachAppServerWorker never sends a prompt. It first checks the original
// process and thread, then uses a native snapshot to settle a completed turn
// or subscribes to the still-running one. Unknown states remain untouched.
func reattachAppServerWorker(ctx context.Context, c Config, run *Run, execution *ExecutionRecord) (*Worker, bool, error) {
	if run == nil || execution == nil || run.WorktreePath == nil {
		return nil, false, errors.New("app-server recovery lacks execution/workspace")
	}
	serverDir := filepath.Join(c.RunDir, "runs", run.RunID, "app-server")
	var savedProcess appServerProcessReceipt
	processBytes, processErr := os.ReadFile(filepath.Join(serverDir, "process.json"))
	if processErr != nil || json.Unmarshal(processBytes, &savedProcess) != nil || savedProcess.Endpoint == "" || savedProcess.Generation == "" {
		return nil, false, errors.New("original app-server process receipt is missing or unreadable")
	}
	if execution.Process == nil {
		execution.Process = &savedProcess.Identity
		execution.AppServerEndpoint = savedProcess.Endpoint
		execution.AppServerGeneration = savedProcess.Generation
	}
	if *execution.Process != savedProcess.Identity || execution.AppServerEndpoint != savedProcess.Endpoint || execution.AppServerGeneration != savedProcess.Generation {
		return nil, false, errors.New("app-server process receipt differs from authoritative execution")
	}
	if execution.Process == nil || execution.AppServerEndpoint == "" || execution.AppServerGeneration == "" || inspectProcess(execution.Process) != "matching process still exists" {
		return nil, false, errors.New("original app-server process identity is unverified")
	}
	if execution.SessionID == "" {
		var receipt struct {
			Thread struct {
				ID  string `json:"id"`
				Cwd string `json:"cwd"`
			} `json:"thread"`
		}
		if raw, err := os.ReadFile(filepath.Join(serverDir, "thread.json")); err == nil && json.Unmarshal(raw, &receipt) == nil && codexThreadID.MatchString(receipt.Thread.ID) && canonical(receipt.Thread.Cwd) == canonical(*run.WorktreePath) {
			execution.SessionID = receipt.Thread.ID
		}
	}
	if !codexThreadID.MatchString(execution.SessionID) {
		return nil, false, errors.New("original app-server thread identity is unverified")
	}
	commandID := execution.CurrentCommandID
	turnID := execution.CurrentTurnID
	if execution.PendingCommandID != "" {
		commandID = execution.PendingCommandID
		turnID = execution.PendingTurnID
	}
	if !stableExecutionID(commandID) {
		return nil, false, errors.New("original app-server command identity is unverified")
	}
	dir := filepath.Join(c.RunDir, "runs", run.RunID, "turns", commandID)
	if !codexThreadID.MatchString(turnID) {
		var receipt struct {
			Turn struct {
				ID string `json:"id"`
			} `json:"turn"`
		}
		if raw, err := os.ReadFile(filepath.Join(dir, "turn-start.json")); err == nil && json.Unmarshal(raw, &receipt) == nil && codexThreadID.MatchString(receipt.Turn.ID) {
			turnID = receipt.Turn.ID
			execution.PendingTurnID = turnID
		}
	}
	if !codexThreadID.MatchString(turnID) {
		return nil, false, errors.New("original app-server native turn identity is unverified")
	}
	client, err := connectWorkerAppServer(ctx, execution.AppServerEndpoint)
	if err != nil {
		return nil, false, err
	}
	closeClient := true
	defer func() {
		if closeClient {
			client.Close()
		}
	}()
	nativeTurn, err := codexworker.ReadTurnSnapshot(ctx, client, execution.SessionID, *run.WorktreePath, turnID)
	if err != nil {
		return nil, false, err
	}
	journalPath := filepath.Join(dir, "appserver.jsonl")
	journal, err := os.OpenFile(journalPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, false, err
	}
	if nativeTurn.CompletedAt != nil && nativeTurn.Status != "inProgress" {
		start := codexapp.Envelope{Method: "turn/started", Params: mustJSON(map[string]any{"threadId": execution.SessionID, "turn": map[string]any{"id": turnID}})}
		completed := codexapp.Envelope{Method: "turn/completed", Params: mustJSON(map[string]any{"threadId": execution.SessionID, "turn": nativeTurn})}
		if _, err = journal.Write(append(mustJSON(start), '\n')); err == nil {
			_, err = journal.Write(append(mustJSON(completed), '\n'))
		}
		if err == nil {
			err = journal.Sync()
		}
		_ = journal.Close()
		if err != nil {
			return nil, false, err
		}
		return nil, true, nil
	}
	if nativeTurn.Status != "inProgress" {
		_ = journal.Close()
		return nil, false, fmt.Errorf("native turn has unsupported status %q", nativeTurn.Status)
	}
	process, err := os.FindProcess(execution.Process.PID)
	if err != nil {
		_ = journal.Close()
		return nil, false, err
	}
	observeCtx, cancel := context.WithCancel(context.Background())
	worker := &Worker{proc: &durableWorkerProcess{identity: *execution.Process, process: process}, Events: make(chan WorkerEvent, 64), Done: make(chan error, 1), cancelObservation: cancel, observationDone: make(chan struct{})}
	replay, err := appServerPendingAskEvents(journalPath, run, execution, turnID)
	if err != nil {
		_ = journal.Close()
		return nil, false, err
	}
	go observeAppServerWorker(observeCtx, worker, client, journal, run.RunID, run.AttemptID, execution.SessionID, turnID, replay)
	closeClient = false
	return worker, false, nil
}
