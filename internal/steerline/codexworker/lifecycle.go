package codexworker

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

// ExecutionLifecycle owns app-server session setup, turn input, observation,
// and reconciliation. Every mutating call receives one center-persisted
// operation; the returned binding must be committed before another step.
type ExecutionLifecycle struct {
	Backend        Backend
	HistoryCommand contracts.Command
	ProbeLifecycle func(context.Context, executionapi.LifecycleSetup) (executionapi.CapabilityReport, error)
}

func (l ExecutionLifecycle) PolicyCapabilities() executionapi.LifecyclePolicyCapabilities {
	return executionapi.LifecyclePolicyCapabilities{LinkedWorkspaceWriteRoots: true}
}

func NewExecutionLifecycle(setup executionapi.LifecycleSetup) (executionapi.LifecyclePort, error) {
	setup = setup.Clone()
	snapshot := setup.Snapshot
	if snapshot.Ref.Backend != executionapi.BackendLocal || snapshot.Ref.Protocol != executionapi.ProtocolLocalCodexAppServer || snapshot.Session.Agent.Name != "codex" || setup.ProcessHost == nil {
		return nil, &executionapi.BackendError{Code: executionapi.BackendUnsupported, Diagnostic: "local app-server lifecycle setup differs from its fixed implementation target"}
	}
	profile, err := DecodeProfileConfigEnvelope(setup.ProfileData, ProfileConfigIdentity{
		ProjectID: snapshot.Session.ProjectID, ProfileID: setup.ProfileID, AcceptedConfigVersion: setup.AcceptedConfigVersion,
	})
	if err != nil {
		return nil, err
	}
	permissions, err := CompilePolicy(setup.Policy, runtime.GOOS, "local", "codex", profile.WindowsSandbox, profile.Profile, profile.Args)
	if err != nil {
		return nil, err
	}
	serverDir := filepath.Join(setup.RunDirectory, "runs", snapshot.Session.WorkflowRunID, "app-server")
	if setup.AttemptNumber > 1 {
		serverDir = filepath.Join(setup.RunDirectory, "runs", snapshot.Session.WorkflowRunID, "attempts", snapshot.Session.AttemptID, "app-server")
	}
	processEnv := func(base []string) []string {
		return lifecycleProcessEnvironment(base, snapshot.Session.WorkflowRunID, snapshot.Session.ExecutionID, setup.BlockedEnvironment)
	}
	backend := Backend{
		Config: Config{
			Policy: setup.Policy, Executable: profile.Executable, Args: append([]string(nil), profile.Args...),
			Profile: profile.Profile, Sandbox: permissions.Sandbox, WindowsSandbox: permissions.WindowsSandbox,
			Model: profile.Model, ReasoningEffort: profile.ReasoningEffort, Workspace: snapshot.Ref.WorkspacePath,
			RunDir: setup.RunDirectory, RunID: snapshot.Session.WorkflowRunID, ServerDir: serverDir,
			Environment: processEnv,
		},
		Process: ProcessPort{
			Start: setup.ProcessHost.StartManaged, Inspect: setup.ProcessHost.InspectProcess,
			NewGeneration: setup.ProcessHost.NewProcessGeneration, WriteAtomic: setup.ProcessHost.WriteExecutionFile,
		},
	}
	if err := backend.Config.Validate(runtime.GOOS); err != nil {
		return nil, err
	}
	history := ReaderCommand(contracts.Command{Executable: profile.Executable, Args: append([]string(nil), profile.Args...)}, profile.Profile)
	return ExecutionLifecycle{Backend: backend, HistoryCommand: history, ProbeLifecycle: func(ctx context.Context, probeSetup executionapi.LifecycleSetup) (executionapi.CapabilityReport, error) {
		probeSetup.ProcessHost = setup.ProcessHost
		return probeLifecycleCapabilities(ctx, backend, history, probeSetup)
	}}, nil
}

func lifecycleProcessEnvironment(base []string, runID, executionID string, blocked []string) []string {
	values := []string{"STEERLINE_WORKFLOW_RUN_ID=" + runID, "STEERLINE_EXECUTION_ID=" + executionID}
	merged := make([]string, 0, len(base)+len(values))
	for _, item := range base {
		key, _, found := strings.Cut(item, "=")
		if !found {
			continue
		}
		key = strings.ToUpper(key)
		if key == "STEERLINE_WORKFLOW_RUN_ID" || key == "STEERLINE_EXECUTION_ID" || blockedEnvironmentKey(key, blocked) {
			continue
		}
		merged = append(merged, item)
	}
	return append(merged, values...)
}

func blockedEnvironmentKey(key string, blocked []string) bool {
	for _, pattern := range blocked {
		pattern = strings.ToUpper(pattern)
		if strings.HasSuffix(pattern, "*") {
			if strings.HasPrefix(key, strings.TrimSuffix(pattern, "*")) {
				return true
			}
		} else if key == pattern {
			return true
		}
	}
	return false
}

func probeLifecycleCapabilities(ctx context.Context, backend Backend, history contracts.Command, setup executionapi.LifecycleSetup) (executionapi.CapabilityReport, error) {
	probe := ProbePort{
		Output: func(ctx context.Context, executable string, args []string) ([]byte, error) {
			return setup.ProcessHost.OutputCommand(ctx, executionapi.LaunchCommand{Executable: executable, Args: append([]string(nil), args...), WorkingDirectory: setup.RepositoryPath})
		},
		HistoryAvailable: func(ctx context.Context) bool {
			client, err := OpenHistory(ctx, history, setup.RepositoryPath, "", setup.ProcessHost.StartManaged)
			if err != nil {
				return false
			}
			client.Close()
			return true
		},
		StartManaged: func(command *exec.Cmd) (func(), error) {
			process, err := setup.ProcessHost.StartManaged(command)
			if err != nil {
				return nil, err
			}
			return process.Close, nil
		},
	}
	err := ProbeCapabilities(ctx, ProbeConfig{Executable: backend.Config.Executable, Args: backend.Config.Args, Repository: setup.RepositoryPath, RunDir: setup.RunDirectory}, probe)
	status := executionapi.CapabilityNotTested
	if err == nil {
		status = executionapi.CapabilityVerified
	}
	report := executionapi.CapabilityReport{
		Version: 1, Backend: setup.Snapshot.Ref.Backend, Protocol: setup.Snapshot.Ref.Protocol,
		Target: setup.Snapshot.Ref.Target, Agent: setup.Snapshot.Ref.Agent, CheckedAt: time.Now().UTC(),
		Capabilities: []executionapi.Capability{{Name: "start", Status: status}, {Name: "observe", Status: status}, {Name: "operation_reconciliation", Status: status}},
	}
	return report, err
}

func (l ExecutionLifecycle) PreflightLifecycle(ctx context.Context, setup executionapi.LifecycleSetup) (executionapi.CapabilityReport, error) {
	if l.ProbeLifecycle == nil {
		return executionapi.CapabilityReport{}, &executionapi.BackendError{Code: executionapi.BackendUnsupported, Diagnostic: "lifecycle capability probe is not configured"}
	}
	return l.ProbeLifecycle(ctx, setup.Clone())
}

func (l ExecutionLifecycle) Next(_ context.Context, snapshot executionapi.LifecycleSnapshot) (executionapi.LifecycleCandidate, error) {
	if snapshot.PendingTurn == nil {
		return executionapi.LifecycleCandidate{Done: true}, nil
	}
	if snapshot.PendingTurn.CommandID == "" || snapshot.PendingTurn.Prompt == "" {
		return executionapi.LifecycleCandidate{}, errors.New("app-server lifecycle has no saved turn input")
	}
	binding, found, err := l.readBinding(snapshot)
	if err != nil {
		return executionapi.LifecycleCandidate{}, err
	}
	if !found || binding.Endpoint == "" || binding.Generation == "" {
		candidate := executionapi.LifecycleCandidate{Action: executionapi.LifecyclePrepareSession, Input: executionapi.LifecycleInput{Turn: cloneTurn(snapshot.PendingTurn)}}
		return candidate, candidate.Validate()
	}
	candidate := executionapi.LifecycleCandidate{Action: executionapi.LifecycleStartTurn, Input: executionapi.LifecycleInput{Turn: cloneTurn(snapshot.PendingTurn)}}
	return candidate, candidate.Validate()
}

func (l ExecutionLifecycle) Execute(ctx context.Context, request executionapi.LifecycleRequest) (executionapi.LifecycleReceipt, executionapi.LifecycleObserver, error) {
	if err := request.Validate(); err != nil {
		return executionapi.LifecycleReceipt{Operation: request.Operation, Disposition: executionapi.OperationRejected, Diagnostic: err.Error()}, nil, err
	}
	if err := l.validateIdentity(request.Snapshot); err != nil {
		return executionapi.LifecycleReceipt{Operation: request.Operation, Disposition: executionapi.OperationUnknown, Diagnostic: err.Error()}, nil, err
	}
	snapshot := request.Snapshot.Clone()
	var receipt executionapi.LifecycleReceipt
	var observer executionapi.LifecycleObserver
	var err error
	switch executionapi.LifecycleAction(request.Operation.Action) {
	case executionapi.LifecyclePrepareSession:
		receipt, observer, err = l.prepareSession(ctx, snapshot, *request.Input.Turn)
	case executionapi.LifecycleStartTurn:
		receipt, observer, err = l.startTurn(ctx, snapshot, *request.Input.Turn)
	case executionapi.LifecycleDeliverFeedback:
		receipt, err = l.deliverFeedback(ctx, request)
	default:
		return executionapi.LifecycleReceipt{}, nil, errors.New("app-server lifecycle does not implement this operation")
	}
	receipt.Operation = request.Operation
	return receipt, observer, err
}

func (l ExecutionLifecycle) deliverFeedback(ctx context.Context, request executionapi.LifecycleRequest) (executionapi.LifecycleReceipt, error) {
	snapshot := request.Snapshot.Clone()
	input := request.Input.Feedback
	receipt := executionapi.LifecycleReceipt{Operation: request.Operation, Disposition: executionapi.OperationUnknown}
	if input == nil || input.Answer == nil || snapshot.QuestionRoute == nil {
		return receipt, errors.New("app-server feedback lacks its original structured question")
	}
	binding, _, err := l.readBinding(snapshot)
	if err != nil {
		return receipt, err
	}
	ask, err := DecodeQuestionRoute(*snapshot.QuestionRoute, snapshot.Session)
	if err != nil {
		return receipt, err
	}
	if input.QuestionID != "" && input.QuestionID != ask.CallID {
		return receipt, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "saved feedback changed its original question identity"}
	}
	if err := l.Backend.ValidateReply(binding, ask.Generation, ask.ThreadID, ask.TurnID, snapshot.Session.CurrentTurnID, snapshot.Session.LastStoppedTurnID); err != nil {
		return receipt, err
	}
	err = SendAskReply(ctx, AskReply{
		Endpoint: binding.Endpoint, Workspace: snapshot.Ref.WorkspacePath, ThreadID: ask.ThreadID,
		RequestID: ask.RequestID, Answer: *input.Answer,
	})
	receipt.State.SessionID, receipt.State.SessionGeneration = binding.SessionID, binding.Generation
	receipt.RequestID = ask.CallID
	if err != nil {
		receipt.Disposition, receipt.Diagnostic = executionapi.OperationUnknown, err.Error()
		return receipt, err
	}
	receipt.Disposition = executionapi.OperationAccepted
	receipt.State.InputAccepted = true
	return receipt, nil
}

func (l ExecutionLifecycle) Reconcile(ctx context.Context, request executionapi.LifecycleRequest) (executionapi.LifecycleReceipt, executionapi.LifecycleObserver, error) {
	if err := request.Validate(); err != nil {
		return executionapi.LifecycleReceipt{Operation: request.Operation, Disposition: executionapi.OperationRejected, Diagnostic: err.Error()}, nil, err
	}
	if err := l.validateIdentity(request.Snapshot); err != nil {
		return executionapi.LifecycleReceipt{Operation: request.Operation, Disposition: executionapi.OperationUnknown, Diagnostic: err.Error()}, nil, err
	}
	snapshot := request.Snapshot.Clone()
	if executionapi.LifecycleAction(request.Operation.Action) == executionapi.LifecycleDeliverFeedback {
		return l.reconcileFeedback(ctx, request)
	}
	binding, _, err := l.readBinding(snapshot)
	if err != nil {
		return executionapi.LifecycleReceipt{}, nil, err
	}
	commandID := snapshot.Session.CurrentCommandID
	turnID := snapshot.Session.CurrentTurnID
	if snapshot.Session.PendingCommandID != "" {
		commandID, turnID = snapshot.Session.PendingCommandID, snapshot.Session.PendingTurnID
	}
	known := append([]string(nil), snapshot.KnownQuestionIDs...)
	recovered, reconcileErr := l.Backend.Recover(ctx, binding, commandID, turnID, known)
	receipt := l.bindingReceipt(snapshot, recovered.Binding)
	receipt.Operation = request.Operation
	receipt.State.SessionID = recovered.Binding.SessionID
	receipt.State.SessionGeneration = recovered.Binding.Generation
	receipt.State.TurnID = recovered.TurnID
	receipt.State.TurnCompleted = recovered.Completed
	receipt.State.WorkerAbsent = recovered.Dead
	if reconcileErr != nil {
		receipt.Disposition = executionapi.OperationUnknown
		receipt.Diagnostic = reconcileErr.Error()
	} else if recovered.Completed {
		receipt.Disposition = executionapi.OperationConfirmed
	} else if recovered.Observer != nil {
		receipt.Disposition = executionapi.OperationAccepted
		receipt.Handle = recovered.Observer.Process
		fixed := lifecycleSession(snapshot, recovered.Binding.SessionID, recovered.TurnID)
		return receipt, newLifecycleObserver(recovered.Observer, fixed), nil
	} else {
		receipt.Disposition = executionapi.OperationUnknown
	}
	return receipt, nil, reconcileErr
}

func (l ExecutionLifecycle) ObserveLifecycle(ctx context.Context, snapshot executionapi.LifecycleSnapshot) (executionapi.LifecycleReceipt, executionapi.LifecycleObserver, error) {
	if err := l.validateIdentity(snapshot); err != nil {
		return executionapi.LifecycleReceipt{}, nil, err
	}
	snapshot = snapshot.Clone()
	binding, _, err := l.readBinding(snapshot)
	if err != nil {
		return executionapi.LifecycleReceipt{}, nil, err
	}
	commandID, turnID := snapshot.Session.CurrentCommandID, snapshot.Session.CurrentTurnID
	if snapshot.Session.PendingCommandID != "" {
		commandID, turnID = snapshot.Session.PendingCommandID, snapshot.Session.PendingTurnID
	}
	recovered, err := l.Backend.Reattach(ctx, binding, commandID, turnID, snapshot.KnownQuestionIDs)
	receipt := l.bindingReceipt(snapshot, recovered.Binding)
	receipt.State.SessionID, receipt.State.TurnID = recovered.Binding.SessionID, recovered.TurnID
	receipt.State.SessionGeneration = recovered.Binding.Generation
	receipt.State.TurnCompleted = recovered.Completed
	if commandID != "" && recovered.Binding.SessionID != "" && recovered.TurnID != "" {
		path := filepath.Join(l.Backend.Config.RunDir, "runs", snapshot.Session.WorkflowRunID, "turns", commandID, "appserver.jsonl")
		if outcome, outcomeErr := ReadAppServerOutcome(path, recovered.Binding.SessionID, recovered.TurnID); outcomeErr == nil && outcome.Started && outcome.Completed {
			receipt.Disposition = executionapi.OperationConfirmed
			receipt.State.TurnCompleted = true
			receipt.TurnEvidence = &executionapi.TurnEvidence{
				SourcePath: path, TurnID: recovered.TurnID, CommandID: commandID, SessionID: outcome.SessionID,
				Started: outcome.Started, Completed: outcome.Completed, Succeeded: outcome.Succeeded,
				LastMessage: outcome.LastMessage, Failure: outcome.Failure, PRCandidates: append([]string(nil), outcome.PRCandidates...),
			}
			return receipt, nil, nil
		}
	}
	if err != nil {
		receipt.Disposition, receipt.Diagnostic = executionapi.OperationUnknown, err.Error()
		return receipt, nil, err
	}
	if recovered.Observer != nil {
		receipt.Disposition = executionapi.OperationAccepted
		receipt.Handle = recovered.Observer.Process
		fixed := lifecycleSession(snapshot, recovered.Binding.SessionID, recovered.TurnID)
		return receipt, newLifecycleObserver(recovered.Observer, fixed), nil
	}
	receipt.Disposition = executionapi.OperationUnknown
	return receipt, nil, nil
}

func (l ExecutionLifecycle) reconcileFeedback(ctx context.Context, request executionapi.LifecycleRequest) (executionapi.LifecycleReceipt, executionapi.LifecycleObserver, error) {
	snapshot := request.Snapshot.Clone()
	input := request.Input.Feedback
	receipt := executionapi.LifecycleReceipt{Operation: request.Operation, Disposition: executionapi.OperationUnknown}
	if input == nil || input.Answer == nil || snapshot.QuestionRoute == nil || snapshot.Session.CurrentCommandID == "" {
		return receipt, nil, errors.New("saved app-server reply lacks its original question, command, or answers")
	}
	binding, _, err := l.readBinding(snapshot)
	if err != nil {
		return receipt, nil, err
	}
	ask, err := DecodeQuestionRoute(*snapshot.QuestionRoute, snapshot.Session)
	if err != nil {
		return receipt, nil, err
	}
	path := filepath.Join(l.Backend.Config.RunDir, "runs", snapshot.Session.WorkflowRunID, "turns", snapshot.Session.CurrentCommandID, "appserver.jsonl")
	confirmed, err := AnswerEvidence(path, ask.ThreadID, ask.TurnID, ask.CallID, input.Answer.Answers)
	if err != nil {
		receipt.Diagnostic = err.Error()
		return receipt, nil, err
	}
	if !confirmed {
		receipt.Diagnostic = "original app-server answer has no matching committed tool reply"
		return receipt, nil, nil
	}
	receipt.Disposition = executionapi.OperationConfirmed
	receipt.RequestID = ask.CallID
	receipt.State.SessionID = binding.SessionID
	receipt.State.SessionGeneration = binding.Generation
	receipt.State.InputAccepted = true
	return receipt, nil, nil
}

func (l ExecutionLifecycle) StopLifecycle(ctx context.Context, request executionapi.LifecycleRequest, handle executionapi.StopHandle) (executionapi.LifecycleReceipt, error) {
	if err := request.Validate(); err != nil {
		return executionapi.LifecycleReceipt{}, err
	}
	ref := request.Snapshot.Ref
	result := executionapi.LifecycleReceipt{Operation: request.Operation, RequestID: request.Operation.ID}
	if ref.Protocol != executionapi.ProtocolLocalCodexAppServer || ref.External.Kind != "os" || ref.External.PID <= 0 || handle == nil || handle.Identity() != ref.External {
		return result, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "app-server stop handle does not match the fixed process identity"}
	}
	if executionapi.LifecycleAction(request.Operation.Action) == executionapi.LifecycleStopObservation {
		binding, found, err := l.readBinding(request.Snapshot.Clone())
		if err != nil || !found || binding.SessionID != request.Snapshot.Session.SessionID || binding.Process == nil || *binding.Process != ref.External ||
			request.Input.Stop.TurnID == "" || request.Input.Stop.TurnID != request.Snapshot.Session.CurrentTurnID || request.Input.Stop.TurnID == request.Snapshot.Session.LastStoppedTurnID {
			return result, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "app-server interrupt does not match the fixed live session and turn", Cause: err}
		}
		if err := Interrupt(ctx, binding.Endpoint, binding.SessionID, request.Input.Stop.TurnID); err != nil {
			result.Disposition, result.Diagnostic = executionapi.OperationUnknown, err.Error()
			return result, &executionapi.BackendError{Code: executionapi.BackendOutcomeUnknown, Diagnostic: "app-server interrupt receipt is unverified", Cause: err}
		}
		result.Disposition = executionapi.OperationAccepted
		return result, nil
	}
	if executionapi.LifecycleAction(request.Operation.Action) != executionapi.LifecycleStopExecution {
		return result, &executionapi.BackendError{Code: executionapi.BackendUnsupported, Diagnostic: "app-server stop action is unsupported"}
	}
	if l.Backend.Process.Inspect(&ref.External) == "process absent" {
		handle.Close()
		result.Disposition, result.State.Stopped = executionapi.OperationConfirmed, true
		return result, nil
	}
	handle.Kill()
	ticker := time.NewTicker(40 * time.Millisecond)
	defer ticker.Stop()
	for {
		if l.Backend.Process.Inspect(&ref.External) == "process absent" {
			handle.Close()
			result.Disposition, result.State.Stopped = executionapi.OperationConfirmed, true
			return result, nil
		}
		select {
		case <-ctx.Done():
			result.Disposition, result.Diagnostic = executionapi.OperationUnknown, ctx.Err().Error()
			return result, &executionapi.BackendError{Code: executionapi.BackendOutcomeUnknown, Diagnostic: "app-server process exit is unverified", Cause: ctx.Err()}
		case <-ticker.C:
		}
	}
}

func (l ExecutionLifecycle) CleanupLifecycle(_ context.Context, request executionapi.LifecycleRequest) (executionapi.LifecycleReceipt, error) {
	return executionapi.LifecycleReceipt{Operation: request.Operation, Disposition: executionapi.OperationRejected}, &executionapi.BackendError{Code: executionapi.BackendUnsupported, Diagnostic: "app-server cleanup is owned by the workspace and process owners"}
}

func (l ExecutionLifecycle) VerifyManualRecovery(ctx context.Context, snapshot executionapi.LifecycleSnapshot) (executionapi.LifecycleRecoveryEvidence, error) {
	if err := ctx.Err(); err != nil {
		return executionapi.LifecycleRecoveryEvidence{}, err
	}
	if err := l.validateIdentity(snapshot); err != nil {
		return executionapi.LifecycleRecoveryEvidence{}, err
	}
	binding, found, err := l.readBinding(snapshot.Clone())
	if err != nil {
		return executionapi.LifecycleRecoveryEvidence{}, err
	}
	if !found || !ValidNativeID(binding.SessionID) {
		return executionapi.LifecycleRecoveryEvidence{SessionAvailable: false}, nil
	}
	managed, absent := l.Backend.ManagedProcessState(binding)
	return executionapi.LifecycleRecoveryEvidence{SessionAvailable: true, ManagedProcess: managed, ProcessAbsent: absent}, nil
}

func (l ExecutionLifecycle) BindRecovery(ctx context.Context, previous, next executionapi.LifecycleSnapshot) (executionapi.ImplementationDataEnvelope, error) {
	if err := ctx.Err(); err != nil {
		return executionapi.ImplementationDataEnvelope{}, err
	}
	if err := l.validateIdentity(previous); err != nil {
		return executionapi.ImplementationDataEnvelope{}, err
	}
	if err := l.validateIdentity(next); err != nil {
		return executionapi.ImplementationDataEnvelope{}, err
	}
	if previous.Session.ProjectID != next.Session.ProjectID || previous.Session.ExecutionID != next.Session.ExecutionID ||
		previous.Session.AttemptID == next.Session.AttemptID || previous.ConfigurationDigest != next.ConfigurationDigest {
		return executionapi.ImplementationDataEnvelope{}, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "manual recovery changed the fixed execution or configuration identity"}
	}
	binding, found, err := l.readBinding(previous.Clone())
	if err != nil {
		return executionapi.ImplementationDataEnvelope{}, err
	}
	if !found || !ValidNativeID(binding.SessionID) {
		return executionapi.ImplementationDataEnvelope{}, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "original app-server thread identity is unavailable for manual recovery"}
	}
	seed := AppServerBinding{SessionID: binding.SessionID}
	return NewAppServerBindingEnvelope(l.identity(next), seed)
}

func (l ExecutionLifecycle) prepareSession(ctx context.Context, snapshot executionapi.LifecycleSnapshot, input executionapi.TurnInput) (executionapi.LifecycleReceipt, executionapi.LifecycleObserver, error) {
	fixed, _, err := l.readBinding(snapshot)
	if err != nil {
		return executionapi.LifecycleReceipt{}, nil, err
	}
	handle, binding, prepareErr := l.Backend.PrepareSession(ctx, fixed, input)
	receipt := l.bindingReceipt(snapshot, binding)
	receipt.State.SessionID = binding.SessionID
	receipt.State.SessionGeneration = binding.Generation
	if binding.Process != nil {
		process := *binding.Process
		receipt.Process = &process
	}
	if prepareErr != nil {
		receipt.Disposition = executionapi.OperationUnknown
		receipt.Diagnostic = prepareErr.Error()
		if handle != nil {
			receipt.Handle = handle.Detach()
		}
		return receipt, nil, prepareErr
	}
	if handle == nil || binding.SessionID == "" || binding.Process == nil {
		if handle != nil {
			receipt.Handle = handle.Detach()
		}
		receipt.Disposition = executionapi.OperationUnknown
		err := errors.New("app-server preparation returned no complete original session receipt")
		receipt.Diagnostic = err.Error()
		return receipt, nil, err
	}
	if err := l.writeHistoryBinding(snapshot, binding, handle); err != nil {
		receipt.Handle = handle.Detach()
		receipt.Disposition = executionapi.OperationUnknown
		receipt.Diagnostic = err.Error()
		return receipt, nil, err
	}
	receipt.EventPath = handle.Paths.JournalPath
	receipt.Handle = handle.Detach()
	receipt.Disposition = executionapi.OperationConfirmed
	return receipt, nil, nil
}

func (l ExecutionLifecycle) startTurn(ctx context.Context, snapshot executionapi.LifecycleSnapshot, input executionapi.TurnInput) (executionapi.LifecycleReceipt, executionapi.LifecycleObserver, error) {
	fixed, found, err := l.readBinding(snapshot)
	if err != nil {
		return executionapi.LifecycleReceipt{}, nil, err
	}
	if !found || fixed.SessionID == "" || fixed.Endpoint == "" || fixed.Generation == "" {
		return executionapi.LifecycleReceipt{}, nil, errors.New("app-server turn requires its committed original session binding")
	}
	handle, binding, err := l.Backend.PrepareSession(ctx, fixed, input)
	if err != nil {
		receipt := l.bindingReceipt(snapshot, binding)
		receipt.State.SessionGeneration = binding.Generation
		receipt.Disposition, receipt.Diagnostic = executionapi.OperationUnknown, err.Error()
		if binding.Process != nil {
			process := *binding.Process
			receipt.Process = &process
		}
		return receipt, nil, err
	}
	observer, turnID, startErr := l.Backend.StartTurn(ctx, handle, input, input.LinkedWorktreeArgs)
	receipt := l.bindingReceipt(snapshot, binding)
	receipt.State.SessionID, receipt.State.TurnID = binding.SessionID, turnID
	receipt.State.SessionGeneration = binding.Generation
	if binding.Process != nil {
		process := *binding.Process
		receipt.Process = &process
	}
	if startErr != nil {
		receipt.Disposition, receipt.Diagnostic = executionapi.OperationUnknown, startErr.Error()
		if handle != nil {
			receipt.Handle = handle.Detach()
		}
		return receipt, nil, startErr
	}
	receipt.Disposition = executionapi.OperationAccepted
	receipt.State.InputAccepted = true
	receipt.State.TurnStarted = true
	receipt.EventPath = handle.Paths.JournalPath
	fixedContext := lifecycleSession(snapshot, binding.SessionID, turnID)
	return receipt, newLifecycleObserver(observer, fixedContext), nil
}

func (l ExecutionLifecycle) writeHistoryBinding(snapshot executionapi.LifecycleSnapshot, binding Binding, handle *SessionHandle) error {
	if snapshot.Ref.WorkspacePath == "" || handle == nil {
		return errors.New("app-server history binding has no fixed workspace")
	}
	processRef := ""
	if binding.Process != nil && binding.Process.PID > 0 {
		processRef = fmt.Sprintf("process:%d@%s", binding.Process.PID, binding.Process.Created)
	}
	history := HistoryBinding{
		Version: 1, ProjectID: snapshot.Session.ProjectID, TaskID: snapshot.Session.TaskID,
		WorkflowID: snapshot.Session.WorkflowRunID, ExecutionID: snapshot.Session.ExecutionID,
		AttemptID: snapshot.Session.AttemptID, Backend: executionapi.BackendLocal,
		Protocol: executionapi.ProtocolLocalCodexAppServer, TargetID: snapshot.Ref.Target.ID,
		ExternalRef: processRef, Workspace: snapshot.Ref.WorkspacePath, SessionID: binding.SessionID,
		Command: l.HistoryCommand, Home: EnvironmentHome(),
	}
	if err := WriteHistoryBinding(localHistoryBindingPath(handle.Paths.TurnDir, binding.SessionID), history, l.Backend.Process.WriteAtomic); err != nil {
		return err
	}
	return nil
}

func localHistoryBindingPath(dir, session string) string {
	return filepath.Join(dir, "codex-history."+session+".v1.json")
}

func (l ExecutionLifecycle) validateIdentity(snapshot executionapi.LifecycleSnapshot) error {
	if snapshot.Session.ProjectID == "" || snapshot.Session.ExecutionID == "" || snapshot.Session.AttemptID == "" ||
		snapshot.Session.WorkflowRunID == "" || snapshot.ConfigurationDigest == "" || snapshot.ResourceGeneration == "" {
		return errors.New("app-server lifecycle has no fixed execution identity")
	}
	return nil
}

func (l ExecutionLifecycle) identity(snapshot executionapi.LifecycleSnapshot) AppServerBindingIdentity {
	return AppServerBindingIdentity{
		ProjectID: snapshot.Session.ProjectID, ExecutionID: snapshot.Session.ExecutionID,
		AttemptID: snapshot.Session.AttemptID, Generation: snapshot.ResourceGeneration,
		ConfigDigest: snapshot.ConfigurationDigest,
	}
}

func (l ExecutionLifecycle) readBinding(snapshot executionapi.LifecycleSnapshot) (Binding, bool, error) {
	var envelope *executionapi.ImplementationDataEnvelope
	for i := range snapshot.Implementation {
		if snapshot.Implementation[i].Purpose != executionapi.ImplementationResourcePurpose || snapshot.Implementation[i].ImplementationKey != ExecutionBindingCodecKey {
			return Binding{}, false, errors.New("app-server lifecycle received an unsupported resource envelope")
		}
		if envelope != nil {
			return Binding{}, false, errors.New("execution has duplicate app-server binding envelopes")
		}
		value := snapshot.Implementation[i].Clone()
		envelope = &value
	}
	if envelope == nil {
		return Binding{Process: optionalProcess(snapshot.Ref.External)}, false, nil
	}
	decoded, err := DecodeAppServerBindingEnvelope(*envelope, l.identity(snapshot))
	if err != nil {
		return Binding{}, false, err
	}
	binding := Binding{Process: optionalProcess(snapshot.Ref.External), Endpoint: decoded.Endpoint, Generation: decoded.Generation, SessionID: decoded.SessionID}
	return binding, true, nil
}

func (l ExecutionLifecycle) bindingReceipt(snapshot executionapi.LifecycleSnapshot, binding Binding) executionapi.LifecycleReceipt {
	receipt := executionapi.LifecycleReceipt{}
	if binding.Process != nil {
		process := *binding.Process
		receipt.Process = &process
	}
	if binding.Endpoint != "" || binding.SessionID != "" || binding.Generation != "" {
		if envelope, err := NewAppServerBindingEnvelope(l.identity(snapshot), AppServerBinding{Endpoint: binding.Endpoint, SessionID: binding.SessionID, Generation: binding.Generation}); err == nil {
			receipt.Implementation = []executionapi.ImplementationDataEnvelope{envelope}
		}
	}
	return receipt
}

func optionalProcess(identity executionapi.ProcessIdentity) *executionapi.ProcessIdentity {
	if identity == (executionapi.ProcessIdentity{}) {
		return nil
	}
	copy := identity
	return &copy
}

func cloneTurn(turn *executionapi.TurnInput) *executionapi.TurnInput {
	if turn == nil {
		return nil
	}
	copy := *turn
	copy.LinkedWorktreeArgs = append([]string(nil), turn.LinkedWorktreeArgs...)
	return &copy
}

func lifecycleSession(snapshot executionapi.LifecycleSnapshot, sessionID, turnID string) executionapi.SessionContext {
	fixed := snapshot.Session
	fixed.SessionID, fixed.CurrentTurnID = sessionID, turnID
	return fixed
}

type lifecycleObserver struct {
	original *Observation
	events   chan executionapi.LifecycleEvent
	fixed    executionapi.SessionContext
}

func newLifecycleObserver(original *Observation, fixed executionapi.SessionContext) executionapi.LifecycleObserver {
	observer := &lifecycleObserver{original: original, events: make(chan executionapi.LifecycleEvent, cap(original.Events)), fixed: fixed}
	go observer.forward()
	return observer
}

func (observer *lifecycleObserver) Events() <-chan executionapi.LifecycleEvent {
	return observer.events
}
func (observer *lifecycleObserver) Done() <-chan error { return observer.original.Done }
func (observer *lifecycleObserver) StopHandle() executionapi.StopHandle {
	return observer.original.Process
}

func (observer *lifecycleObserver) Close(ctx context.Context) error {
	observer.original.Cancel()
	select {
	case <-observer.original.Finished:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (observer *lifecycleObserver) forward() {
	defer close(observer.events)
	for value := range observer.original.Events {
		fact := executionapi.EventFact{}
		if value.Forward {
			decoded, err := DecodeEvent(value.Envelope, observer.fixed, true)
			if err != nil {
				value.Persisted <- err
				return
			}
			fact = decoded.EventFact
			fact.Source = "execution"
			fact.SourceEventID = value.Envelope.Method + ":" + digestValue(value.Envelope)
		} else {
			fact.Kind = executionapi.EventStatus
			fact.Source = "execution"
			fact.SourceEventID = value.Envelope.Method + ":" + digestValue(value.Envelope)
		}
		persisted := make(chan error, 1)
		event := executionapi.LifecycleEvent{Fact: fact, Summary: value.Summary, Kind: value.Kind, Status: value.Status, Title: value.Title, Persisted: persisted}
		select {
		case observer.events <- event:
		case <-observer.original.Finished:
			value.Persisted <- errors.New("app-server observation ended before event acknowledgement")
			return
		}
		select {
		case err := <-persisted:
			value.Persisted <- err
		case <-observer.original.Finished:
			value.Persisted <- errors.New("app-server observation ended before event acknowledgement")
			return
		}
	}
}
