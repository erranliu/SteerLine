package steerline

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

func appServerAskDecision(run *Run, questions []executionapi.Question, route executionapi.QuestionRoute) (*DecisionState, error) {
	return WriteDecisionRequest(run, decisionRequestFromEventFact(executionapi.EventFact{Questions: questions, QuestionRoute: &route}))
}

func appServerDirectory(c Config, run *Run) string {
	if run.DomainAttemptNumber > 1 {
		return filepath.Join(c.RunDir, "runs", run.RunID, "attempts", run.AttemptID, "app-server")
	}
	return filepath.Join(c.RunDir, "runs", run.RunID, "app-server")
}

func codexBackend(c Config, run *Run) codexworker.Backend {
	settings := codexProfileSettings(c)
	permissions, _ := codexworker.CompilePolicy(c.ExecutionPolicy, runtime.GOOS, c.hostName(), c.agentName(), settings.WindowsSandbox, settings.Profile, settings.Args)
	return codexworker.Backend{Config: codexworker.Config{Policy: c.ExecutionPolicy, Executable: settings.Executable, Args: settings.Args, Profile: settings.Profile, Sandbox: permissions.Sandbox, WindowsSandbox: settings.WindowsSandbox, Model: settings.Model, ReasoningEffort: settings.ReasoningEffort, Workspace: deref(run.WorktreePath), RunDir: c.RunDir, RunID: run.RunID, ServerDir: appServerDirectory(c, run), Environment: func(base []string) []string {
		return workerProcessEnvironment(c, run, base)
	}}, Process: codexworker.ProcessPort{Start: codexProcessStart, Inspect: inspectProcess, NewGeneration: newID, WriteAtomic: AtomicWrite}}
}
func workerProcessEnvironment(c Config, run *Run, base []string) []string {
	blocked := append(c.publicationSettings().AuthenticationRefs, "BEADS_*", "BD_*", "GH_TOKEN", "GITHUB_TOKEN", "GITEA_TOKEN", "GIT_CONFIG_COUNT", "GIT_CONFIG_KEY_*", "GIT_CONFIG_VALUE_*")
	return unsetEnv(overrideEnv(base, map[string]string{"STEERLINE_WORKFLOW_RUN_ID": run.RunID, "STEERLINE_EXECUTION_ID": run.ExecutionID}), blocked)
}
func codexBinding(e *ExecutionRecord) (codexworker.Binding, error) {
	if e == nil {
		return codexworker.Binding{}, errors.New("app-server binding has no execution")
	}
	native, err := decodeCodexAppServerBinding(e)
	if err != nil {
		return codexworker.Binding{}, err
	}
	return codexworker.Binding{Process: e.Process, Endpoint: native.Endpoint, Generation: native.Generation, SessionID: native.SessionID}, nil
}
func applyCodexBinding(e *ExecutionRecord, b codexworker.Binding) error {
	e.Process = b.Process
	return storeCodexAppServerBinding(e, b)
}
func startAppServerWorkerTurn(ctx context.Context, c Config, run *Run, execution *ExecutionRecord, commandID, prompt string, persistThread func() error) (*Worker, string, string, error) {
	if run == nil || execution == nil || run.WorkflowVersion != WorkerFlowVersion || run.Protocol != string(executionapi.ProtocolLocalCodexAppServer) || run.WorktreePath == nil || run.AttemptID == "" {
		return nil, "", "", errors.New("app-server worker turn lacks its fixed execution, workspace or command")
	}
	if run.ExecutionID != execution.ExecutionID || run.RunID != execution.WorkflowRunID || canonical(*run.WorktreePath) != canonical(execution.Workspace.Worktree) {
		return nil, "", "", errors.New("app-server execution/workspace identity mismatch")
	}
	var linkedArgs []string
	var err error
	if c.ExecutionPolicy.Filesystem.WriteScope == "workspace-write" {
		linkedArgs, err = codexLinkedWorktreeWriteArgs(ctx, c, *run.WorktreePath)
		if err != nil {
			return nil, "", "", err
		}
	}
	if err = bindPolicyResources(ctx, c, execution, *run.WorktreePath, linkedArgs); err != nil {
		return nil, "", "", err
	}
	backend := codexBackend(c, run)
	input := executionapi.TurnInput{CommandID: commandID, Prompt: prompt}
	fixedBinding, err := codexBinding(execution)
	if err != nil {
		return nil, "", "", err
	}
	session, binding, err := backend.PrepareSession(ctx, fixedBinding, input)
	if bindingErr := applyCodexBinding(execution, binding); bindingErr != nil {
		return nil, "", "", bindingErr
	}
	if err != nil {
		return nil, "", "", err
	}
	defer session.Close()
	history := codexworker.HistoryBinding{Version: 1, ProjectID: c.identity, TaskID: run.DomainTaskID, WorkflowID: run.RunID, ExecutionID: run.ExecutionID, AttemptID: run.AttemptID, Backend: executionapi.BackendLocal, Protocol: executionapi.ProtocolLocalCodexAppServer, TargetID: run.Target.ID, ExternalRef: conversationExternalRef(execution.Process), Workspace: *run.WorktreePath, SessionID: binding.SessionID, Command: codexReaderCommand(c), Home: codexEnvironmentHome()}
	if err = writeCodexHistoryBinding(localCodexHistoryBindingPath(session.Paths.TurnDir, binding.SessionID), history); err != nil {
		return nil, "", "", err
	}
	refreshExecutionView(run, execution)
	if persistThread == nil {
		return nil, "", "", errors.New("app-server thread cannot be sent without a durable execution receipt")
	}
	if err = persistThread(); err != nil {
		return nil, "", "", fmt.Errorf("app-server thread was created but its SQLite receipt is unverified: %w", err)
	}

	observer, turnID, err := backend.StartTurn(ctx, session, input, linkedArgs)
	if err != nil {
		return nil, "", turnID, err
	}
	fixed := workerSessionContext(run, execution)
	fixed.CurrentTurnID, fixed.PendingTurnID = turnID, turnID
	return attachCodexObservation(observer, run.RunID, run.AttemptID, *fixed), session.Paths.JournalPath, turnID, nil
}

func (s *Service) interruptOwnedLocalTurn(runID string, owned *childProcess) {
	for _, run := range s.runs {
		if run == nil || run.RunID != runID || run.Protocol != string(executionapi.ProtocolLocalCodexAppServer) {
			continue
		}
		execution := s.executionRecord(run)
		if execution == nil || execution.Process == nil || *execution.Process != owned.Identity() ||
			!codexworker.ValidNativeID(codexSessionID(execution)) || !codexworker.ValidNativeID(execution.CurrentTurnID) ||
			execution.CurrentTurnID == execution.LastStoppedTurnID {
			return
		}
		binding, err := codexBinding(execution)
		if err != nil || binding.Endpoint == "" {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = codexworker.Interrupt(ctx, binding.Endpoint, binding.SessionID, execution.CurrentTurnID)
		cancel()
		return
	}
}

// attachCodexObservation maps typed platform output into the existing center
// event queue. The adapter owns cancellation, protocol I/O and journal files.
func attachCodexObservation(observer *codexworker.Observation, runID, attemptID string, fixed executionapi.SessionContext) *Worker {
	ctx, cancel := context.WithCancel(context.Background())
	worker := &Worker{proc: observer.Process, Events: make(chan WorkerEvent, 64), Done: make(chan error, 1), cancelObservation: func() { cancel(); observer.Cancel() }, observationDone: make(chan struct{})}
	go func() {
		defer close(worker.observationDone)
		defer func() { observer.Cancel(); <-observer.Finished; cancel() }()
		defer close(worker.Events)
		for observed := range observer.Events {
			event := WorkerEvent{RunID: runID, AttemptID: attemptID, Source: "codex-app-server", Summary: observed.Summary, Activity: ActivityEvent{Source: "codex", Kind: observed.Kind, Status: observed.Status, Title: observed.Title}}
			if observed.Forward {
				fact, err := codexworker.DecodeEvent(observed.Envelope, fixed, true)
				if err != nil {
					observed.Persisted <- err
					observer.Cancel()
					return
				}
				event.Fact = fact.EventFact
				event.Fact.Source = event.Source
				event.Fact.SourceEventID = observed.Envelope.Method + ":" + digestValue(observed.Envelope)
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
	if run.Decision != nil && run.Decision.Request.QuestionRoute != nil {
		calls = append(calls, run.Decision.Request.QuestionRoute.QuestionID)
	}
	for _, question := range e.PendingQuestions {
		calls = append(calls, question.Route.QuestionID)
	}
	return calls
}
func applyCodexRecovery(run *Run, e *ExecutionRecord, result codexworker.Recovery) *Worker {
	if err := applyCodexBinding(e, result.Binding); err != nil {
		return nil
	}
	if e.PendingCommandID != "" {
		e.PendingTurnID = result.TurnID
	}
	if result.Observer != nil {
		fixed := workerSessionContext(run, e)
		fixed.CurrentTurnID, fixed.PendingTurnID = result.TurnID, result.TurnID
		return attachCodexObservation(result.Observer, run.RunID, run.AttemptID, *fixed)
	}
	return nil
}
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
	messageBinding, err := codexBinding(&message.execution)
	if err != nil {
		return err
	}
	currentBinding, err := codexBinding(execution)
	if err != nil {
		return err
	}
	acceptResources := func() error {
		resource := message.execution.PolicyResources
		if err := verifyPolicyResources(resource, workflow.ExecutionProfile.Digest, execution.Workspace.Worktree, &message.execution); err != nil {
			return err
		}
		if execution.PolicyResources != nil && execution.PolicyResources.Digest != resource.Digest {
			return ErrWorkerTurnStale
		}
		execution.PolicyResources = resource
		execution.Generation = message.execution.Generation
		execution.ConfigurationDigest = message.execution.ConfigurationDigest
		return nil
	}
	if message.phase == "thread_ready" {
		if message.execution.Process == nil || !codexworker.ValidNativeID(messageBinding.SessionID) || messageBinding.Endpoint == "" || messageBinding.Generation == "" {
			return errors.New("app-server thread receipt is incomplete")
		}
		if execution.Process != nil && *execution.Process != *message.execution.Process || currentBinding.SessionID != "" && currentBinding.SessionID != messageBinding.SessionID {
			return errors.New("app-server thread receipt conflicts with its fixed execution")
		}
		if err := acceptResources(); err != nil {
			return err
		}
		ref := executionapi.ExecutionRef{Version: executionapi.ExecutionRecordVersion, Backend: executionapi.BackendLocal, Protocol: executionapi.ProtocolLocalCodexAppServer, Target: targetFor(c), Agent: agentFor(c), RunID: run.RunID, AttemptID: run.AttemptID, WorkspacePath: deref(run.WorktreePath), WorkspaceBranch: deref(run.WorktreeBranch), BaseCommit: deref(run.BaseCommit), External: *message.execution.Process, Generation: messageBinding.Generation}
		if err := applyCodexBinding(execution, messageBinding); err != nil {
			return err
		}
		s.refreshRun(run)
		return s.commitWorkerBinding(run, ref)
	}
	if message.phase != "turn_result" {
		return fmt.Errorf("unknown app-server launch phase %q", message.phase)
	}
	if message.err != nil {
		if message.execution.Process != nil {
			if execution.Process != nil && *execution.Process != *message.execution.Process ||
				currentBinding.Endpoint != "" && currentBinding.Endpoint != messageBinding.Endpoint ||
				currentBinding.Generation != "" && currentBinding.Generation != messageBinding.Generation ||
				currentBinding.SessionID != "" && messageBinding.SessionID != "" && currentBinding.SessionID != messageBinding.SessionID {
				return ErrWorkerTurnStale
			}
			if messageBinding.SessionID != "" && !codexworker.ValidNativeID(messageBinding.SessionID) {
				return errors.New("partial app-server thread identity is invalid")
			}
			if err := acceptResources(); err != nil {
				return err
			}
			if messageBinding.Endpoint != "" {
				if err := applyCodexBinding(execution, messageBinding); err != nil {
					return err
				}
			} else {
				execution.Process = message.execution.Process
			}
			s.refreshRun(run)
		}
		return s.failStartedWorkerTurn(run, message.opID, message.turnID, message.err)
	}
	if message.worker == nil || !codexworker.ValidNativeID(message.turnID) || execution.Process == nil || message.worker.proc.Identity() != *execution.Process || messageBinding.SessionID != currentBinding.SessionID || messageBinding.Generation != currentBinding.Generation {
		return errors.New("app-server turn result differs from its fixed thread/process")
	}
	return s.acceptStartedWorkerTurn(run, message.worker, message.events, message.turnID, message.opID, message.commandID, true)
}
