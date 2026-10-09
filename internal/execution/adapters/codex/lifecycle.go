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

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	"github.com/erranliu/SteerLine/internal/execution/workspace"
)

// ExecutionLifecycle owns app-server session setup, turn input, observation,
// and reconciliation. Every mutating call receives one center-persisted
// operation; the returned binding must be committed before another step.
type ExecutionLifecycle struct {
	setup          executionbackend.LifecycleSetup
	Backend        Backend
	HistoryCommand executionbackend.Command
	ProbeLifecycle func(context.Context, executionbackend.LifecycleSetup) (executionbackend.CapabilityReport, error)
	prepared       *SessionHandle
}

func (l ExecutionLifecycle) PolicyCapabilities() executionbackend.LifecyclePolicyCapabilities {
	return executionbackend.LifecyclePolicyCapabilities{LinkedWorkspaceWriteRoots: true}
}

func NewExecutionLifecycle(setup executionbackend.LifecycleSetup) (executionbackend.LifecyclePort, error) {
	setup = setup.Clone()
	snapshot := setup.Snapshot
	if snapshot.Ref.Backend != executionbackend.BackendLocal || snapshot.Ref.Protocol != executionbackend.ProtocolLocalCodexAppServer || snapshot.Session.Agent.Name != "codex" || setup.ProcessHost == nil {
		return nil, &executionbackend.BackendError{Code: executionbackend.BackendUnsupported, Diagnostic: "local app-server lifecycle setup differs from its fixed implementation target"}
	}
	profile, err := DecodeProfileConfigEnvelope(setup.ProfileData, ProfileConfigIdentity{
		ProjectID: snapshot.Session.ProjectID, ProfileID: setup.ProfileID, AcceptedConfigVersion: setup.AcceptedConfigVersion,
	})
	if err != nil {
		return nil, err
	}
	permissions, err := NativePermissions(runtime.GOOS, "local", "codex")
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
			Executable: profile.Executable, Args: []string(nil),
			Profile: "", Sandbox: permissions.Sandbox, WindowsSandbox: permissions.WindowsSandbox,
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
	history := ReaderCommand(executionbackend.Command{Executable: profile.Executable, Args: []string(nil)}, "")
	return &ExecutionLifecycle{setup: setup, Backend: backend, HistoryCommand: history, ProbeLifecycle: func(ctx context.Context, probeSetup executionbackend.LifecycleSetup) (executionbackend.CapabilityReport, error) {
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

func probeLifecycleCapabilities(ctx context.Context, backend Backend, history executionbackend.Command, setup executionbackend.LifecycleSetup) (executionbackend.CapabilityReport, error) {
	probe := ProbePort{
		Output: func(ctx context.Context, executable string, args []string) ([]byte, error) {
			return setup.ProcessHost.OutputCommand(ctx, executionbackend.LaunchCommand{Executable: executable, Args: append([]string(nil), args...), WorkingDirectory: setup.RepositoryPath})
		},
		CheckHistory: func(ctx context.Context) error {
			client, err := OpenHistory(ctx, history, setup.RepositoryPath, "", setup.ProcessHost.StartManaged)
			if err != nil {
				return err
			}
			client.Close()
			return nil
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
	status := executionbackend.CapabilityNotTested
	if err == nil {
		status = executionbackend.CapabilityVerified
	}
	report := executionbackend.CapabilityReport{
		Version: 1, Backend: setup.Snapshot.Ref.Backend, Protocol: setup.Snapshot.Ref.Protocol,
		Target: setup.Snapshot.Ref.Target, Agent: setup.Snapshot.Ref.Agent, CheckedAt: time.Now().UTC(),
		Capabilities: []executionbackend.Capability{{Name: "start", Status: status}, {Name: "observe", Status: status}, {Name: "operation_reconciliation", Status: status}},
	}
	return report, err
}

func (l ExecutionLifecycle) PreflightLifecycle(ctx context.Context, setup executionbackend.LifecycleSetup) (executionbackend.CapabilityReport, error) {
	if l.ProbeLifecycle == nil {
		return executionbackend.CapabilityReport{}, &executionbackend.BackendError{Code: executionbackend.BackendUnsupported, Diagnostic: "lifecycle capability probe is not configured"}
	}
	return l.ProbeLifecycle(ctx, setup.Clone())
}

func (l ExecutionLifecycle) Next(_ context.Context, snapshot executionbackend.LifecycleSnapshot) (executionbackend.LifecycleCandidate, error) {
	if snapshot.Workspace.Path == "" && snapshot.WorkspacePlan.Path != "" {
		if snapshot.Session.SessionID != "" || snapshot.Ref.External.Kind != "" || len(snapshot.Implementation) != 0 {
			return executionbackend.LifecycleCandidate{}, errors.New("missing original workspace cannot be recreated for a bound worker")
		}
		intent := snapshot.WorkspacePlan
		candidate := executionbackend.LifecycleCandidate{Action: executionbackend.LifecyclePrepareWorkspace, Input: executionbackend.LifecycleInput{Workspace: &intent}}
		return candidate, candidate.Validate()
	}
	if snapshot.PendingTurn == nil {
		return executionbackend.LifecycleCandidate{Done: true}, nil
	}
	if snapshot.PendingTurn.CommandID == "" || snapshot.PendingTurn.Prompt == "" {
		return executionbackend.LifecycleCandidate{}, errors.New("app-server lifecycle has no saved turn input")
	}
	binding, found, err := l.readBinding(snapshot)
	if err != nil {
		return executionbackend.LifecycleCandidate{}, err
	}
	if !found || binding.Endpoint == "" || binding.Generation == "" {
		candidate := executionbackend.LifecycleCandidate{Action: executionbackend.LifecyclePrepareSession, Input: executionbackend.LifecycleInput{Turn: cloneTurn(snapshot.PendingTurn)}}
		return candidate, candidate.Validate()
	}
	candidate := executionbackend.LifecycleCandidate{Action: executionbackend.LifecycleStartTurn, Input: executionbackend.LifecycleInput{Turn: cloneTurn(snapshot.PendingTurn)}}
	return candidate, candidate.Validate()
}

func (l *ExecutionLifecycle) Execute(ctx context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, executionbackend.LifecycleObserver, error) {
	if err := request.Validate(); err != nil {
		return executionbackend.LifecycleReceipt{Operation: request.Operation, Disposition: executionbackend.OperationRejected, Diagnostic: err.Error()}, nil, err
	}
	if err := l.validateIdentity(request.Snapshot); err != nil {
		return executionbackend.LifecycleReceipt{Operation: request.Operation, Disposition: executionbackend.OperationUnknown, Diagnostic: err.Error()}, nil, err
	}
	snapshot := request.Snapshot.Clone()
	var receipt executionbackend.LifecycleReceipt
	var observer executionbackend.LifecycleObserver
	var err error
	switch executionbackend.LifecycleAction(request.Operation.Action) {
	case executionbackend.LifecyclePrepareWorkspace:
		receipt, err = l.prepareWorkspace(ctx, request, false)
	case executionbackend.LifecyclePrepareSession:
		receipt, observer, err = l.prepareSession(ctx, snapshot, *request.Input.Turn)
	case executionbackend.LifecycleStartTurn:
		receipt, observer, err = l.startTurn(ctx, snapshot, *request.Input.Turn)
	case executionbackend.LifecycleDeliverFeedback:
		receipt, err = l.deliverFeedback(ctx, request)
	default:
		return executionbackend.LifecycleReceipt{}, nil, errors.New("app-server lifecycle does not implement this operation")
	}
	receipt.Operation = request.Operation
	return receipt, observer, err
}

func (l ExecutionLifecycle) deliverFeedback(ctx context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, error) {
	snapshot := request.Snapshot.Clone()
	input := request.Input.Feedback
	receipt := executionbackend.LifecycleReceipt{Operation: request.Operation, Disposition: executionbackend.OperationUnknown}
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
		return receipt, &executionbackend.BackendError{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "saved feedback changed its original question identity"}
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
		receipt.Disposition, receipt.Diagnostic = executionbackend.OperationUnknown, err.Error()
		return receipt, err
	}
	receipt.Disposition = executionbackend.OperationAccepted
	receipt.State.InputAccepted = true
	return receipt, nil
}

func (l ExecutionLifecycle) Reconcile(ctx context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, executionbackend.LifecycleObserver, error) {
	if err := request.Validate(); err != nil {
		return executionbackend.LifecycleReceipt{Operation: request.Operation, Disposition: executionbackend.OperationRejected, Diagnostic: err.Error()}, nil, err
	}
	if err := l.validateIdentity(request.Snapshot); err != nil {
		return executionbackend.LifecycleReceipt{Operation: request.Operation, Disposition: executionbackend.OperationUnknown, Diagnostic: err.Error()}, nil, err
	}
	snapshot := request.Snapshot.Clone()
	if executionbackend.LifecycleAction(request.Operation.Action) == executionbackend.LifecyclePrepareWorkspace {
		receipt, err := l.prepareWorkspace(ctx, request, true)
		return receipt, nil, err
	}
	if executionbackend.LifecycleAction(request.Operation.Action) == executionbackend.LifecycleDeliverFeedback {
		return l.reconcileFeedback(ctx, request)
	}
	binding, _, err := l.readBinding(snapshot)
	if err != nil {
		return executionbackend.LifecycleReceipt{}, nil, err
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
		receipt.Disposition = executionbackend.OperationUnknown
		receipt.Diagnostic = reconcileErr.Error()
	} else if recovered.Completed {
		receipt.Disposition = executionbackend.OperationConfirmed
	} else if recovered.Observer != nil {
		receipt.Disposition = executionbackend.OperationAccepted
		receipt.Handle = recovered.Observer.Process
		fixed := lifecycleSession(snapshot, recovered.Binding.SessionID, recovered.TurnID)
		return receipt, newLifecycleObserver(recovered.Observer, fixed), nil
	} else {
		receipt.Disposition = executionbackend.OperationUnknown
	}
	return receipt, nil, reconcileErr
}

func (l ExecutionLifecycle) ObserveLifecycle(ctx context.Context, snapshot executionbackend.LifecycleSnapshot) (executionbackend.LifecycleReceipt, executionbackend.LifecycleObserver, error) {
	if err := l.validateIdentity(snapshot); err != nil {
		return executionbackend.LifecycleReceipt{}, nil, err
	}
	snapshot = snapshot.Clone()
	binding, _, err := l.readBinding(snapshot)
	if err != nil {
		return executionbackend.LifecycleReceipt{}, nil, err
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
			receipt.Disposition = executionbackend.OperationConfirmed
			receipt.State.TurnCompleted = true
			receipt.TurnEvidence = &executionbackend.TurnEvidence{
				SourcePath: path, TurnID: recovered.TurnID, CommandID: commandID, SessionID: outcome.SessionID,
				Started: outcome.Started, Completed: outcome.Completed, Succeeded: outcome.Succeeded,
				LastMessage: outcome.LastMessage, Failure: outcome.Failure, PRCandidates: append([]string(nil), outcome.PRCandidates...),
			}
			return receipt, nil, nil
		}
	}
	if err != nil {
		receipt.Disposition, receipt.Diagnostic = executionbackend.OperationUnknown, err.Error()
		return receipt, nil, err
	}
	if recovered.Observer != nil {
		receipt.Disposition = executionbackend.OperationAccepted
		receipt.Handle = recovered.Observer.Process
		fixed := lifecycleSession(snapshot, recovered.Binding.SessionID, recovered.TurnID)
		return receipt, newLifecycleObserver(recovered.Observer, fixed), nil
	}
	receipt.Disposition = executionbackend.OperationUnknown
	return receipt, nil, nil
}

func (l ExecutionLifecycle) reconcileFeedback(ctx context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, executionbackend.LifecycleObserver, error) {
	snapshot := request.Snapshot.Clone()
	input := request.Input.Feedback
	receipt := executionbackend.LifecycleReceipt{Operation: request.Operation, Disposition: executionbackend.OperationUnknown}
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
	receipt.Disposition = executionbackend.OperationConfirmed
	receipt.RequestID = ask.CallID
	receipt.State.SessionID = binding.SessionID
	receipt.State.SessionGeneration = binding.Generation
	receipt.State.InputAccepted = true
	return receipt, nil, nil
}

func (l ExecutionLifecycle) StopLifecycle(ctx context.Context, request executionbackend.LifecycleRequest, handle executionbackend.StopHandle) (executionbackend.LifecycleReceipt, error) {
	if err := request.Validate(); err != nil {
		return executionbackend.LifecycleReceipt{}, err
	}
	ref := request.Snapshot.Ref
	result := executionbackend.LifecycleReceipt{Operation: request.Operation, RequestID: request.Operation.ID}
	if ref.Protocol != executionbackend.ProtocolLocalCodexAppServer || ref.External.Kind != "os" || ref.External.PID <= 0 || handle == nil || handle.Identity() != ref.External {
		return result, &executionbackend.BackendError{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "app-server stop handle does not match the fixed process identity"}
	}
	if executionbackend.LifecycleAction(request.Operation.Action) == executionbackend.LifecycleStopObservation {
		binding, found, err := l.readBinding(request.Snapshot.Clone())
		if err != nil || !found || binding.SessionID != request.Snapshot.Session.SessionID || binding.Process == nil || *binding.Process != ref.External ||
			request.Input.Stop.TurnID == "" || request.Input.Stop.TurnID != request.Snapshot.Session.CurrentTurnID || request.Input.Stop.TurnID == request.Snapshot.Session.LastStoppedTurnID {
			return result, &executionbackend.BackendError{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "app-server interrupt does not match the fixed live session and turn", Cause: err}
		}
		if err := Interrupt(ctx, binding.Endpoint, binding.SessionID, request.Input.Stop.TurnID); err != nil {
			result.Disposition, result.Diagnostic = executionbackend.OperationUnknown, err.Error()
			return result, &executionbackend.BackendError{Code: executionbackend.BackendOutcomeUnknown, Diagnostic: "app-server interrupt receipt is unverified", Cause: err}
		}
		result.Disposition = executionbackend.OperationAccepted
		return result, nil
	}
	if executionbackend.LifecycleAction(request.Operation.Action) != executionbackend.LifecycleStopExecution {
		return result, &executionbackend.BackendError{Code: executionbackend.BackendUnsupported, Diagnostic: "app-server stop action is unsupported"}
	}
	if l.Backend.Process.Inspect(&ref.External) == "process absent" {
		handle.Close()
		result.Disposition, result.State.Stopped = executionbackend.OperationConfirmed, true
		return result, nil
	}
	handle.Kill()
	ticker := time.NewTicker(40 * time.Millisecond)
	defer ticker.Stop()
	for {
		if l.Backend.Process.Inspect(&ref.External) == "process absent" {
			handle.Close()
			result.Disposition, result.State.Stopped = executionbackend.OperationConfirmed, true
			return result, nil
		}
		select {
		case <-ctx.Done():
			result.Disposition, result.Diagnostic = executionbackend.OperationUnknown, ctx.Err().Error()
			return result, &executionbackend.BackendError{Code: executionbackend.BackendOutcomeUnknown, Diagnostic: "app-server process exit is unverified", Cause: ctx.Err()}
		case <-ticker.C:
		}
	}
}

func (l ExecutionLifecycle) CleanupLifecycle(_ context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, error) {
	return executionbackend.LifecycleReceipt{Operation: request.Operation, Disposition: executionbackend.OperationRejected}, &executionbackend.BackendError{Code: executionbackend.BackendUnsupported, Diagnostic: "app-server cleanup is owned by the workspace and process owners"}
}

func (l ExecutionLifecycle) VerifyManualRecovery(ctx context.Context, snapshot executionbackend.LifecycleSnapshot) (executionbackend.LifecycleRecoveryEvidence, error) {
	if err := ctx.Err(); err != nil {
		return executionbackend.LifecycleRecoveryEvidence{}, err
	}
	if err := l.validateIdentity(snapshot); err != nil {
		return executionbackend.LifecycleRecoveryEvidence{}, err
	}
	binding, found, err := l.readBinding(snapshot.Clone())
	if err != nil {
		return executionbackend.LifecycleRecoveryEvidence{}, err
	}
	if !found || !ValidNativeID(binding.SessionID) {
		return executionbackend.LifecycleRecoveryEvidence{SessionAvailable: false}, nil
	}
	managed, absent := l.Backend.ManagedProcessState(binding)
	return executionbackend.LifecycleRecoveryEvidence{SessionAvailable: true, ManagedProcess: managed, ProcessAbsent: absent}, nil
}

func (l ExecutionLifecycle) BindRecovery(ctx context.Context, previous, next executionbackend.LifecycleSnapshot) (executionbackend.ImplementationDataEnvelope, error) {
	if err := ctx.Err(); err != nil {
		return executionbackend.ImplementationDataEnvelope{}, err
	}
	if err := l.validateIdentity(previous); err != nil {
		return executionbackend.ImplementationDataEnvelope{}, err
	}
	if err := l.validateIdentity(next); err != nil {
		return executionbackend.ImplementationDataEnvelope{}, err
	}
	if previous.Session.ProjectID != next.Session.ProjectID || previous.Session.ExecutionID != next.Session.ExecutionID ||
		previous.Session.AttemptID == next.Session.AttemptID || previous.ConfigurationDigest != next.ConfigurationDigest {
		return executionbackend.ImplementationDataEnvelope{}, &executionbackend.BackendError{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "manual recovery changed the fixed execution or configuration identity"}
	}
	binding, found, err := l.readBinding(previous.Clone())
	if err != nil {
		return executionbackend.ImplementationDataEnvelope{}, err
	}
	if !found || !ValidNativeID(binding.SessionID) {
		return executionbackend.ImplementationDataEnvelope{}, &executionbackend.BackendError{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "original app-server thread identity is unavailable for manual recovery"}
	}
	seed := AppServerBinding{SessionID: binding.SessionID}
	return NewAppServerBindingEnvelope(l.identity(next), seed)
}

// ReleasePreparation closes only an unconsumed preparation connection. The
// committed process remains owned by the center; a started observer owns its
// own connection. The launcher calls this even when the receipt is rejected.
func (l *ExecutionLifecycle) ReleasePreparation() {
	if l.prepared != nil {
		l.prepared.Close()
		l.prepared = nil
	}
}

func (l *ExecutionLifecycle) prepareSession(ctx context.Context, snapshot executionbackend.LifecycleSnapshot, input executionbackend.TurnInput) (executionbackend.LifecycleReceipt, executionbackend.LifecycleObserver, error) {
	l.ReleasePreparation()
	fixed, _, err := l.readBinding(snapshot)
	if err != nil {
		return executionbackend.LifecycleReceipt{}, nil, err
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
		receipt.Disposition = executionbackend.OperationUnknown
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
		receipt.Disposition = executionbackend.OperationUnknown
		err := errors.New("app-server preparation returned no complete original session receipt")
		receipt.Diagnostic = err.Error()
		return receipt, nil, err
	}
	if err := l.writeHistoryBinding(snapshot, binding, handle); err != nil {
		receipt.Handle = handle.Detach()
		receipt.Disposition = executionbackend.OperationUnknown
		receipt.Diagnostic = err.Error()
		return receipt, nil, err
	}
	receipt.EventPath = handle.Paths.JournalPath
	// An empty native thread has no rollout to resume after disconnect. Keep
	// its connection until the center commits this receipt and starts its turn.
	handle.owned = false
	l.prepared = handle
	receipt.Handle = handle.process
	receipt.Disposition = executionbackend.OperationConfirmed
	return receipt, nil, nil
}

func (l *ExecutionLifecycle) startTurn(ctx context.Context, snapshot executionbackend.LifecycleSnapshot, input executionbackend.TurnInput) (executionbackend.LifecycleReceipt, executionbackend.LifecycleObserver, error) {
	fixed, found, err := l.readBinding(snapshot)
	if err != nil {
		return executionbackend.LifecycleReceipt{}, nil, err
	}
	if !found || fixed.SessionID == "" || fixed.Endpoint == "" || fixed.Generation == "" {
		return executionbackend.LifecycleReceipt{}, nil, errors.New("app-server turn requires its committed original session binding")
	}
	handle := l.prepared
	l.prepared = nil
	binding := fixed
	if handle != nil {
		defer handle.Close()
		if !sameBinding(handle.Binding, fixed) {
			err = errors.New("app-server turn changed its prepared original session binding")
		}
	} else {
		handle, binding, err = l.Backend.PrepareSession(ctx, fixed, input)
	}
	if err != nil {
		receipt := l.bindingReceipt(snapshot, binding)
		receipt.State.SessionID = binding.SessionID
		receipt.State.SessionGeneration = binding.Generation
		receipt.Disposition, receipt.Diagnostic = executionbackend.OperationUnknown, err.Error()
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
		receipt.Disposition, receipt.Diagnostic = executionbackend.OperationUnknown, startErr.Error()
		if handle != nil {
			receipt.Handle = handle.Detach()
		}
		return receipt, nil, startErr
	}
	receipt.Disposition = executionbackend.OperationAccepted
	receipt.State.InputAccepted = true
	receipt.State.TurnStarted = true
	receipt.EventPath = handle.Paths.JournalPath
	fixedContext := lifecycleSession(snapshot, binding.SessionID, turnID)
	return receipt, newLifecycleObserver(observer, fixedContext), nil
}

func sameBinding(a, b Binding) bool {
	return a.Process != nil && b.Process != nil && *a.Process == *b.Process &&
		a.Endpoint == b.Endpoint && a.Generation == b.Generation && a.SessionID == b.SessionID
}

func (l ExecutionLifecycle) writeHistoryBinding(snapshot executionbackend.LifecycleSnapshot, binding Binding, handle *SessionHandle) error {
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
		AttemptID: snapshot.Session.AttemptID, Backend: executionbackend.BackendLocal,
		Protocol: executionbackend.ProtocolLocalCodexAppServer, TargetID: snapshot.Ref.Target.ID,
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

func (l ExecutionLifecycle) validateIdentity(snapshot executionbackend.LifecycleSnapshot) error {
	if snapshot.Session.ProjectID == "" || snapshot.Session.ExecutionID == "" || snapshot.Session.AttemptID == "" ||
		snapshot.Session.WorkflowRunID == "" || snapshot.ConfigurationDigest == "" || snapshot.ResourceGeneration == "" {
		return errors.New("app-server lifecycle has no fixed execution identity")
	}
	return nil
}

func (l ExecutionLifecycle) identity(snapshot executionbackend.LifecycleSnapshot) AppServerBindingIdentity {
	return AppServerBindingIdentity{
		ProjectID: snapshot.Session.ProjectID, ExecutionID: snapshot.Session.ExecutionID,
		AttemptID: snapshot.Session.AttemptID, Generation: snapshot.ResourceGeneration,
		ConfigDigest: snapshot.ConfigurationDigest,
	}
}

func (l ExecutionLifecycle) readBinding(snapshot executionbackend.LifecycleSnapshot) (Binding, bool, error) {
	var envelope *executionbackend.ImplementationDataEnvelope
	for i := range snapshot.Implementation {
		if snapshot.Implementation[i].Purpose != executionbackend.ImplementationResourcePurpose || snapshot.Implementation[i].ImplementationKey != ExecutionBindingCodecKey {
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

func (l ExecutionLifecycle) bindingReceipt(snapshot executionbackend.LifecycleSnapshot, binding Binding) executionbackend.LifecycleReceipt {
	receipt := executionbackend.LifecycleReceipt{}
	if binding.Process != nil {
		process := *binding.Process
		receipt.Process = &process
	}
	if binding.Endpoint != "" || binding.SessionID != "" || binding.Generation != "" {
		if envelope, err := NewAppServerBindingEnvelope(l.identity(snapshot), AppServerBinding{Endpoint: binding.Endpoint, SessionID: binding.SessionID, Generation: binding.Generation}); err == nil {
			receipt.Implementation = []executionbackend.ImplementationDataEnvelope{envelope}
		}
	}
	return receipt
}

func optionalProcess(identity executionbackend.ProcessIdentity) *executionbackend.ProcessIdentity {
	if identity == (executionbackend.ProcessIdentity{}) {
		return nil
	}
	copy := identity
	return &copy
}

func cloneTurn(turn *executionbackend.TurnInput) *executionbackend.TurnInput {
	if turn == nil {
		return nil
	}
	copy := *turn
	copy.LinkedWorktreeArgs = append([]string(nil), turn.LinkedWorktreeArgs...)
	return &copy
}

func lifecycleSession(snapshot executionbackend.LifecycleSnapshot, sessionID, turnID string) executionbackend.SessionContext {
	fixed := snapshot.Session
	fixed.SessionID, fixed.CurrentTurnID = sessionID, turnID
	return fixed
}

type lifecycleObserver struct {
	original *Observation
	events   chan executionbackend.LifecycleEvent
	fixed    executionbackend.SessionContext
}

func newLifecycleObserver(original *Observation, fixed executionbackend.SessionContext) executionbackend.LifecycleObserver {
	observer := &lifecycleObserver{original: original, events: make(chan executionbackend.LifecycleEvent, cap(original.Events)), fixed: fixed}
	go observer.forward()
	return observer
}

func (observer *lifecycleObserver) Events() <-chan executionbackend.LifecycleEvent {
	return observer.events
}

func (observer *lifecycleObserver) Done() <-chan error { return observer.original.Done }

func (observer *lifecycleObserver) StopHandle() executionbackend.StopHandle {
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
		fact := executionbackend.EventFact{}
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
			fact.Kind = executionbackend.EventStatus
			fact.Source = "execution"
			fact.SourceEventID = value.Envelope.Method + ":" + digestValue(value.Envelope)
		}
		persisted := make(chan error, 1)
		event := executionbackend.LifecycleEvent{Fact: fact, Summary: value.Summary, Kind: value.Kind, Status: value.Status, Title: value.Title, Persisted: persisted}
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

func (l ExecutionLifecycle) InspectLifecycleWorkspace(ctx context.Context, snapshot executionbackend.LifecycleSnapshot) (executionbackend.WorkspaceInspection, error) {
	return workspace.InspectLifecycleWorkspace(ctx, l.setup, snapshot)
}
