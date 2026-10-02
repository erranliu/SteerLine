package orcaworker

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/workspace"
)

// ExecutionLifecycle owns native Orca requests and interprets their receipts.
// It receives only immutable setup and one center-persisted action at a time.
type ExecutionLifecycle struct {
	setup  executionapi.LifecycleSetup
	config Config
	client Client
	codex  codexworker.ProfileConfig
	orca   ProfileConfig
}

func NewExecutionLifecycle(setup executionapi.LifecycleSetup) (executionapi.LifecyclePort, error) {
	setup = setup.Clone()
	ref := setup.Snapshot.Ref
	if ref.Backend != executionapi.BackendOrca || ref.Protocol != executionapi.ProtocolOrcaTerminalV2 || ref.Agent.Name != "codex" || setup.ProcessHost == nil {
		return nil, &executionapi.BackendError{Code: executionapi.BackendUnsupported, Diagnostic: "native Orca lifecycle setup differs from its fixed target"}
	}
	profile := func(key string) ([]byte, error) {
		var found *executionapi.ImplementationDataEnvelope
		for i := range setup.ProfileData {
			if setup.ProfileData[i].ImplementationKey != key {
				continue
			}
			if found != nil {
				return nil, fmt.Errorf("duplicate implementation profile %q", key)
			}
			found = &setup.ProfileData[i]
		}
		if found == nil {
			return nil, fmt.Errorf("missing implementation profile %q", key)
		}
		if err := found.ValidateProfileIdentity(setup.Snapshot.Session.ProjectID, setup.ProfileID, setup.AcceptedConfigVersion); err != nil {
			return nil, err
		}
		if found.CodecVersion != 1 || found.Purpose != executionapi.ImplementationConfigPurpose {
			return nil, fmt.Errorf("unsupported implementation profile %q", key)
		}
		return append([]byte(nil), found.Payload...), nil
	}
	codexPayload, err := profile(codexworker.ProfileCodecKey)
	if err != nil {
		return nil, err
	}
	codex, err := codexworker.DecodeProfileConfig(codexPayload)
	if err != nil {
		return nil, err
	}
	orcaPayload, err := profile(ProfileCodecKey)
	if err != nil {
		return nil, err
	}
	orca, err := DecodeProfileConfig(orcaPayload)
	if err != nil {
		return nil, err
	}
	if err := orca.Validate(); err != nil {
		return nil, err
	}
	if ref.Target.ID != nativeTargetID(orca) {
		return nil, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "native Orca profile differs from the fixed target"}
	}
	timeout := setup.CommandTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	output := func(ctx context.Context, commandTimeout time.Duration, args ...string) ([]byte, error) {
		if commandTimeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, commandTimeout)
			defer cancel()
		}
		all := append(append([]string(nil), orca.Args...), args...)
		return setup.ProcessHost.OutputCommand(ctx, executionapi.LaunchCommand{
			Executable: orca.Executable, Args: all, WorkingDirectory: setup.RepositoryPath,
		})
	}
	config := Config{Repository: setup.RepositoryPath, CommandTimeout: timeout.String(), Target: ref.Target, Output: output}
	if codex.Model != "" {
		config.AgentVersion = func(ctx context.Context) (string, error) {
			out, err := setup.ProcessHost.OutputCommand(ctx, executionapi.LaunchCommand{
				Executable: codex.Executable, Args: append(append([]string(nil), codex.Args...), "--version"), WorkingDirectory: setup.RepositoryPath,
			})
			return strings.TrimSpace(string(out)), err
		}
	}
	return &ExecutionLifecycle{setup: setup, config: config, client: New(config), codex: codex, orca: orca}, nil
}

func nativeTargetID(config ProfileConfig) string {
	return "orca:" + digestValue(struct {
		Executable string
		Args       []string
	}{config.Executable, append([]string(nil), config.Args...)})[:24]
}

func (l *ExecutionLifecycle) PolicyCapabilities() executionapi.LifecyclePolicyCapabilities {
	return executionapi.LifecyclePolicyCapabilities{LinkedWorkspaceWriteRoots: true, SeparateTurnStart: true, ActiveTurnFeedback: true, ResumeActiveObservation: true}
}

func (l *ExecutionLifecycle) PreflightLifecycle(ctx context.Context, setup executionapi.LifecycleSetup) (executionapi.CapabilityReport, error) {
	if err := sameNativeTarget(l.setup.Snapshot.Ref, setup.Snapshot.Ref); err != nil {
		return executionapi.CapabilityReport{}, err
	}
	if err := ValidateProfileSelection(setup.Policy, setup.Snapshot.Ref.Agent.Name, mustEncodeCodex(l.codex), mustEncodeOrca(l.orca)); err != nil {
		return executionapi.CapabilityReport{}, &executionapi.BackendError{Code: executionapi.BackendUnsupported, Diagnostic: err.Error(), Cause: err}
	}
	if err := CheckStatus(ctx, l.client.Call); err != nil {
		return executionapi.CapabilityReport{}, &executionapi.BackendError{Code: executionapi.BackendUnreachable, Diagnostic: "Orca status probe failed", Cause: err}
	}
	if _, err := FindRepo(ctx, l.client.Call, setup.RepositoryPath); err != nil {
		return executionapi.CapabilityReport{}, &executionapi.BackendError{Code: executionapi.BackendUnsupported, Diagnostic: err.Error(), Cause: err}
	}
	if l.fixedAgentLaunch() {
		command, err := l.agentCommand(setup.RepositoryPath)
		if err != nil {
			return executionapi.CapabilityReport{}, &executionapi.BackendError{Code: executionapi.BackendUnsupported, Diagnostic: err.Error(), Cause: err}
		}
		if err := codexworker.ProbeManagedCLI(ctx, contracts.Command{Executable: command.Executable, Args: command.Args}, func(ctx context.Context, executable string, args []string) ([]byte, error) {
			return l.setup.ProcessHost.OutputCommand(ctx, executionapi.LaunchCommand{Executable: executable, Args: append([]string(nil), args...), WorkingDirectory: setup.RepositoryPath})
		}); err != nil {
			return executionapi.CapabilityReport{}, err
		}
	}
	report, err := NewBackend(l.config).Probe(ctx, setup.Snapshot.Ref.Target, setup.Snapshot.Ref.Agent)
	if err != nil {
		return report, err
	}
	for i := range report.Capabilities {
		if report.Capabilities[i].Name == "start" {
			report.Capabilities[i].Status = executionapi.CapabilityVerified
			report.Capabilities[i].Evidence = "native Orca lifecycle preflight"
		}
	}
	return report, nil
}

func mustEncodeCodex(config codexworker.ProfileConfig) []byte {
	value, _ := codexworker.EncodeProfileConfig(config)
	return value
}
func mustEncodeOrca(config ProfileConfig) []byte {
	value, _ := EncodeProfileConfig(config)
	return value
}

func (l *ExecutionLifecycle) Next(_ context.Context, snapshot executionapi.LifecycleSnapshot) (executionapi.LifecycleCandidate, error) {
	if err := l.validateSnapshot(snapshot); err != nil {
		return executionapi.LifecycleCandidate{}, err
	}
	state, _, err := l.readState(snapshot)
	if err != nil {
		return executionapi.LifecycleCandidate{}, err
	}
	if snapshot.WorkflowPhase == "completed" && snapshot.TaskDelivered {
		for _, turn := range state.Turns {
			if !turn.Settled {
				return executionapi.LifecycleCandidate{Done: true}, nil
			}
		}
		if snapshot.Ref.External.Terminal != "" && !state.AgentClosed {
			return nativeCleanupCandidate(snapshot.Ref.External), nil
		}
		if state.Coordinator.Terminal != "" && !state.CoordinatorClosed {
			return nativeCleanupCandidate(state.Coordinator), nil
		}
		return executionapi.LifecycleCandidate{Done: true}, nil
	}
	if snapshot.Workspace.Path == "" {
		if snapshot.WorkspacePlan.BaseCommit == "" {
			return executionapi.LifecycleCandidate{}, errors.New("native Orca has no fixed workspace plan")
		}
		candidate := executionapi.LifecycleCandidate{Action: executionapi.LifecyclePrepareWorkspace,
			Input: executionapi.LifecycleInput{Workspace: nativeWorkspace(&snapshot.WorkspacePlan)}}
		return candidate, candidate.Validate()
	}
	if snapshot.Ref.Target.WorktreeID == "" {
		return executionapi.LifecycleCandidate{}, errors.New("native Orca workspace receipt has no Worktree identity")
	}
	if !state.AgentBound {
		purpose := "agent-bind"
		if snapshot.Ref.External.Terminal == "" {
			purpose = "agent-terminal"
		}
		return nativeResourceCandidate(purpose, FixedAgentTitle(snapshot.Session.ExecutionID), "", ""), nil
	}
	if state.Coordinator.Terminal == "" {
		return nativeResourceCandidate("coordinator", "SteerLine coordinator "+snapshot.Session.WorkflowRunID, "", ""), nil
	}
	if snapshot.Ref.Target.OrchestrationRun == "" {
		return nativeResourceCandidate("workflow", "", "SteerLine worker "+snapshot.Session.ExecutionID, ""), nil
	}
	if !state.RelayIgnored {
		return nativeResourceCandidate("relay-ignore", "", "", ""), nil
	}
	if snapshot.PendingTurn == nil {
		return executionapi.LifecycleCandidate{Done: true}, nil
	}
	turn := nativeTurn(&state, snapshot.PendingTurn.CommandID)
	if turn == nil || turn.TaskID == "" {
		candidate := nativeResourceCandidate("conversation", snapshot.TaskTitle, snapshot.PendingTurn.Prompt, snapshot.PendingTurn.CommandID)
		candidate.Input.Resource.TurnPurpose = snapshot.PendingTurn.Purpose
		return candidate, candidate.Validate()
	}
	if turn.DispatchID == "" {
		candidate := executionapi.LifecycleCandidate{Action: executionapi.LifecycleStartTurn, Input: executionapi.LifecycleInput{Turn: nativeTurnInput(snapshot.PendingTurn)}}
		return candidate, candidate.Validate()
	}
	if turn.InputSubmissionPending {
		candidate := executionapi.LifecycleCandidate{Action: executionapi.LifecycleSubmitInput, Input: executionapi.LifecycleInput{Turn: nativeTurnInput(snapshot.PendingTurn)}}
		return candidate, candidate.Validate()
	}
	return executionapi.LifecycleCandidate{Done: true}, nil
}

func nativeWorkspace(value *executionapi.WorkspaceIntent) *executionapi.WorkspaceIntent {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func nativeTurnInput(value *executionapi.TurnInput) *executionapi.TurnInput {
	if value == nil {
		return nil
	}
	copy := *value
	copy.LinkedWorktreeArgs = append([]string(nil), value.LinkedWorktreeArgs...)
	return &copy
}

func nativeResourceCandidate(purpose, name, objective, turnID string) executionapi.LifecycleCandidate {
	return executionapi.LifecycleCandidate{Action: executionapi.LifecycleCreateResource,
		Input: executionapi.LifecycleInput{Resource: &executionapi.ResourceIntent{Purpose: purpose, Name: name, Objective: objective, TurnID: turnID}}}
}

func nativeCleanupCandidate(identity executionapi.ProcessIdentity) executionapi.LifecycleCandidate {
	return executionapi.LifecycleCandidate{Action: executionapi.LifecycleCleanup,
		Input: executionapi.LifecycleInput{Cleanup: &executionapi.CleanupInput{Resources: []executionapi.ProcessIdentity{identity}}}}
}

func (l *ExecutionLifecycle) Execute(ctx context.Context, request executionapi.LifecycleRequest) (executionapi.LifecycleReceipt, executionapi.LifecycleObserver, error) {
	if err := request.Validate(); err != nil {
		return nativeReceipt(request, executionapi.OperationRejected), nil, err
	}
	if err := l.validateSnapshot(request.Snapshot); err != nil {
		return nativeReceipt(request, executionapi.OperationRejected), nil, err
	}
	switch executionapi.LifecycleAction(request.Operation.Action) {
	case executionapi.LifecyclePrepareWorkspace:
		receipt, err := l.prepareWorkspace(ctx, request)
		return receipt, nil, err
	case executionapi.LifecycleCreateResource:
		receipt, err := l.createResource(ctx, request)
		return receipt, nil, err
	case executionapi.LifecycleStartTurn:
		return l.startTurn(ctx, request)
	case executionapi.LifecycleSubmitInput:
		return l.submitInput(ctx, request)
	case executionapi.LifecycleReportCompletion:
		receipt, err := l.reportCompletion(ctx, request)
		return receipt, nil, err
	case executionapi.LifecycleDeliverFeedback:
		receipt, err := l.deliverFeedback(ctx, request)
		return receipt, nil, err
	case executionapi.LifecycleAcknowledge:
		receipt, err := l.acknowledge(ctx, request)
		return receipt, nil, err
	case executionapi.LifecycleCleanup:
		receipt, err := l.cleanup(ctx, request)
		return receipt, nil, err
	case executionapi.LifecycleHostAction:
		receipt, err := l.ExecuteHostAction(ctx, request)
		return receipt, nil, err
	default:
		return nativeReceipt(request, executionapi.OperationRejected), nil,
			executionapi.UnsupportedAdapterOperation(request.Operation.Action, "native Orca action is unsupported")
	}
}

func (l *ExecutionLifecycle) Reconcile(ctx context.Context, request executionapi.LifecycleRequest) (executionapi.LifecycleReceipt, executionapi.LifecycleObserver, error) {
	if err := request.Validate(); err != nil {
		return nativeReceipt(request, executionapi.OperationRejected), nil, err
	}
	if err := l.validateSnapshot(request.Snapshot); err != nil {
		return nativeReceipt(request, executionapi.OperationRejected), nil, err
	}
	switch executionapi.LifecycleAction(request.Operation.Action) {
	case executionapi.LifecyclePrepareWorkspace:
		return l.reconcileWorkspace(ctx, request)
	case executionapi.LifecycleCreateResource:
		return l.reconcileResource(ctx, request)
	case executionapi.LifecycleStartTurn:
		return l.reconcileStart(ctx, request)
	case executionapi.LifecycleSubmitInput:
		return l.reconcileInput(ctx, request)
	case executionapi.LifecycleReportCompletion:
		return l.reconcileCompletion(ctx, request)
	case executionapi.LifecycleDeliverFeedback:
		return l.reconcileFeedback(ctx, request)
	case executionapi.LifecycleAcknowledge:
		return l.reconcileAck(ctx, request)
	case executionapi.LifecycleCleanup:
		return l.reconcileCleanup(ctx, request)
	case executionapi.LifecycleHostAction:
		receipt, err := l.ReconcileHostAction(ctx, request)
		return receipt, nil, err
	default:
		return nativeReceipt(request, executionapi.OperationUnknown), nil, nil
	}
}

func (l *ExecutionLifecycle) validateSnapshot(snapshot executionapi.LifecycleSnapshot) error {
	if snapshot.Ref.Backend != executionapi.BackendOrca || snapshot.Ref.Protocol != executionapi.ProtocolOrcaTerminalV2 ||
		snapshot.Ref.Agent.Name != "codex" || snapshot.Ref.Target.ID != nativeTargetID(l.orca) ||
		snapshot.ConfigurationDigest == "" || snapshot.ResourceGeneration == "" ||
		snapshot.Session.ProjectID == "" || snapshot.Session.ExecutionID == "" || snapshot.Session.WorkflowRunID == "" ||
		snapshot.Session.AttemptID == "" || snapshot.Session.TaskID == "" {
		return &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "native Orca snapshot differs from the fixed execution"}
	}
	if snapshot.Ref.RunID != "" && snapshot.Ref.RunID != snapshot.Session.WorkflowRunID ||
		snapshot.Ref.AttemptID != "" && snapshot.Ref.AttemptID != snapshot.Session.AttemptID {
		return &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "native Orca snapshot Run or Attempt changed"}
	}
	if snapshot.Ref.External.Kind != "" && snapshot.Ref.External.Kind != "orca" {
		return &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "native Orca process kind is not generic"}
	}
	if snapshot.Workspace.Path != "" && snapshot.Ref.WorkspacePath != "" && snapshot.Workspace.Path != snapshot.Ref.WorkspacePath {
		return &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "native Orca workspace differs from its fixed reference"}
	}
	if snapshot.PendingTurn != nil && (snapshot.PendingTurn.CommandID == "" || snapshot.PendingTurn.Prompt == "") {
		return errors.New("native Orca input has no stable command or prompt")
	}
	_, _, err := l.readState(snapshot)
	return err
}

func (l *ExecutionLifecycle) readState(snapshot executionapi.LifecycleSnapshot) (NativeExecutionState, bool, error) {
	var found *executionapi.ImplementationDataEnvelope
	for i := range snapshot.Implementation {
		if snapshot.Implementation[i].ImplementationKey != ExecutionStateCodecKey {
			continue
		}
		if found != nil {
			return NativeExecutionState{}, false, errors.New("duplicate native Orca state envelopes")
		}
		found = &snapshot.Implementation[i]
	}
	if found == nil {
		return NativeExecutionState{}, false, nil
	}
	identity := ExecutionStateIdentity{ProjectID: snapshot.Session.ProjectID, ExecutionID: snapshot.Session.ExecutionID,
		AttemptID: snapshot.Session.AttemptID, Generation: snapshot.ResourceGeneration,
		ConfigDigest: snapshot.ConfigurationDigest, Protocol: snapshot.Ref.Protocol, Process: snapshot.Ref.External,
		CoordinatorTerminal: snapshot.Ref.Target.CoordinatorTerminal}
	state, err := DecodeExecutionStateEnvelope(*found, identity)
	return state, true, err
}

func (l *ExecutionLifecycle) stateEnvelope(snapshot executionapi.LifecycleSnapshot, target executionapi.ExecutionTarget,
	process executionapi.ProcessIdentity, state NativeExecutionState) (executionapi.ImplementationDataEnvelope, error) {
	return NewExecutionStateEnvelope(ExecutionStateIdentity{ProjectID: snapshot.Session.ProjectID,
		ExecutionID: snapshot.Session.ExecutionID, AttemptID: snapshot.Session.AttemptID,
		Generation: snapshot.ResourceGeneration, ConfigDigest: snapshot.ConfigurationDigest,
		Protocol: snapshot.Ref.Protocol, Process: process, CoordinatorTerminal: target.CoordinatorTerminal}, state)
}

func (l *ExecutionLifecycle) targetEnvelope(snapshot executionapi.LifecycleSnapshot, target executionapi.ExecutionTarget,
	process executionapi.ProcessIdentity, state NativeExecutionState) ([]executionapi.ImplementationDataEnvelope, error) {
	envelope, err := l.stateEnvelope(snapshot, target, process, state)
	if err != nil {
		return nil, err
	}
	return []executionapi.ImplementationDataEnvelope{envelope}, nil
}

func nativeReceipt(request executionapi.LifecycleRequest, disposition executionapi.OperationStatus) executionapi.LifecycleReceipt {
	return executionapi.LifecycleReceipt{Operation: request.Operation, Disposition: disposition}
}

func (l *ExecutionLifecycle) fixedAgentLaunch() bool {
	return l.codex.Model != "" || l.codex.WindowsSandbox != ""
}

func (l *ExecutionLifecycle) agentCommand(worktree string) (executionapi.LaunchCommand, error) {
	permissions, err := codexworker.CompilePolicy(l.setup.Policy, runtime.GOOS, "orca", "codex", l.codex.WindowsSandbox, l.codex.Profile, l.codex.Args)
	if err != nil {
		return executionapi.LaunchCommand{}, err
	}
	if err := codexworker.ValidateManagedArgs(l.codex.Args); err != nil {
		return executionapi.LaunchCommand{}, err
	}
	executable, err := codexworker.NativeExecutable(l.codex.Executable)
	if err != nil {
		return executionapi.LaunchCommand{}, err
	}
	command, err := codexworker.ManagedCommand(codexworker.ManagedCommandOptions{Policy: l.setup.Policy,
		WorkingDirectory: worktree, Host: "orca", Agent: "codex", Model: l.codex.Model,
		ReasoningEffort: l.codex.ReasoningEffort, Executable: executable, Args: l.codex.Args,
		Sandbox: permissions.Sandbox, WindowsSandbox: permissions.WindowsSandbox})
	if err != nil {
		return executionapi.LaunchCommand{}, err
	}
	return command, nil
}

func (l *ExecutionLifecycle) agentLaunchCommand(snapshot executionapi.LifecycleSnapshot, worktree string) (executionapi.LaunchCommand, error) {
	if snapshot.PolicyResources == nil {
		return l.agentCommand(worktree)
	}
	identity := codexworker.PolicyResourceIdentity{ProjectID: snapshot.Session.ProjectID, ExecutionID: snapshot.Session.ExecutionID,
		AttemptID: snapshot.Session.AttemptID, Generation: snapshot.ResourceGeneration, ConfigDigest: snapshot.ConfigurationDigest}
	resources, err := codexworker.DecodePolicyResourceEnvelope(*snapshot.PolicyResources, identity)
	if err != nil {
		return executionapi.LaunchCommand{}, err
	}
	if resources.Settings.WorkingDirectory != worktree {
		return executionapi.LaunchCommand{}, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "native Orca launch command is bound to a different Worktree"}
	}
	return resources.Settings, nil
}

func (l *ExecutionLifecycle) worktreeName(snapshot executionapi.LifecycleSnapshot) (string, error) {
	return WorkerWorktreeName(snapshot.Session.TaskID, snapshot.Session.WorkflowRunID)
}

func (l *ExecutionLifecycle) repositoryID(ctx context.Context) (string, error) {
	return FindRepo(ctx, l.client.Call, l.setup.RepositoryPath)
}

func (l *ExecutionLifecycle) processGeneration(snapshot executionapi.LifecycleSnapshot) string {
	if snapshot.Ref.External.Incarnation != "" {
		return snapshot.Ref.External.Incarnation
	}
	return snapshot.ResourceGeneration
}

func (l *ExecutionLifecycle) boundProcess(snapshot executionapi.LifecycleSnapshot, target executionapi.ExecutionTarget, turn *NativeTurnState) executionapi.ProcessIdentity {
	process := snapshot.Ref.External
	process.Kind = "orca"
	process.WorktreeID = target.WorktreeID
	if turn != nil {
		process.Task, process.Dispatch = turn.TaskID, turn.DispatchID
		process.OrchestrationRun = target.OrchestrationRun
	}
	return process
}

var _ executionapi.LifecyclePort = (*ExecutionLifecycle)(nil)

func (l *ExecutionLifecycle) InspectLifecycleWorkspace(ctx context.Context, snapshot executionapi.LifecycleSnapshot) (executionapi.WorkspaceInspection, error) {
	return workspace.InspectLifecycleWorkspace(ctx, l.setup, snapshot)
}
