package orca_remote_worker

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/execution/adapters/orca"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

// ExecutionLifecycle owns supervised resource creation, dispatch, messages,
// settlement, and reconciliation. It receives only immutable setup and one
// center-persisted operation at a time.
type ExecutionLifecycle struct {
	setup         executionbackend.LifecycleSetup
	config        Config
	client        Client
	remotePayload []byte
}

func (l *ExecutionLifecycle) PolicyCapabilities() executionbackend.LifecyclePolicyCapabilities {
	return executionbackend.LifecyclePolicyCapabilities{ResumeActiveObservation: true, ActiveTurnFeedback: true, SeparateTurnStart: true, LinkedWorkspaceWriteRoots: true}
}

func NewExecutionLifecycle(setup executionbackend.LifecycleSetup) (executionbackend.LifecyclePort, error) {
	setup = setup.Clone()
	snapshot := setup.Snapshot
	if snapshot.Ref.Backend != executionbackend.BackendOrca || snapshot.Ref.Protocol != executionbackend.ProtocolOrcaSupervised || strings.TrimSpace(snapshot.Ref.Agent.Name) == "" || setup.ProcessHost == nil {
		return nil, &Error{Code: executionbackend.BackendUnsupported, Diagnostic: "supervised lifecycle setup differs from its fixed implementation target"}
	}
	decode := func(key string) ([]byte, error) {
		var found *executionbackend.ImplementationDataEnvelope
		for i := range setup.ProfileData {
			if setup.ProfileData[i].ImplementationKey != key {
				continue
			}
			if found != nil {
				return nil, fmt.Errorf("duplicate supervised profile envelope %q", key)
			}
			found = &setup.ProfileData[i]
		}
		if found == nil {
			return nil, fmt.Errorf("missing supervised profile envelope %q", key)
		}
		if err := found.ValidateProfileIdentity(snapshot.Session.ProjectID, setup.ProfileID, setup.AcceptedConfigVersion); err != nil {
			return nil, err
		}
		if found.CodecVersion != ProfileCodecVersion || found.Purpose != executionbackend.ImplementationConfigPurpose {
			return nil, fmt.Errorf("unsupported remote profile envelope %q", key)
		}
		return append([]byte(nil), found.Payload...), nil
	}
	remotePayload, err := decode(ProfileCodecKey)
	if err != nil {
		return nil, err
	}
	profile, err := DecodeProfileConfig(remotePayload)
	if err != nil {
		return nil, err
	}
	executable := profile.Executable
	args := append([]string(nil), profile.Args...)
	commandTimeout := setup.CommandTimeout
	if commandTimeout <= 0 {
		commandTimeout = 30 * time.Second
	}
	call := func(ctx context.Context, timeout time.Duration, cliArgs ...string) ([]byte, error) {
		if timeout <= 0 {
			timeout = commandTimeout
		}
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		all := append(append([]string(nil), args...), cliArgs...)
		return setup.ProcessHost.OutputCommand(ctx, executionbackend.LaunchCommand{Executable: executable, Args: all, WorkingDirectory: setup.RepositoryPath, Timeout: timeout})
	}
	gitExecutable := setup.GitExecutable
	if gitExecutable == "" {
		gitExecutable = "git"
	}
	remoteName := setup.RemoteName
	if remoteName == "" {
		remoteName = "origin"
	}
	gitArgs := append([]string(nil), setup.GitArgs...)
	origin := func(ctx context.Context) ([]byte, error) {
		args := append(append([]string(nil), gitArgs...), "remote", "get-url", remoteName)
		return setup.ProcessHost.OutputCommand(ctx, executionbackend.LaunchCommand{Executable: gitExecutable, Args: args, WorkingDirectory: setup.RepositoryPath})
	}
	config := Config{
		Repository: setup.RepositoryPath, ProjectID: snapshot.Session.ProjectID, Agent: snapshot.Ref.Agent.Name,
		Model: profile.Model, ReasoningEffort: profile.ReasoningEffort, CommandTimeout: commandTimeout.String(),
		ExpectedRemoteRuntime: snapshot.Ref.External.RuntimeID,
		Settings: Settings{ConnectionID: profile.ConnectionID, EnvironmentID: profile.EnvironmentID,
			RepositorySelector: profile.RepositorySelector, CoordinatorTerminal: profile.CoordinatorTerminal,
			PollInterval: profile.PollInterval},
		Call: call, Origin: origin, CoordinatorShell: profile.Shell,
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return &ExecutionLifecycle{setup: setup, config: config, client: New(config), remotePayload: remotePayload}, nil
}

func (l *ExecutionLifecycle) PreflightLifecycle(ctx context.Context, setup executionbackend.LifecycleSetup) (executionbackend.CapabilityReport, error) {
	if err := sameLifecycleTarget(l.setup.Snapshot.Ref, setup.Snapshot.Ref); err != nil {
		return executionbackend.CapabilityReport{}, err
	}
	if err := ValidateProfileSelection(setup.Snapshot.Ref.Agent.Name, l.remotePayload); err != nil {
		return executionbackend.CapabilityReport{}, err
	}
	pair, err := Preflight(ctx, l.config)
	if err != nil {
		return executionbackend.CapabilityReport{}, backendError(err)
	}
	report := NewBackend(l.config).report(setup.Snapshot.Ref.Target, setup.Snapshot.Ref.Agent, pair)
	for i := range report.Capabilities {
		if report.Capabilities[i].Name == "start" {
			report.Capabilities[i].Status = executionbackend.CapabilityVerified
			report.Capabilities[i].Evidence = "supervised lifecycle preflight"
		}
	}
	l.config.ExpectedRemoteRuntime = pair.RemoteRuntime
	l.client = New(l.config)
	return report, nil
}

func (l *ExecutionLifecycle) Next(_ context.Context, snapshot executionbackend.LifecycleSnapshot) (executionbackend.LifecycleCandidate, error) {
	if err := l.validateSnapshot(snapshot); err != nil {
		return executionbackend.LifecycleCandidate{}, err
	}
	if snapshot.WorkflowPhase == "completed" && snapshot.TaskDelivered {
		// Delivery is business completion, not a resource-close instruction.
		// Keep the original remote session/worktree as the current product
		// contract requires; an explicitly owned teardown is separate.
		return executionbackend.LifecycleCandidate{Done: true}, nil
	}
	if snapshot.Workspace.Path == "" {
		if snapshot.WorkspacePlan.Path == "" || snapshot.WorkspacePlan.BaseCommit == "" {
			return executionbackend.LifecycleCandidate{}, errors.New("supervised lifecycle has no fixed workspace plan")
		}
		candidate := executionbackend.LifecycleCandidate{Action: executionbackend.LifecyclePrepareWorkspace, Input: executionbackend.LifecycleInput{Workspace: cloneWorkspace(&snapshot.WorkspacePlan)}}
		return candidate, candidate.Validate()
	}
	state, _, err := l.readState(snapshot)
	if err != nil {
		return executionbackend.LifecycleCandidate{}, err
	}
	if snapshot.Ref.Target.CoordinatorTerminal == "" {
		return resourceCandidate("coordinator", "SteerLine coordinator "+snapshot.Session.WorkflowRunID, ""), nil
	}
	if snapshot.Ref.Target.OrchestrationRun == "" {
		return resourceCandidate("workflow", "", "SteerLine worker "+snapshot.Session.ExecutionID), nil
	}
	if snapshot.PendingTurn == nil {
		return executionbackend.LifecycleCandidate{Done: true}, nil
	}
	turn := findTurn(&state, snapshot.PendingTurn.CommandID)
	if turn == nil || turn.TaskID == "" {
		candidate := resourceCandidate("conversation", snapshot.Session.TaskID, snapshot.PendingTurn.Prompt)
		candidate.Input.Resource.TurnID = snapshot.PendingTurn.CommandID
		return candidate, candidate.Validate()
	}
	candidate := executionbackend.LifecycleCandidate{Action: executionbackend.LifecycleStartTurn, Input: executionbackend.LifecycleInput{Turn: cloneLifecycleTurn(snapshot.PendingTurn)}}
	return candidate, candidate.Validate()
}

func resourceCandidate(purpose, name, objective string) executionbackend.LifecycleCandidate {
	candidate := executionbackend.LifecycleCandidate{Action: executionbackend.LifecycleCreateResource, Input: executionbackend.LifecycleInput{Resource: &executionbackend.ResourceIntent{Purpose: purpose, Name: name, Objective: objective}}}
	return candidate
}

func (l *ExecutionLifecycle) Execute(ctx context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, executionbackend.LifecycleObserver, error) {
	if err := request.Validate(); err != nil {
		return executionbackend.LifecycleReceipt{Operation: request.Operation, Disposition: executionbackend.OperationRejected, Diagnostic: err.Error()}, nil, err
	}
	if err := l.validateSnapshot(request.Snapshot); err != nil {
		return executionbackend.LifecycleReceipt{Operation: request.Operation, Disposition: executionbackend.OperationRejected, Diagnostic: err.Error()}, nil, err
	}
	snapshot := request.Snapshot.Clone()
	switch executionbackend.LifecycleAction(request.Operation.Action) {
	case executionbackend.LifecyclePrepareWorkspace:
		receipt, err := l.prepareWorkspace(ctx, request)
		return receipt, nil, err
	case executionbackend.LifecycleCreateResource:
		receipt, err := l.createResource(ctx, request)
		return receipt, nil, err
	case executionbackend.LifecycleStartTurn:
		receipt, observer, err := l.startTurn(ctx, request)
		return receipt, observer, err
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
		_ = snapshot
		return executionbackend.LifecycleReceipt{Operation: request.Operation, Disposition: executionbackend.OperationRejected}, nil,
			executionbackend.UnsupportedAdapterOperation(request.Operation.Action, "supervised lifecycle action is unsupported")
	}
}

func (l *ExecutionLifecycle) Reconcile(ctx context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, executionbackend.LifecycleObserver, error) {
	if err := request.Validate(); err != nil {
		return executionbackend.LifecycleReceipt{Operation: request.Operation, Disposition: executionbackend.OperationRejected}, nil, err
	}
	snapshot := request.Snapshot.Clone()
	if err := l.validateSnapshot(snapshot); err != nil {
		return executionbackend.LifecycleReceipt{Operation: request.Operation, Disposition: executionbackend.OperationRejected}, nil, err
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
	default:
		return executionbackend.LifecycleReceipt{Operation: request.Operation, Disposition: executionbackend.OperationUnknown, Diagnostic: "original supervised operation has no read-only reconciliation"}, nil, nil
	}
}

func (l *ExecutionLifecycle) ObserveLifecycle(ctx context.Context, snapshot executionbackend.LifecycleSnapshot) (executionbackend.LifecycleReceipt, executionbackend.LifecycleObserver, error) {
	if err := l.validateSnapshot(snapshot); err != nil {
		return executionbackend.LifecycleReceipt{}, nil, err
	}
	state, _, err := l.readState(snapshot)
	if err != nil {
		return executionbackend.LifecycleReceipt{}, nil, err
	}
	turn := currentTurn(&state, snapshot)
	if turn == nil || turn.DispatchID == "" || snapshot.Ref.External.Dispatch == "" {
		return executionbackend.LifecycleReceipt{Disposition: executionbackend.OperationUnknown, Diagnostic: "supervised observation has no original Dispatch"}, nil, nil
	}
	ref := RemoteRef(snapshot.Ref)
	observation, showErr := l.client.Show(ctx, StartReceipt{RunID: snapshot.Ref.External.OrchestrationRun, TaskID: snapshot.Ref.External.Task,
		DispatchID: snapshot.Ref.External.Dispatch, WorktreeID: snapshot.Ref.External.WorktreeID, CenterRuntimeID: snapshot.Ref.External.CoordinatorRuntimeID})
	receipt := executionbackend.LifecycleReceipt{Disposition: executionbackend.OperationUnknown,
		State: executionbackend.LifecycleStateFacts{SessionID: SessionID(snapshot.Ref.External), SessionGeneration: snapshot.Ref.External.Incarnation,
			TurnID: turn.TurnID}}
	if showErr != nil {
		receipt.Diagnostic = showErr.Error()
		return receipt, nil, showErr
	}
	outcome, settleErr := l.client.Settlement(ctx, ref, turn.Outcome)
	if settleErr == nil && turn.DoneMessageID != "" {
		turn.Settled, turn.Outcome = true, outcome
		receipt.Disposition = executionbackend.OperationConfirmed
		receipt.State.TurnCompleted = true
		receipt.TurnEvidence = &executionbackend.TurnEvidence{TurnID: turn.TurnID, CommandID: turn.TurnID, SessionID: SessionID(snapshot.Ref.External), Started: true, Completed: true, Succeeded: outcome == "succeeded", LastMessage: turn.LastMessage, PRCandidates: append([]string(nil), turn.PRCandidates...)}
		receipt.Implementation, err = l.stateEnvelopes(snapshot, state)
		if err != nil {
			return receipt, nil, err
		}
		return receipt, nil, nil
	}
	if observation.Liveness != "live" && settleErr != nil {
		receipt.Diagnostic = "original supervised Dispatch is not live or settled"
		return receipt, nil, nil
	}
	receipt.Disposition = executionbackend.OperationAccepted
	receipt.Handle = &remoteStopHandle{identity: snapshot.Ref.External}
	return receipt, newLifecycleObserver(ctx, l, snapshot), nil
}

func (l *ExecutionLifecycle) StopLifecycle(_ context.Context, request executionbackend.LifecycleRequest, _ executionbackend.StopHandle) (executionbackend.LifecycleReceipt, error) {
	return executionbackend.LifecycleReceipt{Operation: request.Operation, Disposition: executionbackend.OperationRejected},
		executionbackend.UnsupportedAdapterOperation(request.Operation.Action, "supervised remote cancellation is unsupported")
}

func (l *ExecutionLifecycle) CleanupLifecycle(ctx context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, error) {
	return l.cleanup(ctx, request)
}

func (l *ExecutionLifecycle) VerifyManualRecovery(ctx context.Context, snapshot executionbackend.LifecycleSnapshot) (executionbackend.LifecycleRecoveryEvidence, error) {
	if err := l.validateSnapshot(snapshot); err != nil {
		return executionbackend.LifecycleRecoveryEvidence{}, err
	}
	if snapshot.Ref.External.Dispatch == "" {
		return executionbackend.LifecycleRecoveryEvidence{SessionAvailable: snapshot.Session.SessionID != ""}, nil
	}
	ref := RemoteRef(snapshot.Ref)
	if err := l.client.VerifyWorker(ctx, ref); err != nil {
		return executionbackend.LifecycleRecoveryEvidence{}, backendError(err)
	}
	obs, err := l.client.Show(ctx, StartReceipt{RunID: snapshot.Ref.External.OrchestrationRun, TaskID: snapshot.Ref.External.Task,
		DispatchID: snapshot.Ref.External.Dispatch, WorktreeID: snapshot.Ref.External.WorktreeID,
		CenterRuntimeID: snapshot.Ref.External.CoordinatorRuntimeID})
	if err != nil {
		return executionbackend.LifecycleRecoveryEvidence{}, backendError(err)
	}
	return executionbackend.LifecycleRecoveryEvidence{SessionAvailable: true, ManagedProcess: obs.Liveness == "live", ProcessAbsent: obs.Liveness != "live"}, nil
}

func (l *ExecutionLifecycle) BindRecovery(ctx context.Context, previous, next executionbackend.LifecycleSnapshot) (executionbackend.ImplementationDataEnvelope, error) {
	if err := l.validateSnapshot(previous); err != nil {
		return executionbackend.ImplementationDataEnvelope{}, err
	}
	if err := l.validateSnapshot(next); err != nil {
		return executionbackend.ImplementationDataEnvelope{}, err
	}
	if previous.Ref.External != next.Ref.External || previous.Ref.Target != next.Ref.Target || previous.Session.ExecutionID != next.Session.ExecutionID || previous.Session.AttemptID == next.Session.AttemptID {
		return executionbackend.ImplementationDataEnvelope{}, &Error{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "manual recovery changed the fixed remote worker incarnation"}
	}
	if err := ctx.Err(); err != nil {
		return executionbackend.ImplementationDataEnvelope{}, err
	}
	state, found, err := l.readState(previous)
	if err != nil || !found {
		return executionbackend.ImplementationDataEnvelope{}, errors.Join(err, errors.New("original supervised state is unavailable"))
	}
	return l.stateEnvelope(next, state)
}

func (l *ExecutionLifecycle) prepareWorkspace(ctx context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, error) {
	receipt := lifecycleReceipt(request)
	intent := request.Input.Workspace
	if intent == nil {
		return receipt, errors.New("supervised workspace intent is missing")
	}
	pair, err := l.preflight(ctx, request.Snapshot)
	if err != nil {
		return receipt, backendError(err)
	}
	name := worktreeName(request.Snapshot.Session.TaskID, request.Snapshot.Session.WorkflowRunID)
	if name == "" {
		return receipt, errors.New("supervised workspace has no stable task/run name")
	}
	worktree, err := l.client.CreateWorktree(ctx, pair, name, intent.BaseCommit)
	if err != nil {
		receipt.Disposition, receipt.Diagnostic = executionbackend.OperationUnknown, err.Error()
		return receipt, backendError(err)
	}
	inspection, err := orcaworker.WorkspaceBinding(worktree, request.Snapshot.Session.ProjectID, intent.BaseCommit)
	if err != nil {
		return receipt, backendError(err)
	}
	receipt.Disposition = executionbackend.OperationConfirmed
	receipt.Workspace = &inspection
	receipt.State.WorktreeID = worktree.ID
	target := targetWithWorkspace(request.Snapshot.Ref.Target, worktree.ID, inspection.Branch, inspection.Base)
	receipt.Target = &target
	state, _, stateErr := l.readState(request.Snapshot)
	if stateErr != nil {
		return receipt, stateErr
	}
	state.RemoteRuntimeID, state.RepositoryID = pair.RemoteRuntime, pair.RepositoryID
	receipt.Implementation, err = l.envelopeWithTarget(request.Snapshot, state, *receipt.Target)
	return receipt, err
}

func (l *ExecutionLifecycle) reconcileWorkspace(ctx context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, executionbackend.LifecycleObserver, error) {
	receipt := lifecycleReceipt(request)
	intent := request.Input.Workspace
	if intent == nil {
		return receipt, nil, errors.New("supervised workspace reconciliation lost its original intent")
	}
	pair, err := l.preflight(ctx, request.Snapshot)
	if err != nil {
		return receipt, nil, backendError(err)
	}
	name := worktreeName(request.Snapshot.Session.TaskID, request.Snapshot.Session.WorkflowRunID)
	worktree, found, err := l.client.FindWorktree(ctx, pair, name, intent.BaseCommit)
	if err != nil || !found {
		receipt.Disposition = executionbackend.OperationUnknown
		if err != nil {
			receipt.Diagnostic = err.Error()
		}
		return receipt, nil, backendError(err)
	}
	inspection, err := orcaworker.WorkspaceBinding(worktree, request.Snapshot.Session.ProjectID, intent.BaseCommit)
	if err != nil {
		return receipt, nil, backendError(err)
	}
	state, _, err := l.readState(request.Snapshot)
	if err != nil {
		return receipt, nil, err
	}
	state.RemoteRuntimeID, state.RepositoryID = pair.RemoteRuntime, pair.RepositoryID
	receipt.Disposition, receipt.Workspace = executionbackend.OperationConfirmed, &inspection
	receipt.State.WorktreeID = worktree.ID
	target := targetWithWorkspace(request.Snapshot.Ref.Target, worktree.ID, inspection.Branch, inspection.Base)
	receipt.Target = &target
	receipt.Implementation, err = l.envelopeWithTarget(request.Snapshot, state, *receipt.Target)
	return receipt, nil, err
}

func (l *ExecutionLifecycle) createResource(ctx context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, error) {
	resource := request.Input.Resource
	receipt := lifecycleReceipt(request)
	if resource == nil {
		return receipt, errors.New("supervised resource intent is missing")
	}
	pair, err := l.preflight(ctx, request.Snapshot)
	if err != nil {
		return receipt, backendError(err)
	}
	state, _, err := l.readState(request.Snapshot)
	if err != nil {
		return receipt, err
	}
	target := request.Snapshot.Ref.Target
	switch resource.Purpose {
	case "coordinator":
		handle, callErr := l.client.CreateCoordinator(ctx, request.Snapshot.Session.WorkflowRunID, pair.CenterRuntime)
		if callErr != nil {
			receipt.Disposition, receipt.Diagnostic = executionbackend.OperationUnknown, callErr.Error()
			return receipt, backendError(callErr)
		}
		identity, showErr := l.client.ShowCoordinator(ctx, handle, pair.CenterRuntime)
		if showErr != nil {
			receipt.Disposition, receipt.Diagnostic = executionbackend.OperationUnknown, showErr.Error()
			return receipt, backendError(showErr)
		}
		state.Coordinator, state.RemoteRuntimeID = identity, pair.RemoteRuntime
		target.CoordinatorTerminal = handle
		receipt.State.ResourceID, receipt.State.RuntimeID = handle, pair.CenterRuntime
		receipt.RequestID = ""
	case "workflow":
		if state.Coordinator.Terminal == "" {
			return receipt, errors.New("supervised Run creation has no committed coordinator")
		}
		created, callErr := orcaworker.CreateRun(ctx, l.client.CenterCall, resource.Objective, state.Coordinator.Terminal, state.Coordinator.RuntimeID)
		if callErr != nil {
			receipt.Disposition, receipt.Diagnostic = executionbackend.OperationUnknown, callErr.Error()
			return receipt, backendError(callErr)
		}
		target.OrchestrationRun, target.OrchestrationRunRequestID = created.ID, created.RequestID
		receipt.State.ResourceID, receipt.RequestID = created.ID, created.RequestID
	case "conversation":
		if target.OrchestrationRun == "" || state.Coordinator.Terminal == "" || resource.TurnID == "" || resource.Objective == "" {
			return receipt, errors.New("supervised conversation creation has no committed Run, coordinator, or input")
		}
		created, callErr := orcaworker.CreateTask(ctx, l.client.CenterCall, resource.Objective, request.Snapshot.TaskTitle, target.OrchestrationRun, state.Coordinator.Terminal, state.Coordinator.RuntimeID)
		if callErr != nil {
			receipt.Disposition, receipt.Diagnostic = executionbackend.OperationUnknown, callErr.Error()
			return receipt, backendError(callErr)
		}
		state.Turns = append(state.Turns, orcaworker.NativeTurnState{TurnID: resource.TurnID, TaskID: created.ID,
			TaskRequestID: created.RequestID, Purpose: "business", IntentAt: time.Now().UTC()})
		receipt.State.ResourceID, receipt.RequestID = created.ID, created.RequestID
	default:
		return receipt, executionbackend.UnsupportedAdapterOperation(string(request.Operation.Action), "supervised resource purpose is unsupported")
	}
	receipt.Target = &target
	receipt.Implementation, err = l.envelopeWithTarget(request.Snapshot, state, target)
	if err != nil {
		return receipt, err
	}
	receipt.Disposition = executionbackend.OperationConfirmed
	return receipt, nil
}

func (l *ExecutionLifecycle) reconcileResource(ctx context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, executionbackend.LifecycleObserver, error) {
	resource := request.Input.Resource
	receipt := lifecycleReceipt(request)
	if resource == nil {
		return receipt, nil, errors.New("supervised resource reconciliation lost its original intent")
	}
	pair, err := l.preflight(ctx, request.Snapshot)
	if err != nil {
		return receipt, nil, backendError(err)
	}
	state, _, err := l.readState(request.Snapshot)
	if err != nil {
		return receipt, nil, err
	}
	target := request.Snapshot.Ref.Target
	switch resource.Purpose {
	case "coordinator":
		handle, findErr := l.client.FindCoordinator(ctx, resource.Name, pair.CenterRuntime)
		if findErr != nil || handle == "" {
			receipt.Disposition = executionbackend.OperationUnknown
			receipt.Diagnostic = "original coordinator creation has no unique matching terminal"
			return receipt, nil, backendError(findErr)
		}
		identity, showErr := l.client.ShowCoordinator(ctx, handle, pair.CenterRuntime)
		if showErr != nil {
			return receipt, nil, backendError(showErr)
		}
		state.Coordinator, state.RemoteRuntimeID = identity, pair.RemoteRuntime
		target.CoordinatorTerminal = handle
		receipt.State.ResourceID, receipt.State.RuntimeID = handle, pair.CenterRuntime
	case "workflow":
		id, findErr := l.client.FindRun(ctx, resource.Objective, state.Coordinator.RuntimeID, state.Coordinator.Terminal)
		if findErr != nil || id == "" {
			receipt.Disposition, receipt.Diagnostic = executionbackend.OperationUnknown, "original Run creation has no unique matching Run"
			return receipt, nil, backendError(findErr)
		}
		target.OrchestrationRun = id
		receipt.State.ResourceID = id
	case "conversation":
		id, findErr := l.client.FindTaskFrom(ctx, target.OrchestrationRun, state.Coordinator.Terminal, resource.Objective, state.Coordinator.RuntimeID)
		if findErr != nil || id == "" {
			receipt.Disposition, receipt.Diagnostic = executionbackend.OperationUnknown, "original conversation creation has no unique matching Task"
			return receipt, nil, backendError(findErr)
		}
		if resource.TurnID == "" {
			return receipt, nil, errors.New("original conversation recovery lost its saved turn identity")
		}
		state.Turns = append(state.Turns, orcaworker.NativeTurnState{TurnID: resource.TurnID, TaskID: id,
			Purpose: "business", IntentAt: request.Operation.StartedAt})
		receipt.State.ResourceID = id
	default:
		return receipt, nil, executionbackend.UnsupportedAdapterOperation(string(request.Operation.Action), "supervised resource reconciliation is unsupported")
	}
	receipt.Target = &target
	receipt.Implementation, err = l.envelopeWithTarget(request.Snapshot, state, target)
	receipt.Disposition = executionbackend.OperationConfirmed
	return receipt, nil, err
}

func (l *ExecutionLifecycle) startTurn(ctx context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, executionbackend.LifecycleObserver, error) {
	receipt := lifecycleReceipt(request)
	input := request.Input.Turn
	if input == nil {
		return receipt, nil, errors.New("supervised dispatch has no original input")
	}
	state, _, err := l.readState(request.Snapshot)
	if err != nil {
		return receipt, nil, err
	}
	turn := findTurn(&state, input.CommandID)
	if turn == nil || turn.TaskID == "" || request.Snapshot.Ref.Target.WorktreeID == "" {
		return receipt, nil, errors.New("supervised dispatch lacks its committed conversation or workspace")
	}
	var previous *executionbackend.ProcessIdentity
	if request.Snapshot.Ref.External.Kind == "orca_supervised" {
		identity := request.Snapshot.Ref.External
		previous = &identity
	}
	// The platform Run and the SteerLine WorkflowRun are different namespaces.
	config := l.config
	config.Settings.CoordinatorTerminal = state.Coordinator.Terminal
	client := New(config)
	started, err := client.StartTask(ctx, request.Snapshot.Ref.Target.OrchestrationRun, turn.TaskID, request.Snapshot.Ref.Target.WorktreeID, previous)
	if err != nil {
		receipt.Disposition, receipt.Diagnostic = executionbackend.OperationUnknown, err.Error()
		return receipt, nil, backendError(err)
	}
	identity, err := l.client.BindStart(ctx, started, previous, request.Snapshot.Ref.WorkspacePath, request.Snapshot.Ref.WorkspaceBranch)
	if err != nil {
		receipt.Disposition, receipt.Diagnostic = executionbackend.OperationUnknown, err.Error()
		return receipt, nil, backendError(err)
	}
	turn.DispatchID, turn.DispatchRequestID = started.DispatchID, started.PlatformRequestID
	target := request.Snapshot.Ref.Target
	target.ExternalTask, target.Dispatch = started.TaskID, started.DispatchID
	target.OrchestrationRun, target.OrchestrationRunRequestID = started.RunID, started.PlatformRequestID
	target.ExecutionHost, target.WorktreeID = identity.ExecutionHost, identity.WorktreeID
	target.CoordinatorTerminal = request.Snapshot.Ref.Target.CoordinatorTerminal
	receipt.Target = &target
	receipt.Process = &identity
	receipt.RequestID = started.PlatformRequestID
	receipt.Disposition = executionbackend.OperationAccepted
	receipt.State.SessionID, receipt.State.SessionGeneration = SessionID(identity), identity.Incarnation
	receipt.State.TurnID = input.CommandID
	receipt.State.InputAccepted, receipt.State.TurnStarted = true, false
	receipt.Implementation, err = l.envelopeWithTarget(request.Snapshot, state, target)
	if err != nil {
		return receipt, nil, err
	}
	bound := lifecycleSnapshotWithBinding(request.Snapshot, target, identity, state)
	bound.Implementation = cloneEnvelopes(receipt.Implementation)
	return receipt, newLifecycleObserver(ctx, l, bound), nil
}

func (l *ExecutionLifecycle) reconcileStart(ctx context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, executionbackend.LifecycleObserver, error) {
	receipt := lifecycleReceipt(request)
	input := request.Input.Turn
	if input == nil {
		return receipt, nil, errors.New("supervised Dispatch reconciliation lost the original input")
	}
	state, _, err := l.readState(request.Snapshot)
	if err != nil {
		return receipt, nil, err
	}
	turn := findTurn(&state, input.CommandID)
	if turn == nil || turn.TaskID == "" || request.Snapshot.Ref.Target.WorktreeID == "" {
		return receipt, nil, errors.New("original supervised Task/Workspace is unavailable")
	}
	started, err := l.client.RecoverStart(ctx, request.Snapshot.Ref.Target.OrchestrationRun, turn.TaskID, request.Snapshot.Ref.Target.WorktreeID, state.Coordinator.RuntimeID)
	if err != nil {
		return receipt, nil, backendError(err)
	}
	if request.Snapshot.Ref.External.Kind == "orca_supervised" {
		previous := request.Snapshot.Ref.External
		if previous.Dispatch != started.DispatchID || previous.Task != turn.TaskID || previous.OrchestrationRun != started.RunID {
			return receipt, nil, &Error{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "reconciled Dispatch differs from the original session"}
		}
	}
	identity, err := l.client.BindStart(ctx, started, processPointer(request.Snapshot.Ref.External), request.Snapshot.Ref.WorkspacePath, request.Snapshot.Ref.WorkspaceBranch)
	if err != nil {
		return receipt, nil, backendError(err)
	}
	turn.DispatchID, turn.DispatchRequestID = started.DispatchID, started.PlatformRequestID
	target := request.Snapshot.Ref.Target
	target.ExternalTask, target.Dispatch, target.OrchestrationRun = started.TaskID, started.DispatchID, started.RunID
	target.OrchestrationRunRequestID, target.ExecutionHost, target.WorktreeID = started.PlatformRequestID, identity.ExecutionHost, identity.WorktreeID
	receipt.Target, receipt.Process, receipt.RequestID = &target, &identity, started.PlatformRequestID
	receipt.Disposition = executionbackend.OperationAccepted
	receipt.State.SessionID, receipt.State.SessionGeneration = SessionID(identity), identity.Incarnation
	receipt.State.TurnID, receipt.State.InputAccepted, receipt.State.TurnStarted = input.CommandID, true, false
	receipt.Implementation, err = l.envelopeWithTarget(request.Snapshot, state, target)
	if err != nil {
		return receipt, nil, err
	}
	bound := lifecycleSnapshotWithBinding(request.Snapshot, target, identity, state)
	bound.Implementation = cloneEnvelopes(receipt.Implementation)
	return receipt, newLifecycleObserver(ctx, l, bound), nil
}

func (l *ExecutionLifecycle) deliverFeedback(ctx context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, error) {
	receipt := lifecycleReceipt(request)
	input := request.Input.Feedback
	if input == nil || input.Answer == nil || request.Snapshot.QuestionRoute == nil {
		return receipt, errors.New("supervised answer lacks its original question route")
	}
	state, _, err := l.readState(request.Snapshot)
	if err != nil {
		return receipt, err
	}
	turn, route, err := l.questionRoute(request.Snapshot, state)
	if err != nil {
		return receipt, err
	}
	body := strings.Join(input.Answer.Answers[route.MessageID], "\n\n")
	if body == "" {
		body = input.Message
	}
	if body == "" {
		return receipt, &Error{Code: executionbackend.BackendRejected, Diagnostic: "supervised structured answer contains no response text"}
	}
	requestID, err := l.client.Reply(ctx, RemoteRef(request.Snapshot.Ref), route.MessageID, body)
	receipt.RequestID = requestID
	if err != nil {
		receipt.Disposition, receipt.Diagnostic = executionbackend.OperationUnknown, err.Error()
		return receipt, backendError(err)
	}
	receipt.Disposition, receipt.State.InputAccepted = executionbackend.OperationConfirmed, true
	receipt.State.SessionID, receipt.State.SessionGeneration, receipt.State.TurnID = SessionID(request.Snapshot.Ref.External), request.Snapshot.Ref.External.Incarnation, turn.TurnID
	return receipt, nil
}

func (l *ExecutionLifecycle) reconcileFeedback(ctx context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, executionbackend.LifecycleObserver, error) {
	receipt := lifecycleReceipt(request)
	input := request.Input.Feedback
	if input == nil || input.Answer == nil || request.Snapshot.QuestionRoute == nil {
		return receipt, nil, errors.New("supervised answer reconciliation lost its original question route")
	}
	state, _, err := l.readState(request.Snapshot)
	if err != nil {
		return receipt, nil, err
	}
	turn, route, err := l.questionRoute(request.Snapshot, state)
	if err != nil {
		return receipt, nil, err
	}
	body := strings.Join(input.Answer.Answers[route.MessageID], "\n\n")
	if body == "" {
		body = input.Message
	}
	if body == "" {
		return receipt, nil, &Error{Code: executionbackend.BackendRejected, Diagnostic: "supervised structured answer contains no response text"}
	}
	requestID, err := l.client.FindAnswer(ctx, RemoteRef(request.Snapshot.Ref), route.MessageID, body)
	if err != nil || requestID == "" {
		receipt.Disposition, receipt.Diagnostic = executionbackend.OperationUnknown, "original answer has no unique confirmed remote receipt"
		return receipt, nil, backendError(err)
	}
	receipt.RequestID, receipt.Disposition = requestID, executionbackend.OperationConfirmed
	receipt.State.InputAccepted, receipt.State.TurnID = true, turn.TurnID
	receipt.State.SessionID, receipt.State.SessionGeneration = SessionID(request.Snapshot.Ref.External), request.Snapshot.Ref.External.Incarnation
	return receipt, nil, nil
}

func (l *ExecutionLifecycle) acknowledge(ctx context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, error) {
	receipt := lifecycleReceipt(request)
	input := request.Input.Ack
	if input == nil || input.DeliveryID == "" || request.Snapshot.PendingDeliveryID != input.DeliveryID {
		return receipt, errors.New("supervised acknowledgement differs from the committed Delivery")
	}
	requestID, err := l.client.Ack(ctx, RemoteRef(request.Snapshot.Ref), input.DeliveryID)
	receipt.RequestID, receipt.DeliveryID = requestID, input.DeliveryID
	if err != nil {
		receipt.Disposition, receipt.Diagnostic = executionbackend.OperationUnknown, err.Error()
		return receipt, backendError(err)
	}
	receipt.Disposition = executionbackend.OperationConfirmed
	return receipt, nil
}

func (l *ExecutionLifecycle) reconcileAck(ctx context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, executionbackend.LifecycleObserver, error) {
	receipt := lifecycleReceipt(request)
	input := request.Input.Ack
	if input == nil || input.DeliveryID == "" {
		return receipt, nil, errors.New("supervised acknowledgement reconciliation lost its Delivery ID")
	}
	confirmed, err := l.client.DeliveryAcknowledged(ctx, RemoteRef(request.Snapshot.Ref), input.DeliveryID)
	if err != nil || !confirmed {
		receipt.Disposition, receipt.Diagnostic = executionbackend.OperationUnknown, "original Delivery acknowledgement is unconfirmed"
		return receipt, nil, backendError(err)
	}
	receipt.Disposition, receipt.DeliveryID = executionbackend.OperationConfirmed, input.DeliveryID
	return receipt, nil, nil
}

func (l *ExecutionLifecycle) reconcileCleanup(ctx context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, executionbackend.LifecycleObserver, error) {
	receipt := lifecycleReceipt(request)
	input := request.Input.Cleanup
	if input == nil || len(input.Resources) != 1 {
		return receipt, nil, errors.New("supervised cleanup reconciliation needs one exact resource")
	}
	state, found, err := l.readState(request.Snapshot)
	if err != nil || !found {
		return receipt, nil, errors.Join(err, errors.New("supervised cleanup state is unavailable"))
	}
	resource := input.Resources[0]
	switch resource.Kind {
	case "orca_supervised":
		if resource != request.Snapshot.Ref.External {
			return receipt, nil, &Error{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "remote release reconciliation changed the original Dispatch"}
		}
		released, checkErr := l.client.ReleaseAcknowledged(ctx, RemoteRef(request.Snapshot.Ref))
		if checkErr != nil || !released {
			receipt.Disposition, receipt.Diagnostic = executionbackend.OperationUnknown, "original worker release remains unconfirmed"
			return receipt, nil, backendError(checkErr)
		}
		turn := findDispatchTurn(&state, resource.Dispatch)
		if turn == nil {
			return receipt, nil, errors.New("released Dispatch has no fixed turn state")
		}
		turn.Released = true
	case "orca-remote-coordinator":
		if resource != state.Coordinator {
			return receipt, nil, &Error{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "remote coordinator reconciliation changed its original identity"}
		}
		if _, checkErr := l.client.CloseCoordinator(ctx, RemoteRef(request.Snapshot.Ref), resource, true); checkErr != nil {
			receipt.Disposition, receipt.Diagnostic = executionbackend.OperationUnknown, checkErr.Error()
			return receipt, nil, backendError(checkErr)
		}
		state.CoordinatorClosed = true
	default:
		return receipt, nil, executionbackend.UnsupportedAdapterOperation(request.Operation.Action, "supervised cleanup resource is unsupported")
	}
	receipt.Disposition = executionbackend.OperationConfirmed
	receipt.Implementation, err = l.stateEnvelopes(request.Snapshot, state)
	return receipt, nil, err
}

func (l *ExecutionLifecycle) questionRoute(snapshot executionbackend.LifecycleSnapshot, state orcaworker.NativeExecutionState) (*orcaworker.NativeTurnState, orcaworker.QuestionRouteData, error) {
	if snapshot.QuestionRoute == nil {
		return nil, orcaworker.QuestionRouteData{}, errors.New("supervised question route is missing")
	}
	for i := range state.Turns {
		turn := &state.Turns[i]
		fixed := snapshot.Session
		fixed.TaskID, fixed.CurrentTurnID = turn.TaskID, turn.TurnID
		data, err := orcaworker.DecodeQuestionRoute(*snapshot.QuestionRoute, fixed, turn.TaskID, turn.DispatchID, snapshot.Ref.External.Terminal)
		if err == nil {
			return turn, data, nil
		}
	}
	return nil, orcaworker.QuestionRouteData{}, errors.New("supervised question route does not match an original Task/Dispatch")
}

func (l *ExecutionLifecycle) cleanup(ctx context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, error) {
	receipt := lifecycleReceipt(request)
	input := request.Input.Cleanup
	if input == nil || len(input.Resources) != 1 {
		return receipt, errors.New("supervised cleanup requires one exact settled resource")
	}
	state, _, err := l.readState(request.Snapshot)
	if err != nil {
		return receipt, err
	}
	ref := RemoteRef(request.Snapshot.Ref)
	resource := input.Resources[0]
	if resource.Kind == "orca_supervised" {
		turn := findDispatchTurn(&state, resource.Dispatch)
		if turn == nil || !turn.Settled || turn.Released || resource != request.Snapshot.Ref.External {
			return receipt, &Error{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "remote release differs from the exact settled original Dispatch"}
		}
		result, callErr := l.client.Release(ctx, ref)
		receipt.RequestID = result.RequestID
		if callErr != nil {
			receipt.Disposition, receipt.Diagnostic = executionbackend.OperationUnknown, callErr.Error()
			return receipt, backendError(callErr)
		}
		turn.Released = true
		receipt.Implementation, err = l.stateEnvelopes(request.Snapshot, state)
		if err != nil {
			return receipt, err
		}
		receipt.Disposition = executionbackend.OperationConfirmed
		return receipt, nil
	}
	if resource.Kind == "orca-remote-coordinator" {
		if state.CoordinatorClosed || resource != state.Coordinator {
			return receipt, &Error{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "remote coordinator cleanup changed its original identity"}
		}
		_, callErr := l.client.CloseCoordinator(ctx, ref, state.Coordinator, false)
		if callErr != nil {
			receipt.Disposition, receipt.Diagnostic = executionbackend.OperationUnknown, callErr.Error()
			return receipt, backendError(callErr)
		}
		state.CoordinatorClosed = true
		receipt.Implementation, err = l.stateEnvelopes(request.Snapshot, state)
		if err != nil {
			return receipt, err
		}
		receipt.Disposition, receipt.Facts = executionbackend.OperationConfirmed, nil
		return receipt, nil
	}
	return receipt, executionbackend.UnsupportedAdapterOperation(request.Operation.Action, "supervised resource cannot be cleaned by this implementation")
}

func (l *ExecutionLifecycle) preflight(ctx context.Context, snapshot executionbackend.LifecycleSnapshot) (PreflightResult, error) {
	state, found, err := l.readState(snapshot)
	if err != nil {
		return PreflightResult{}, err
	}
	config := l.config
	if found && state.RemoteRuntimeID != "" {
		if config.ExpectedRemoteRuntime != "" && config.ExpectedRemoteRuntime != state.RemoteRuntimeID {
			return PreflightResult{}, &Error{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "supervised lifecycle changed its original remote runtime"}
		}
		config.ExpectedRemoteRuntime = state.RemoteRuntimeID
	}
	pair, err := Preflight(ctx, config)
	if err == nil {
		config.ExpectedRemoteRuntime = pair.RemoteRuntime
		l.config, l.client = config, New(config)
	}
	return pair, err
}

func (l *ExecutionLifecycle) validateSnapshot(snapshot executionbackend.LifecycleSnapshot) error {
	if err := sameLifecycleTarget(l.setup.Snapshot.Ref, snapshot.Ref); err != nil {
		return err
	}
	if snapshot.Session.ProjectID == "" || snapshot.Session.WorkflowRunID == "" || snapshot.Session.ExecutionID == "" || snapshot.Session.AttemptID == "" || snapshot.ConfigurationDigest == "" || snapshot.ResourceGeneration == "" {
		return errors.New("supervised lifecycle snapshot has incomplete execution identity")
	}
	return nil
}

func sameLifecycleTarget(want, got executionbackend.ExecutionRef) error {
	if want.Backend != got.Backend || want.Protocol != got.Protocol || want.Agent != got.Agent || !SameBaseTarget(want.Target, got.Target) || want.RunID != got.RunID || want.AttemptID != got.AttemptID || want.BaseCommit != got.BaseCommit {
		return &Error{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "supervised lifecycle target or execution identity changed"}
	}
	return nil
}

func (l *ExecutionLifecycle) readState(snapshot executionbackend.LifecycleSnapshot) (orcaworker.NativeExecutionState, bool, error) {
	var found *executionbackend.ImplementationDataEnvelope
	for i := range snapshot.Implementation {
		if snapshot.Implementation[i].ImplementationKey != orcaworker.ExecutionStateCodecKey {
			continue
		}
		if found != nil {
			return orcaworker.NativeExecutionState{}, false, errors.New("execution contains duplicate supervised state envelopes")
		}
		found = &snapshot.Implementation[i]
	}
	if found == nil {
		return orcaworker.NativeExecutionState{}, false, nil
	}
	state, err := orcaworker.DecodeExecutionStateEnvelope(*found, stateIdentity(snapshot))
	return state, err == nil, err
}

func stateIdentity(snapshot executionbackend.LifecycleSnapshot) orcaworker.ExecutionStateIdentity {
	return orcaworker.ExecutionStateIdentity{ProjectID: snapshot.Session.ProjectID, ExecutionID: snapshot.Session.ExecutionID,
		AttemptID: snapshot.Session.AttemptID, Generation: snapshot.ResourceGeneration, ConfigDigest: snapshot.ConfigurationDigest,
		Protocol: snapshot.Ref.Protocol, Process: snapshot.Ref.External, CoordinatorTerminal: snapshot.Ref.Target.CoordinatorTerminal}
}

func (l *ExecutionLifecycle) stateEnvelope(snapshot executionbackend.LifecycleSnapshot, state orcaworker.NativeExecutionState) (executionbackend.ImplementationDataEnvelope, error) {
	return orcaworker.NewExecutionStateEnvelope(stateIdentity(snapshot), state)
}

func (l *ExecutionLifecycle) stateEnvelopes(snapshot executionbackend.LifecycleSnapshot, state orcaworker.NativeExecutionState) ([]executionbackend.ImplementationDataEnvelope, error) {
	envelope, err := l.stateEnvelope(snapshot, state)
	if err != nil {
		return nil, err
	}
	return []executionbackend.ImplementationDataEnvelope{envelope}, nil
}

func (l *ExecutionLifecycle) envelopeWithTarget(snapshot executionbackend.LifecycleSnapshot, state orcaworker.NativeExecutionState, target executionbackend.ExecutionTarget) ([]executionbackend.ImplementationDataEnvelope, error) {
	copy := snapshot.Clone()
	copy.Ref.Target = target
	copy.Session.Target = target
	envelope, err := orcaworker.NewExecutionStateEnvelope(stateIdentity(copy), state)
	if err != nil {
		return nil, err
	}
	return []executionbackend.ImplementationDataEnvelope{envelope}, nil
}

func lifecycleReceipt(request executionbackend.LifecycleRequest) executionbackend.LifecycleReceipt {
	return executionbackend.LifecycleReceipt{Operation: request.Operation, Disposition: executionbackend.OperationUnknown}
}

func worktreeName(taskID, runID string) string {
	if !stableExecutionID(runID) || strings.TrimSpace(taskID) == "" {
		return ""
	}
	return "sl-" + taskID + "-" + runID
}

func targetWithWorkspace(target executionbackend.ExecutionTarget, id, branch, base string) executionbackend.ExecutionTarget {
	target.WorktreeID, target.Branch, target.BaseRevision = id, branch, base
	return target
}

func findTurn(state *orcaworker.NativeExecutionState, id string) *orcaworker.NativeTurnState {
	if state == nil {
		return nil
	}
	for i := range state.Turns {
		if state.Turns[i].TurnID == id {
			return &state.Turns[i]
		}
	}
	return nil
}

func findDispatchTurn(state *orcaworker.NativeExecutionState, dispatchID string) *orcaworker.NativeTurnState {
	if state == nil || dispatchID == "" {
		return nil
	}
	for i := range state.Turns {
		if state.Turns[i].DispatchID == dispatchID {
			return &state.Turns[i]
		}
	}
	return nil
}

func currentTurn(state *orcaworker.NativeExecutionState, snapshot executionbackend.LifecycleSnapshot) *orcaworker.NativeTurnState {
	if snapshot.Session.PendingCommandID != "" {
		if turn := findTurn(state, snapshot.Session.PendingCommandID); turn != nil {
			return turn
		}
	}
	id := snapshot.Session.CurrentTurnID
	if snapshot.Session.PendingTurnID != "" {
		id = snapshot.Session.PendingTurnID
	}
	if id == "" {
		id = snapshot.Session.CurrentCommandID
	}
	return findTurn(state, id)
}

func cloneWorkspace(value *executionbackend.WorkspaceIntent) *executionbackend.WorkspaceIntent {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneLifecycleTurn(value *executionbackend.TurnInput) *executionbackend.TurnInput {
	if value == nil {
		return nil
	}
	copy := *value
	copy.LinkedWorktreeArgs = append([]string(nil), value.LinkedWorktreeArgs...)
	return &copy
}

func processPointer(value executionbackend.ProcessIdentity) *executionbackend.ProcessIdentity {
	if value.Kind == "" {
		return nil
	}
	copy := value
	return &copy
}

func cloneEnvelopes(values []executionbackend.ImplementationDataEnvelope) []executionbackend.ImplementationDataEnvelope {
	cloned := make([]executionbackend.ImplementationDataEnvelope, len(values))
	for i := range values {
		cloned[i] = values[i].Clone()
	}
	return cloned
}

type remoteStopHandle struct {
	identity executionbackend.ProcessIdentity
}

func (h *remoteStopHandle) Identity() executionbackend.ProcessIdentity { return h.identity }

func (*remoteStopHandle) Kill() {}

func (*remoteStopHandle) Close() {}

type remoteLifecycleObserver struct {
	client   Client
	snapshot executionbackend.LifecycleSnapshot
	state    orcaworker.NativeExecutionState
	turn     orcaworker.NativeTurnState
	stop     *remoteStopHandle
	cancel   context.CancelFunc
	events   chan executionbackend.LifecycleEvent
	done     chan error
	closed   chan struct{}
}

func newLifecycleObserver(parent context.Context, lifecycle *ExecutionLifecycle, snapshot executionbackend.LifecycleSnapshot) executionbackend.LifecycleObserver {
	state, found, err := lifecycle.readState(snapshot)
	turn := currentTurn(&state, snapshot)
	// The single mutation's deadline ends after its receipt commits. Observing
	// the accepted worker has a separate lifetime, ended only by Close.
	ctx, cancel := context.WithCancel(context.WithoutCancel(parent))
	observer := &remoteLifecycleObserver{client: lifecycle.client, snapshot: snapshot.Clone(), state: state,
		stop: &remoteStopHandle{identity: snapshot.Ref.External}, cancel: cancel,
		events: make(chan executionbackend.LifecycleEvent, 16), done: make(chan error, 1), closed: make(chan struct{})}
	if err != nil || !found || turn == nil {
		go func() {
			defer close(observer.events)
			defer close(observer.done)
			if err != nil {
				observer.done <- err
			} else {
				observer.done <- errors.New("supervised observer has no fixed Task/Dispatch")
			}
			close(observer.closed)
		}()
		return observer
	}
	observer.turn = *turn
	go observer.observe(ctx)
	return observer
}

func (o *remoteLifecycleObserver) observe(ctx context.Context) {
	defer close(o.events)
	defer close(o.done)
	defer close(o.closed)
	ref := RemoteRef(o.snapshot.Ref)
	fixed := o.snapshot.Session
	fixed.TaskID, fixed.CurrentTurnID = o.turn.TaskID, o.turn.TurnID
	started := false
	completed := false
	for {
		delivery, err := o.client.Check(ctx, ref)
		if err != nil {
			if ctx.Err() == nil {
				o.done <- backendError(err)
			}
			return
		}
		if len(delivery.Messages) == 0 {
			// An empty coordinator inbox cannot prove that the original worker is
			// still attached. A lost Dispatch binding prevents both questions and
			// worker_done from reaching this inbox, even while the PTY stays live.
			if err := o.client.VerifyWorker(ctx, ref); err != nil {
				if ctx.Err() == nil {
					o.done <- backendError(err)
				}
				return
			}
			if delivery.TimedOut {
				continue
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(250 * time.Millisecond):
				continue
			}
		}
		batch, err := orcaworker.DecodeInbox(orcaworker.Inbox{RunID: o.snapshot.Ref.External.OrchestrationRun,
			DeliveryID: delivery.ID, Messages: delivery.Messages}, o.turn.TaskID, o.turn.DispatchID, o.snapshot.Ref.External.Terminal, fixed)
		if err != nil {
			o.done <- err
			return
		}
		if !started {
			for _, fact := range batch.Facts {
				if fact.Matched && fact.Kind != "" {
					if err := o.emitStarted(ctx); err != nil {
						o.done <- err
						return
					}
					started = true
					break
				}
			}
		}
		for i, fact := range batch.Facts {
			if fact.Kind == executionbackend.EventCompleted {
				if turn := findTurn(&o.state, o.turn.TurnID); turn != nil {
					turn.DoneMessageID, turn.Outcome, turn.LastMessage = fact.SourceEventID, fact.Outcome, fact.LastMessage
					turn.PRCandidates = append([]string(nil), fact.PRCandidates...)
					completed = true
				}
			}
			envelopes := []executionbackend.ImplementationDataEnvelope{}
			if completed {
				if envelope, saveErr := orcaworker.NewExecutionStateEnvelope(stateIdentity(o.snapshot), o.state); saveErr == nil {
					envelopes = append(envelopes, envelope)
				} else {
					o.done <- saveErr
					return
				}
			}
			persisted := make(chan error, 1)
			processed := make(chan error, 1)
			status := string(fact.Kind)
			if status == "" {
				status = "observed"
			}
			event := executionbackend.LifecycleEvent{Fact: fact, Implementation: envelopes,
				DeliveryID: delivery.ID, DeliveryFinal: i == len(batch.Facts)-1,
				Summary: string(fact.Kind), Kind: "execution", Status: status, Title: "Supervised worker event",
				Persisted: persisted, Processed: processed}
			select {
			case o.events <- event:
			case <-ctx.Done():
				return
			}
			select {
			case persistErr := <-persisted:
				if persistErr != nil {
					o.done <- persistErr
					return
				}
			case <-ctx.Done():
				return
			}
			select {
			case processErr := <-processed:
				if processErr != nil {
					o.done <- processErr
					return
				}
			case <-ctx.Done():
				return
			}
		}
		if completed {
			o.done <- nil
			return
		}
	}
}

func (o *remoteLifecycleObserver) Events() <-chan executionbackend.LifecycleEvent { return o.events }

func (o *remoteLifecycleObserver) Done() <-chan error { return o.done }

func (o *remoteLifecycleObserver) StopHandle() executionbackend.StopHandle { return o.stop }

func (o *remoteLifecycleObserver) Close(ctx context.Context) error {
	o.cancel()
	select {
	case <-o.closed:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func RemoteRef(ref executionbackend.ExecutionRef) Ref { return FromExecutionRef(ref) }

func lifecycleSnapshotWithBinding(snapshot executionbackend.LifecycleSnapshot, target executionbackend.ExecutionTarget, identity executionbackend.ProcessIdentity, state orcaworker.NativeExecutionState) executionbackend.LifecycleSnapshot {
	copy := snapshot.Clone()
	copy.Ref.Target, copy.Ref.External = target, identity
	copy.Session.Target = target
	copy.Session.SessionID, copy.Session.Generation = SessionID(identity), identity.Incarnation
	if turn := currentTurn(&state, copy); turn != nil {
		copy.Session.PendingTurnID, copy.Session.CurrentTurnID = turn.TurnID, turn.TurnID
		copy.Session.CurrentCommandID = turn.TurnID
	}
	return copy
}

var _ executionbackend.LifecyclePort = (*ExecutionLifecycle)(nil)

func (l *ExecutionLifecycle) InspectLifecycleWorkspace(ctx context.Context, snapshot executionbackend.LifecycleSnapshot) (executionbackend.WorkspaceInspection, error) {
	inspection, err := NewBackend(l.config).InspectWorkspace(ctx, snapshot.Ref)
	if err == nil {
		inspection.Dirty = snapshot.WorkspaceDirty
	}
	return inspection, err
}

func (o *remoteLifecycleObserver) emitStarted(ctx context.Context) error {
	started := executionbackend.LifecycleEvent{Fact: executionbackend.EventFact{Kind: executionbackend.EventStarted,
		StartedTurnID: o.turn.TurnID, SessionID: SessionID(o.snapshot.Ref.External)},
		Summary: "original Dispatch produced a verified worker event", Kind: "execution", Status: "started", Title: "Worker turn started",
		Persisted: make(chan error, 1), Processed: make(chan error, 1)}
	select {
	case o.events <- started:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-started.Persisted:
		if err != nil {
			return err
		}
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-started.Processed:
		if err != nil {
			return err
		}
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}
