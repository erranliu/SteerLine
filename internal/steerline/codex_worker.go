package steerline

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

func appServerAskDecision(run *Run, call codexAskCall) (*DecisionState, error) {
	return WriteDecisionRequest(run, codexworker.AskDecisionRequest(call))
}

func appServerDirectory(c Config, run *Run) string {
	if run.DomainAttemptNumber > 1 {
		return filepath.Join(c.RunDir, "runs", run.RunID, "attempts", run.AttemptID, "app-server")
	}
	return filepath.Join(c.RunDir, "runs", run.RunID, "app-server")
}

// Native start notifications can be replayed after recovery. Only the first
// confirmed start consumes a pending Command and accepts post-turn feedback.
func (s *Service) recordAppServerTurnStarted(run *Run, workflow *WorkflowRunRecord, execution *ExecutionRecord, turnID string) (string, error) {
	commandID := execution.PendingCommandID
	fresh := commandID != ""
	if !fresh && execution.CurrentTurnID == turnID && execution.LastStoppedTurnID != turnID {
		commandID = execution.CurrentCommandID
	}
	if !stableExecutionID(commandID) || !codexworker.ValidNativeID(turnID) || fresh && execution.PendingTurnID != turnID {
		return "", errors.New("app-server turn start lacks its fixed command or native turn receipt")
	}
	var operation *OperationRecord
	for i := range s.domain.Operations {
		if s.domain.Operations[i].IdempotencyKey == "worker-turn:"+run.ExecutionID+":"+commandID {
			operation = &s.domain.Operations[i]
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

func codexBackend(c Config, run *Run) codexworker.Backend {
	return codexworker.Backend{Config: codexworker.Config{Executable: c.Codex.Executable, Args: c.Codex.Args, Profile: c.Profile, Sandbox: c.Sandbox, WindowsSandbox: c.WindowsSandbox, Model: c.Model, ReasoningEffort: c.ReasoningEffort, Workspace: deref(run.WorktreePath), RunDir: c.RunDir, RunID: run.RunID, ServerDir: appServerDirectory(c, run), Environment: func(base []string) []string {
		return unsetEnv(overrideEnv(base, map[string]string{"STEERLINE_WORKFLOW_RUN_ID": run.RunID, "STEERLINE_EXECUTION_ID": run.ExecutionID}), []string{"BEADS_*", "BD_*"})
	}}, Process: codexworker.ProcessPort{Start: codexProcessStart, Inspect: inspectProcess, NewGeneration: newID, WriteAtomic: AtomicWrite}}
}
func codexBinding(e *ExecutionRecord) codexworker.Binding {
	return codexworker.Binding{Process: e.Process, Endpoint: e.AppServerEndpoint, Generation: e.AppServerGeneration, SessionID: e.SessionID}
}
func applyCodexBinding(e *ExecutionRecord, b codexworker.Binding) {
	e.Process = b.Process
	e.AppServerEndpoint = b.Endpoint
	e.AppServerGeneration = b.Generation
	e.SessionID = b.SessionID
}
func startAppServerWorkerTurn(ctx context.Context, c Config, run *Run, execution *ExecutionRecord, commandID, prompt string, persistThread func() error) (*Worker, string, string, error) {
	if run == nil || execution == nil || run.WorkflowVersion != WorkerFlowVersion || run.Protocol != string(executionapi.ProtocolLocalCodexAppServer) || run.WorktreePath == nil || run.AttemptID == "" {
		return nil, "", "", errors.New("app-server worker turn lacks its fixed execution, workspace or command")
	}
	if run.ExecutionID != execution.ExecutionID || run.RunID != execution.WorkflowRunID || canonical(*run.WorktreePath) != canonical(execution.Workspace.Worktree) {
		return nil, "", "", errors.New("app-server execution/workspace identity mismatch")
	}
	backend := codexBackend(c, run)
	input := executionapi.TurnInput{CommandID: commandID, Prompt: prompt}
	session, binding, err := backend.PrepareSession(ctx, codexBinding(execution), input)
	applyCodexBinding(execution, binding)
	if err != nil {
		return nil, "", "", err
	}
	defer session.Close()
	history := codexworker.HistoryBinding{Version: 1, ProjectID: c.identity, TaskID: run.DomainTaskID, WorkflowID: run.RunID, ExecutionID: run.ExecutionID, AttemptID: run.AttemptID, Backend: executionapi.BackendLocal, Protocol: executionapi.ProtocolLocalCodexAppServer, TargetID: run.Target.ID, ExternalRef: conversationExternalRef(execution.Process), Workspace: *run.WorktreePath, SessionID: execution.SessionID, Command: codexReaderCommand(c), Home: codexEnvironmentHome()}
	if err = writeCodexHistoryBinding(localCodexHistoryBindingPath(session.Paths.TurnDir, execution.SessionID), history); err != nil {
		return nil, "", "", err
	}
	refreshExecutionView(run, execution)
	if persistThread == nil {
		return nil, "", "", errors.New("app-server thread cannot be sent without a durable execution receipt")
	}
	if err = persistThread(); err != nil {
		return nil, "", "", fmt.Errorf("app-server thread was created but its SQLite receipt is unverified: %w", err)
	}
	var linkedArgs []string
	if c.Sandbox == "workspace-write" {
		linkedArgs, err = codexLinkedWorktreeWriteArgs(ctx, c, *run.WorktreePath)
		if err != nil {
			return nil, "", "", err
		}
	}
	observer, turnID, err := backend.StartTurn(ctx, session, input, linkedArgs)
	if err != nil {
		return nil, "", turnID, err
	}
	return attachCodexObservation(observer, run.RunID, run.AttemptID, execution.SessionID), session.Paths.JournalPath, turnID, nil
}

func (s *Service) interruptOwnedLocalTurn(runID string, owned *childProcess) {
	for _, run := range s.runs {
		if run == nil || run.RunID != runID || run.Protocol != string(executionapi.ProtocolLocalCodexAppServer) {
			continue
		}
		execution := s.executionRecord(run)
		if execution == nil || execution.Process == nil || *execution.Process != owned.Identity() ||
			!codexworker.ValidNativeID(execution.SessionID) || !codexworker.ValidNativeID(execution.CurrentTurnID) ||
			execution.CurrentTurnID == execution.LastStoppedTurnID || execution.AppServerEndpoint == "" {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = codexworker.Interrupt(ctx, execution.AppServerEndpoint, execution.SessionID, execution.CurrentTurnID)
		cancel()
		return
	}
}

// attachCodexObservation maps typed platform output into the existing center
// event queue. The adapter owns cancellation, protocol I/O and journal files.
func attachCodexObservation(observer *codexworker.Observation, runID, attemptID, threadID string) *Worker {
	ctx, cancel := context.WithCancel(context.Background())
	worker := &Worker{proc: observer.Process, Events: make(chan WorkerEvent, 64), Done: make(chan error, 1), cancelObservation: func() { cancel(); observer.Cancel() }, observationDone: make(chan struct{})}
	go func() {
		defer close(worker.observationDone)
		defer func() { observer.Cancel(); <-observer.Finished; cancel() }()
		defer close(worker.Events)
		for observed := range observer.Events {
			event := WorkerEvent{RunID: runID, AttemptID: attemptID, Source: "codex-app-server", Session: threadID, Summary: observed.Summary, Activity: ActivityEvent{Source: "codex", Kind: observed.Kind, Status: observed.Status, Title: observed.Title}}
			if observed.Forward {
				envelope := observed.Envelope
				event.AppServer = &envelope
			}
			accepted := deliverDurableWorkerEvent(ctx, worker, event)
			if accepted {
				observed.Persisted <- nil
			} else {
				observed.Persisted <- errors.New("Codex event persistence failed")
				observer.Cancel()
				return
			}
		}
		if err, ok := <-observer.Done; ok {
			worker.Done <- err
		}
	}()
	return worker
}
func readAppServerTurnOutcome(path, threadID, turnID string) (codexworker.Outcome, error) {
	return codexworker.ReadAppServerOutcome(path, threadID, turnID)
}
func knownCodexAsks(run *Run, e *ExecutionRecord) []string {
	var calls []string
	if run.Decision != nil && run.Decision.Request.NativeAsk != nil {
		calls = append(calls, run.Decision.Request.NativeAsk.CallID)
	}
	for _, call := range e.PendingAsks {
		calls = append(calls, call.CallID)
	}
	return calls
}
func applyCodexRecovery(run *Run, e *ExecutionRecord, result codexworker.Recovery) *Worker {
	applyCodexBinding(e, result.Binding)
	if e.PendingCommandID != "" {
		e.PendingTurnID = result.TurnID
	}
	if result.Observer != nil {
		return attachCodexObservation(result.Observer, run.RunID, run.AttemptID, e.SessionID)
	}
	return nil
}
func reattachAppServerWorker(ctx context.Context, c Config, run *Run, e *ExecutionRecord) (*Worker, bool, error) {
	if run == nil || e == nil || run.WorktreePath == nil {
		return nil, false, errors.New("app-server recovery lacks execution/workspace")
	}
	commandID, turnID := e.CurrentCommandID, e.CurrentTurnID
	if e.PendingCommandID != "" {
		commandID, turnID = e.PendingCommandID, e.PendingTurnID
	}
	result, err := codexBackend(c, run).Reattach(ctx, codexBinding(e), commandID, turnID, knownCodexAsks(run, e))
	return applyCodexRecovery(run, e, result), result.Completed, err
}

// Network I/O belongs to the Execution. Only handleAppServerLaunchMessage
// writes the service projection and SQLite, including the mandatory receipt
// barrier between thread/start and turn/start.
func (s *Service) launchAppServerTurn(ctx context.Context, config Config, run Run, execution ExecutionRecord, commandID, prompt, opID string) {
	timeout := positiveDuration(config.CommandTimeout, 30*time.Second)
	if timeout > 30*time.Second {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	persistThread := func() error {
		ack := make(chan error, 1)
		message := &appServerLaunchMessage{phase: "thread_ready", commandID: commandID, opID: opID, run: run, execution: execution, ack: ack}
		select {
		case s.workerMessages <- workerMessage{runID: run.RunID, launch: message}:
		case <-ctx.Done():
			return ctx.Err()
		}
		select {
		case err := <-ack:
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	worker, events, turnID, err := startAppServerWorkerTurn(ctx, config, &run, &execution, commandID, prompt, persistThread)
	message := &appServerLaunchMessage{phase: "turn_result", commandID: commandID, opID: opID, run: run, execution: execution, worker: worker, events: events, turnID: turnID, err: err}
	select {
	case s.workerMessages <- workerMessage{runID: run.RunID, launch: message}:
	case <-s.runContext.Done():
		if worker != nil {
			_ = worker.detachLocalObservation(context.Background())
		}
	}
}

func (s *Service) handleAppServerLaunchMessage(runID string, message *appServerLaunchMessage) (retErr error) {
	if message == nil || message.commandID == "" || message.opID == "" {
		return errors.New("app-server launch message lacks its fixed command")
	}
	defer func() {
		if message.phase == "turn_result" && s.appServerLaunching[runID] == message.commandID {
			delete(s.appServerLaunching, runID)
		}
		if message.worker != nil && s.workers[runID] != message.worker {
			_ = message.worker.detachLocalObservation(context.Background())
		}
	}()
	if message.ack != nil {
		defer func() { message.ack <- retErr }()
	}
	var run *Run
	for _, candidate := range s.runs {
		if candidate != nil && candidate.RunID == runID {
			run = candidate
			break
		}
	}
	if run == nil || run.Protocol != string(executionapi.ProtocolLocalCodexAppServer) {
		return errors.New("app-server launch no longer belongs to a local execution")
	}
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return err
	}
	c := fixed.config
	workflow, execution := s.workflowRecords(run)
	if workflow == nil || execution == nil || execution.ExecutionID != message.execution.ExecutionID || execution.PendingCommandID != message.commandID || run.AttemptID != message.run.AttemptID || run.TaskStatus == "closed" || run.TaskStatus == "discarded" {
		return ErrWorkerTurnStale
	}
	if message.phase == "thread_ready" {
		if message.execution.Process == nil || !codexworker.ValidNativeID(message.execution.SessionID) || message.execution.AppServerEndpoint == "" || message.execution.AppServerGeneration == "" {
			return errors.New("app-server thread receipt is incomplete")
		}
		if execution.Process != nil && *execution.Process != *message.execution.Process || execution.SessionID != "" && execution.SessionID != message.execution.SessionID {
			return errors.New("app-server thread receipt conflicts with its fixed execution")
		}
		ref := executionapi.ExecutionRef{Version: executionapi.ExecutionRecordVersion, Backend: executionapi.BackendLocal, Protocol: executionapi.ProtocolLocalCodexAppServer, Target: targetFor(c), Agent: agentFor(c), RunID: run.RunID, AttemptID: run.AttemptID, WorkspacePath: deref(run.WorktreePath), WorkspaceBranch: deref(run.WorktreeBranch), BaseCommit: deref(run.BaseCommit), External: *message.execution.Process, Generation: message.execution.AppServerGeneration}
		execution.Process = message.execution.Process
		execution.AppServerEndpoint = message.execution.AppServerEndpoint
		execution.AppServerGeneration = message.execution.AppServerGeneration
		execution.SessionID = message.execution.SessionID
		s.refreshRun(run)
		return s.commitWorkerBinding(run, ref)
	}
	if message.phase != "turn_result" {
		return fmt.Errorf("unknown app-server launch phase %q", message.phase)
	}
	if message.err != nil {
		if message.execution.Process != nil {
			if execution.Process != nil && *execution.Process != *message.execution.Process ||
				execution.AppServerEndpoint != "" && execution.AppServerEndpoint != message.execution.AppServerEndpoint ||
				execution.AppServerGeneration != "" && execution.AppServerGeneration != message.execution.AppServerGeneration ||
				execution.SessionID != "" && message.execution.SessionID != "" && execution.SessionID != message.execution.SessionID {
				return ErrWorkerTurnStale
			}
			if message.execution.SessionID != "" && !codexworker.ValidNativeID(message.execution.SessionID) {
				return errors.New("partial app-server thread identity is invalid")
			}
			execution.Process = message.execution.Process
			execution.AppServerEndpoint = message.execution.AppServerEndpoint
			execution.AppServerGeneration = message.execution.AppServerGeneration
			if message.execution.SessionID != "" {
				execution.SessionID = message.execution.SessionID
			}
			s.refreshRun(run)
		}
		return s.failStartedWorkerTurn(run, message.opID, message.turnID, message.err)
	}
	if message.worker == nil || !codexworker.ValidNativeID(message.turnID) || execution.Process == nil || message.worker.proc.Identity() != *execution.Process || message.execution.SessionID != execution.SessionID || message.execution.AppServerGeneration != execution.AppServerGeneration {
		return errors.New("app-server turn result differs from its fixed thread/process")
	}
	return s.acceptStartedWorkerTurn(run, message.worker, message.events, message.turnID, message.opID, message.commandID, true)
}
