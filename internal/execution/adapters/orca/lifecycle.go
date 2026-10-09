package orcaworker

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	"github.com/erranliu/SteerLine/internal/execution/workspace"
)

// ExecutionLifecycle owns native Orca requests and interprets their receipts.
// It receives only immutable setup and one center-persisted action at a time.
type ExecutionLifecycle struct {
	setup  executionbackend.LifecycleSetup
	config Config
	client Client
	orca   ProfileConfig
}

func NewExecutionLifecycle(setup executionbackend.LifecycleSetup) (executionbackend.LifecyclePort, error) {
	setup = setup.Clone()
	ref := setup.Snapshot.Ref
	if ref.Backend != executionbackend.BackendOrca || ref.Protocol != executionbackend.ProtocolOrcaTerminalV2 || strings.TrimSpace(ref.Agent.Name) == "" || setup.ProcessHost == nil {
		return nil, &executionbackend.BackendError{Code: executionbackend.BackendUnsupported, Diagnostic: "native Orca lifecycle setup differs from its fixed target"}
	}
	profile := func(key string) ([]byte, error) {
		var found *executionbackend.ImplementationDataEnvelope
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
		if found.CodecVersion != ProfileCodecVersion || found.Purpose != executionbackend.ImplementationConfigPurpose {
			return nil, fmt.Errorf("unsupported implementation profile %q", key)
		}
		return append([]byte(nil), found.Payload...), nil
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
	if ref.Target.ID != NativeTargetID(orca) {
		return nil, &executionbackend.BackendError{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "native Orca profile differs from the fixed target"}
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
		return setup.ProcessHost.OutputCommand(ctx, executionbackend.LaunchCommand{
			Executable: orca.Executable, Args: all, WorkingDirectory: setup.RepositoryPath, Timeout: commandTimeout,
		})
	}
	config := Config{Repository: setup.RepositoryPath, CommandTimeout: timeout.String(), Target: ref.Target, Output: output}
	return &ExecutionLifecycle{setup: setup, config: config, client: New(config), orca: orca}, nil
}

func (l *ExecutionLifecycle) PolicyCapabilities() executionbackend.LifecyclePolicyCapabilities {
	return executionbackend.LifecyclePolicyCapabilities{LinkedWorkspaceWriteRoots: false, SeparateTurnStart: true, ActiveTurnFeedback: true, ResumeActiveObservation: true}
}

func (l *ExecutionLifecycle) PreflightLifecycle(ctx context.Context, setup executionbackend.LifecycleSetup) (executionbackend.CapabilityReport, error) {
	if err := sameNativeTarget(l.setup.Snapshot.Ref, setup.Snapshot.Ref); err != nil {
		return executionbackend.CapabilityReport{}, err
	}
	if err := ValidateProfileSelection(setup.Snapshot.Ref.Agent.Name, mustEncodeOrca(l.orca)); err != nil {
		return executionbackend.CapabilityReport{}, &executionbackend.BackendError{Code: executionbackend.BackendUnsupported, Diagnostic: err.Error(), Cause: err}
	}
	if err := CheckStatus(ctx, l.client.Call); err != nil {
		return executionbackend.CapabilityReport{}, &executionbackend.BackendError{Code: executionbackend.BackendUnreachable, Diagnostic: "Orca status probe failed", Cause: err}
	}
	if _, err := FindRepo(ctx, l.client.Call, setup.RepositoryPath); err != nil {
		return executionbackend.CapabilityReport{}, &executionbackend.BackendError{Code: executionbackend.BackendUnsupported, Diagnostic: err.Error(), Cause: err}
	}
	report, err := NewBackend(l.config).Probe(ctx, setup.Snapshot.Ref.Target, setup.Snapshot.Ref.Agent)
	if err != nil {
		return report, err
	}
	for i := range report.Capabilities {
		if report.Capabilities[i].Name == "start" {
			report.Capabilities[i].Status = executionbackend.CapabilityVerified
			report.Capabilities[i].Evidence = "native Orca lifecycle preflight"
		}
	}
	return report, nil
}

func mustEncodeOrca(config ProfileConfig) []byte {
	value, _ := EncodeProfileConfig(config)
	return value
}

func (l *ExecutionLifecycle) Next(ctx context.Context, snapshot executionbackend.LifecycleSnapshot) (executionbackend.LifecycleCandidate, error) {
	if err := l.validateSnapshot(snapshot); err != nil {
		return executionbackend.LifecycleCandidate{}, err
	}
	state, _, err := l.readState(snapshot)
	if err != nil {
		return executionbackend.LifecycleCandidate{}, err
	}
	if state.PartialWorker != nil || state.FailedStartReleased {
		return executionbackend.LifecycleCandidate{Done: true}, nil
	}
	if snapshot.WorkflowPhase == "completed" && snapshot.TaskDelivered {
		for _, turn := range state.Turns {
			if !turn.Settled {
				return executionbackend.LifecycleCandidate{Done: true}, nil
			}
		}
		if snapshot.Ref.External.Terminal != "" && !cleanupConfirmed(snapshot, snapshot.Ref.External) {
			return nativeCleanupCandidate(snapshot.Ref.External), nil
		}
		if state.Coordinator.Terminal != "" && !cleanupConfirmed(snapshot, state.Coordinator) {
			return nativeCleanupCandidate(state.Coordinator), nil
		}
		return executionbackend.LifecycleCandidate{Done: true}, nil
	}
	if snapshot.Workspace.Path == "" {
		if snapshot.WorkspacePlan.BaseCommit == "" {
			return executionbackend.LifecycleCandidate{}, errors.New("native Orca has no fixed workspace plan")
		}
		candidate := executionbackend.LifecycleCandidate{Action: executionbackend.LifecyclePrepareWorkspace,
			Input: executionbackend.LifecycleInput{Workspace: nativeWorkspace(&snapshot.WorkspacePlan)}}
		return candidate, candidate.Validate()
	}
	if snapshot.Ref.Target.WorktreeID == "" {
		return executionbackend.LifecycleCandidate{}, errors.New("native Orca workspace receipt has no Worktree identity")
	}
	if state.Coordinator.Terminal == "" {
		return nativeResourceCandidate("coordinator", "SteerLine coordinator "+snapshot.Session.WorkflowRunID, "", ""), nil
	}
	if snapshot.Ref.Target.OrchestrationRun == "" {
		return nativeResourceCandidate("workflow", "", "SteerLine worker "+snapshot.Session.ExecutionID, ""), nil
	}
	if snapshot.PendingTurn == nil {
		return executionbackend.LifecycleCandidate{Done: true}, nil
	}
	turn := nativeTurn(&state, snapshot.PendingTurn.CommandID)
	if turn == nil || turn.TaskID == "" {
		candidate := nativeResourceCandidate("conversation", snapshot.TaskTitle, snapshot.PendingTurn.Prompt, snapshot.PendingTurn.CommandID)
		candidate.Input.Resource.TurnPurpose = snapshot.PendingTurn.Purpose
		return candidate, candidate.Validate()
	}
	if turn.DispatchID == "" {
		candidate := executionbackend.LifecycleCandidate{Action: executionbackend.LifecycleStartTurn, Input: executionbackend.LifecycleInput{Turn: nativeTurnInput(snapshot.PendingTurn)}}
		return candidate, candidate.Validate()
	}
	return executionbackend.LifecycleCandidate{Done: true}, nil
}

func nativeWorkspace(value *executionbackend.WorkspaceIntent) *executionbackend.WorkspaceIntent {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func nativeTurnInput(value *executionbackend.TurnInput) *executionbackend.TurnInput {
	if value == nil {
		return nil
	}
	copy := *value
	copy.LinkedWorktreeArgs = append([]string(nil), value.LinkedWorktreeArgs...)
	return &copy
}

func nativeResourceCandidate(purpose, name, objective, turnID string) executionbackend.LifecycleCandidate {
	return executionbackend.LifecycleCandidate{Action: executionbackend.LifecycleCreateResource,
		Input: executionbackend.LifecycleInput{Resource: &executionbackend.ResourceIntent{Purpose: purpose, Name: name, Objective: objective, TurnID: turnID}}}
}

func nativeCleanupCandidate(identity executionbackend.ProcessIdentity) executionbackend.LifecycleCandidate {
	return executionbackend.LifecycleCandidate{Action: executionbackend.LifecycleCleanup,
		Input: executionbackend.LifecycleInput{Cleanup: &executionbackend.CleanupInput{Resources: []executionbackend.ProcessIdentity{identity}}}}
}

// Delivery evidence stays frozen. Cleanup progress comes from the actor's
// committed original operation receipts, not a rewrite of native turn state.
func cleanupConfirmed(snapshot executionbackend.LifecycleSnapshot, identity executionbackend.ProcessIdentity) bool {
	for _, operation := range snapshot.Operations {
		if operation.Action != string(executionbackend.LifecycleCleanup) || operation.Status != executionbackend.OperationConfirmed || operation.Ref == nil || *operation.Ref != snapshot.Ref || operation.LifecycleIdentity == nil || operation.LifecycleIdentity.ExecutionID != snapshot.Session.ExecutionID {
			continue
		}
		candidate, err := executionbackend.CandidateForOperation(operation)
		if err == nil && candidate.Input.Cleanup != nil && len(candidate.Input.Cleanup.Resources) == 1 && candidate.Input.Cleanup.Resources[0] == identity {
			return true
		}
	}
	return false
}

func (l *ExecutionLifecycle) Execute(ctx context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, executionbackend.LifecycleObserver, error) {
	if err := request.Validate(); err != nil {
		return nativeReceipt(request, executionbackend.OperationRejected), nil, err
	}
	if err := l.validateSnapshot(request.Snapshot); err != nil {
		return nativeReceipt(request, executionbackend.OperationRejected), nil, err
	}
	switch executionbackend.LifecycleAction(request.Operation.Action) {
	case executionbackend.LifecyclePrepareWorkspace:
		receipt, err := l.prepareWorkspace(ctx, request)
		return receipt, nil, err
	case executionbackend.LifecycleCreateResource:
		receipt, err := l.createResource(ctx, request)
		return receipt, nil, err
	case executionbackend.LifecycleStartTurn:
		return l.startTurn(ctx, request)
	case executionbackend.LifecycleDeliverFeedback:
		receipt, err := l.deliverFeedback(ctx, request)
		return receipt, nil, err
	case executionbackend.LifecycleAcknowledge:
		receipt, err := l.acknowledge(ctx, request)
		return receipt, nil, err
	case executionbackend.LifecycleCleanup:
		receipt, err := l.cleanup(ctx, request)
		return receipt, nil, err
	default:
		return nativeReceipt(request, executionbackend.OperationRejected), nil,
			executionbackend.UnsupportedAdapterOperation(request.Operation.Action, "native Orca action is unsupported")
	}
}

func (l *ExecutionLifecycle) Reconcile(ctx context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, executionbackend.LifecycleObserver, error) {
	if err := request.Validate(); err != nil {
		return nativeReceipt(request, executionbackend.OperationRejected), nil, err
	}
	if err := l.validateSnapshot(request.Snapshot); err != nil {
		return nativeReceipt(request, executionbackend.OperationRejected), nil, err
	}
	switch executionbackend.LifecycleAction(request.Operation.Action) {
	case executionbackend.LifecyclePrepareWorkspace:
		return l.reconcileWorkspace(ctx, request)
	case executionbackend.LifecycleCreateResource:
		return l.reconcileResource(ctx, request)
	case executionbackend.LifecycleStartTurn:
		return l.reconcileStart(ctx, request)
	case executionbackend.LifecycleDeliverFeedback:
		return l.reconcileFeedback(ctx, request)
	case executionbackend.LifecycleAcknowledge:
		return l.reconcileAck(ctx, request)
	case executionbackend.LifecycleCleanup:
		return l.reconcileCleanup(ctx, request)
	case executionbackend.LifecycleStopExecution:
		return l.reconcileStop(ctx, request)
	default:
		return nativeReceipt(request, executionbackend.OperationUnknown), nil, nil
	}
}

func (l *ExecutionLifecycle) validateSnapshot(snapshot executionbackend.LifecycleSnapshot) error {
	if snapshot.Ref.Backend != executionbackend.BackendOrca || snapshot.Ref.Protocol != executionbackend.ProtocolOrcaTerminalV2 ||
		snapshot.Ref.Agent.Name != l.setup.Snapshot.Ref.Agent.Name || snapshot.Ref.Target.ID != NativeTargetID(l.orca) ||
		snapshot.ConfigurationDigest == "" || snapshot.ResourceGeneration == "" ||
		snapshot.Session.ProjectID == "" || snapshot.Session.ExecutionID == "" || snapshot.Session.WorkflowRunID == "" ||
		snapshot.Session.AttemptID == "" || snapshot.Session.TaskID == "" {
		return &executionbackend.BackendError{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "native Orca snapshot differs from the fixed execution"}
	}
	if snapshot.Ref.RunID != "" && snapshot.Ref.RunID != snapshot.Session.WorkflowRunID ||
		snapshot.Ref.AttemptID != "" && snapshot.Ref.AttemptID != snapshot.Session.AttemptID {
		return &executionbackend.BackendError{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "native Orca snapshot Run or Attempt changed"}
	}
	if snapshot.Ref.External.Kind != "" && snapshot.Ref.External.Kind != "orca" {
		return &executionbackend.BackendError{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "native Orca process kind is not generic"}
	}
	if snapshot.Workspace.Path != "" && snapshot.Ref.WorkspacePath != "" && snapshot.Workspace.Path != snapshot.Ref.WorkspacePath {
		return &executionbackend.BackendError{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "native Orca workspace differs from its fixed reference"}
	}
	if snapshot.PendingTurn != nil && (snapshot.PendingTurn.CommandID == "" || snapshot.PendingTurn.Prompt == "") {
		return errors.New("native Orca input has no stable command or prompt")
	}
	_, _, err := l.readState(snapshot)
	return err
}

func (l *ExecutionLifecycle) readState(snapshot executionbackend.LifecycleSnapshot) (NativeExecutionState, bool, error) {
	var found *executionbackend.ImplementationDataEnvelope
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

func (l *ExecutionLifecycle) stateEnvelope(snapshot executionbackend.LifecycleSnapshot, target executionbackend.ExecutionTarget,
	process executionbackend.ProcessIdentity, state NativeExecutionState) (executionbackend.ImplementationDataEnvelope, error) {
	return NewExecutionStateEnvelope(ExecutionStateIdentity{ProjectID: snapshot.Session.ProjectID,
		ExecutionID: snapshot.Session.ExecutionID, AttemptID: snapshot.Session.AttemptID,
		Generation: snapshot.ResourceGeneration, ConfigDigest: snapshot.ConfigurationDigest,
		Protocol: snapshot.Ref.Protocol, Process: process, CoordinatorTerminal: target.CoordinatorTerminal}, state)
}

func (l *ExecutionLifecycle) targetEnvelope(snapshot executionbackend.LifecycleSnapshot, target executionbackend.ExecutionTarget,
	process executionbackend.ProcessIdentity, state NativeExecutionState) ([]executionbackend.ImplementationDataEnvelope, error) {
	envelope, err := l.stateEnvelope(snapshot, target, process, state)
	if err != nil {
		return nil, err
	}
	return []executionbackend.ImplementationDataEnvelope{envelope}, nil
}

func nativeReceipt(request executionbackend.LifecycleRequest, disposition executionbackend.OperationStatus) executionbackend.LifecycleReceipt {
	return executionbackend.LifecycleReceipt{Operation: request.Operation, Disposition: disposition}
}

func (l *ExecutionLifecycle) worktreeName(snapshot executionbackend.LifecycleSnapshot) (string, error) {
	return WorkerWorktreeName(snapshot.Session.TaskID, snapshot.Session.WorkflowRunID)
}

func (l *ExecutionLifecycle) repositoryID(ctx context.Context) (string, error) {
	return FindRepo(ctx, l.client.Call, l.setup.RepositoryPath)
}

func (l *ExecutionLifecycle) processGeneration(snapshot executionbackend.LifecycleSnapshot) string {
	if snapshot.Ref.External.Incarnation != "" {
		return snapshot.Ref.External.Incarnation
	}
	return snapshot.ResourceGeneration
}

func (l *ExecutionLifecycle) boundProcess(snapshot executionbackend.LifecycleSnapshot, target executionbackend.ExecutionTarget, turn *NativeTurnState) executionbackend.ProcessIdentity {
	process := snapshot.Ref.External
	process.Kind = "orca"
	process.WorktreeID = target.WorktreeID
	if turn != nil {
		process.Task, process.Dispatch = turn.TaskID, turn.DispatchID
		process.OrchestrationRun = target.OrchestrationRun
	}
	return process
}

var _ executionbackend.LifecyclePort = (*ExecutionLifecycle)(nil)

func (l *ExecutionLifecycle) InspectLifecycleWorkspace(ctx context.Context, snapshot executionbackend.LifecycleSnapshot) (executionbackend.WorkspaceInspection, error) {
	return workspace.InspectLifecycleWorkspace(ctx, l.setup, snapshot)
}
