package orcaworker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func (l *ExecutionLifecycle) prepareWorkspace(ctx context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, error) {
	receipt := nativeReceipt(request, executionbackend.OperationUnknown)
	intent := request.Input.Workspace
	if intent == nil || intent.BaseCommit == "" || intent.Branch == "" {
		return receipt, errors.New("native Orca workspace intent is incomplete")
	}
	repoID, err := l.repositoryID(ctx)
	if err != nil {
		return receipt, err
	}
	name, err := l.worktreeName(request.Snapshot)
	if err != nil {
		return receipt, err
	}
	comment := "SteerLine execution " + request.Snapshot.Session.ExecutionID
	if err := ensureIgnored(request.Snapshot.Session.ProjectID, "/"+outputDirectory+"/"); err != nil {
		return receipt, err
	}
	result, err := CreateFixedWorktree(ctx, l.client.MetaCall, repoID, name, intent.BaseCommit, comment)
	if err != nil {
		receipt.Diagnostic = err.Error()
		return receipt, err
	}
	worktree, process, bound, err := l.workspaceResult(ctx, request.Snapshot, result, repoID, name, intent.BaseCommit, nil)
	if err != nil {
		receipt.Diagnostic = err.Error()
		return receipt, err
	}
	state, _, err := l.readState(request.Snapshot)
	if err != nil {
		return receipt, err
	}
	state.RepositoryID = repoID
	state.LocalRuntimeID = result.RuntimeID
	state.AgentBound = bound
	inspection, err := WorkspaceBinding(worktree, l.setup.RepositoryPath, intent.BaseCommit)
	if err != nil {
		return receipt, err
	}
	target := request.Snapshot.Ref.Target
	target.WorktreeID, target.Branch = worktree.ID, inspection.Branch
	receipt.Target, receipt.Workspace = &target, &inspection
	receipt.Process = processPointer(process)
	receipt.State.WorktreeID = worktree.ID
	receipt.State.ResourceID = worktree.ID
	receipt.RequestID = result.RuntimeID
	receipt.Disposition = executionbackend.OperationConfirmed
	receipt.Operation.Receipt = append([]byte(nil), result.Payload...)
	receipt.Implementation, err = l.targetEnvelope(request.Snapshot, target, process, state)
	return receipt, err
}

func (l *ExecutionLifecycle) reconcileWorkspace(ctx context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, executionbackend.LifecycleObserver, error) {
	receipt := nativeReceipt(request, executionbackend.OperationUnknown)
	intent := request.Input.Workspace
	if intent == nil {
		return receipt, nil, errors.New("native Orca workspace reconciliation lost its original intent")
	}
	repoID, err := l.repositoryID(ctx)
	if err != nil {
		return receipt, nil, err
	}
	name, err := l.worktreeName(request.Snapshot)
	if err != nil {
		return receipt, nil, err
	}
	worktree, found, err := FindWorktree(ctx, l.client.Call, repoID, name, intent.BaseCommit)
	if err != nil || !found {
		return receipt, nil, err
	}
	if err := VerifyWorktreeOwner(worktree, request.Snapshot.Session.ExecutionID, request.Snapshot.Ref.Agent.Name, true); err != nil {
		return receipt, nil, err
	}
	var process executionbackend.ProcessIdentity
	bound := false
	return l.workspaceBoundReceipt(request, worktree, repoID, process, bound)
}

func (l *ExecutionLifecycle) workspaceBoundReceipt(request executionbackend.LifecycleRequest, worktree Worktree, repoID string,
	process executionbackend.ProcessIdentity, bound bool) (executionbackend.LifecycleReceipt, executionbackend.LifecycleObserver, error) {
	receipt := nativeReceipt(request, executionbackend.OperationConfirmed)
	state, _, err := l.readState(request.Snapshot)
	if err != nil {
		return receipt, nil, err
	}
	state.RepositoryID, state.AgentBound = repoID, bound
	inspection, err := WorkspaceBinding(worktree, l.setup.RepositoryPath, request.Input.Workspace.BaseCommit)
	if err != nil {
		return receipt, nil, err
	}
	target := request.Snapshot.Ref.Target
	target.WorktreeID, target.Branch = worktree.ID, inspection.Branch
	receipt.Target, receipt.Workspace = &target, &inspection
	receipt.Process = processPointer(process)
	receipt.RequestID, receipt.State.WorktreeID, receipt.State.ResourceID = process.RuntimeID, worktree.ID, worktree.ID
	receipt.Implementation, err = l.targetEnvelope(request.Snapshot, target, process, state)
	return receipt, nil, err
}

func (l *ExecutionLifecycle) workspaceResult(_ context.Context, snapshot executionbackend.LifecycleSnapshot, result Result,
	repoID, name, base string, _ []byte) (Worktree, executionbackend.ProcessIdentity, bool, error) {
	worktree, err := ParseFixedWorktreeReceipt(result, name, repoID, base)
	if err == nil {
		err = VerifyWorktreeOwner(worktree, snapshot.Session.ExecutionID, snapshot.Ref.Agent.Name, true)
	}
	return worktree, executionbackend.ProcessIdentity{}, false, err
}

func (l *ExecutionLifecycle) createResource(ctx context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, error) {
	intent := request.Input.Resource
	if intent == nil {
		return nativeReceipt(request, executionbackend.OperationRejected), errors.New("native Orca resource intent is missing")
	}
	switch intent.Purpose {
	case "coordinator":
		return l.createCoordinator(ctx, request)
	case "workflow":
		return l.createWorkflow(ctx, request)
	case "conversation":
		return l.createConversation(ctx, request)
	default:
		return nativeReceipt(request, executionbackend.OperationRejected), fmt.Errorf("unsupported native Orca resource %q", intent.Purpose)
	}
}

func (l *ExecutionLifecycle) reconcileResource(ctx context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, executionbackend.LifecycleObserver, error) {
	intent := request.Input.Resource
	if intent == nil {
		return nativeReceipt(request, executionbackend.OperationRejected), nil, errors.New("native Orca resource reconciliation lost its original intent")
	}
	switch intent.Purpose {
	case "coordinator":
		return l.reconcileCoordinator(ctx, request)
	case "workflow":
		return l.reconcileWorkflow(ctx, request)
	case "conversation":
		return l.reconcileConversation(ctx, request)
	default:
		return nativeReceipt(request, executionbackend.OperationUnknown), nil, nil
	}
}

func (l *ExecutionLifecycle) createCoordinator(ctx context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, error) {
	receipt := nativeReceipt(request, executionbackend.OperationUnknown)
	state, _, err := l.readState(request.Snapshot)
	if err != nil {
		return receipt, err
	}
	title := "SteerLine coordinator " + request.Snapshot.Session.WorkflowRunID
	marker := "SLCOORD:" + request.Snapshot.Session.ExecutionID
	result, err := CreateCoordinatorTerminal(ctx, l.client.MetaCall, l.setup.RepositoryPath, title, marker)
	if err != nil {
		receipt.Diagnostic = err.Error()
		return receipt, err
	}
	handle, err := ParseCoordinatorTerminalReceipt(result)
	if err != nil {
		return receipt, err
	}
	terminal, err := ShowTerminal(ctx, l.client.MetaCall, handle)
	if err != nil {
		return receipt, err
	}
	coordinator, err := BindCoordinator(terminal, l.setup.RepositoryPath, result.RuntimeID, l.orca.Executable)
	if err != nil {
		return receipt, err
	}
	if state.LocalRuntimeID != "" && state.LocalRuntimeID != coordinator.RuntimeID {
		return receipt, errors.New("Orca coordinator belongs to another original runtime")
	}
	state.LocalRuntimeID = coordinator.RuntimeID
	state.Coordinator = coordinator
	target := request.Snapshot.Ref.Target
	target.CoordinatorTerminal = coordinator.Terminal
	receipt.Target, receipt.RequestID = &target, result.RuntimeID
	receipt.State.ResourceID = coordinator.Terminal
	receipt.Disposition = executionbackend.OperationConfirmed
	receipt.Operation.Receipt = append([]byte(nil), result.Payload...)
	receipt.Implementation, err = l.targetEnvelope(request.Snapshot, target, request.Snapshot.Ref.External, state)
	return receipt, err
}

func (l *ExecutionLifecycle) reconcileCoordinator(ctx context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, executionbackend.LifecycleObserver, error) {
	receipt := nativeReceipt(request, executionbackend.OperationUnknown)
	state, _, err := l.readState(request.Snapshot)
	if err != nil {
		return receipt, nil, err
	}
	marker := "SLCOORD:" + request.Snapshot.Session.ExecutionID
	handle, err := FindOriginalCoordinator(ctx, l.client.MetaCall, l.setup.RepositoryPath, state.LocalRuntimeID, marker)
	if err != nil || handle == "" {
		return receipt, nil, err
	}
	terminal, err := ShowTerminal(ctx, l.client.MetaCall, handle)
	if err != nil {
		return receipt, nil, err
	}
	coordinator, err := BindCoordinator(terminal, l.setup.RepositoryPath, state.LocalRuntimeID, l.orca.Executable)
	if err != nil {
		return receipt, nil, err
	}
	state.Coordinator = coordinator
	target := request.Snapshot.Ref.Target
	target.CoordinatorTerminal = coordinator.Terminal
	receipt.Target, receipt.RequestID = &target, coordinator.Incarnation
	receipt.State.ResourceID = coordinator.Terminal
	receipt.Disposition = executionbackend.OperationConfirmed
	receipt.Implementation, err = l.targetEnvelope(request.Snapshot, target, request.Snapshot.Ref.External, state)
	return receipt, nil, err
}

func (l *ExecutionLifecycle) createWorkflow(ctx context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, error) {
	receipt := nativeReceipt(request, executionbackend.OperationUnknown)
	state, _, err := l.readState(request.Snapshot)
	if err != nil || state.Coordinator.Terminal == "" || state.Coordinator.RuntimeID == "" {
		return receipt, errors.Join(err, errors.New("native Orca Run requires its committed coordinator"))
	}
	objective := request.Input.Resource.Objective
	if objective == "" {
		return nativeReceipt(request, executionbackend.OperationRejected), errors.New("native Orca Run has no fixed objective")
	}
	created, err := CreateRun(ctx, l.client.MetaCall, objective, state.Coordinator.Terminal, state.Coordinator.RuntimeID)
	if err != nil {
		return receipt, err
	}
	target := request.Snapshot.Ref.Target
	target.OrchestrationRun, target.OrchestrationRunRequestID = created.ID, created.RequestID
	receipt.Target, receipt.RequestID = &target, created.RequestID
	receipt.State.ResourceID = created.ID
	receipt.Disposition = executionbackend.OperationConfirmed
	receipt.Operation.Receipt = append([]byte(nil), created.Payload...)
	receipt.Implementation, err = l.targetEnvelope(request.Snapshot, target, request.Snapshot.Ref.External, state)
	return receipt, err
}

func (l *ExecutionLifecycle) reconcileWorkflow(ctx context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, executionbackend.LifecycleObserver, error) {
	receipt := nativeReceipt(request, executionbackend.OperationUnknown)
	state, _, err := l.readState(request.Snapshot)
	if err != nil || state.Coordinator.Terminal == "" {
		return receipt, nil, errors.Join(err, errors.New("native Orca Run reconciliation has no coordinator"))
	}
	objective := request.Input.Resource.Objective
	runID, err := FindRun(ctx, l.client.Call, objective, state.Coordinator.Terminal)
	if err != nil || runID == "" {
		return receipt, nil, err
	}
	target := request.Snapshot.Ref.Target
	target.OrchestrationRun = runID
	receipt.Target, receipt.State.ResourceID = &target, runID
	receipt.Disposition = executionbackend.OperationConfirmed
	receipt.Implementation, err = l.targetEnvelope(request.Snapshot, target, request.Snapshot.Ref.External, state)
	return receipt, nil, err
}

func (l *ExecutionLifecycle) findBoundWorktree(ctx context.Context, snapshot executionbackend.LifecycleSnapshot) (Worktree, error) {
	if snapshot.Ref.Target.WorktreeID == "" {
		return Worktree{}, errors.New("native Orca resource has no fixed Worktree ID")
	}
	repoID := ""
	state, _, err := l.readState(snapshot)
	if err != nil {
		return Worktree{}, err
	}
	repoID = state.RepositoryID
	if repoID == "" {
		return Worktree{}, errors.New("native Orca resource has no fixed repository ID")
	}
	name, err := l.worktreeName(snapshot)
	if err != nil {
		return Worktree{}, err
	}
	worktree, found, err := FindWorktree(ctx, l.client.Call, repoID, name, snapshot.Ref.BaseCommit)
	if err != nil {
		return Worktree{}, err
	}
	if !found || worktree.ID != snapshot.Ref.Target.WorktreeID {
		return Worktree{}, &executionbackend.BackendError{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "native Orca Worktree differs from its committed binding"}
	}
	return worktree, nil
}

func (l *ExecutionLifecycle) createConversation(ctx context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, error) {
	receipt := nativeReceipt(request, executionbackend.OperationUnknown)
	intent := request.Input.Resource
	if intent == nil || intent.TurnID == "" || intent.Objective == "" {
		return nativeReceipt(request, executionbackend.OperationRejected), errors.New("native Orca Task lacks its fixed input")
	}
	state, _, err := l.readState(request.Snapshot)
	if err != nil || state.Coordinator.Terminal == "" || request.Snapshot.Ref.Target.OrchestrationRun == "" {
		return receipt, errors.Join(err, errors.New("native Orca Task requires its committed Run and coordinator"))
	}
	if nativeTurn(&state, intent.TurnID) != nil {
		return nativeReceipt(request, executionbackend.OperationRejected), errors.New("native Orca Task input already has a fixed turn record")
	}
	if request.Snapshot.Ref.External.Terminal != "" {
		if err := l.verifyAgent(ctx, request.Snapshot, request.Snapshot.Ref.External); err != nil {
			return nativeReceipt(request, executionbackend.OperationRejected), err
		}
	}
	absent, err := InspectCleanupTerminal(ctx, l.client.MetaCall, state.Coordinator, l.setup.RepositoryPath, "coordinator", false)
	if err != nil || absent {
		return nativeReceipt(request, executionbackend.OperationRejected), errors.Join(err, errors.New("original Task coordinator is unavailable"))
	}
	spec := l.taskSpec(request.Snapshot, intent.TurnID, intent.Objective)
	created, err := CreateTask(ctx, l.client.MetaCall, spec, intent.Name, request.Snapshot.Ref.Target.OrchestrationRun, state.Coordinator.Terminal, state.Coordinator.RuntimeID)
	if err != nil {
		return receipt, err
	}
	now := request.Operation.StartedAt.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	state.Turns = append(state.Turns, NativeTurnState{TurnID: intent.TurnID, TaskID: created.ID,
		TaskRequestID: created.RequestID, Purpose: intent.TurnPurpose, IntentAt: now})
	target := request.Snapshot.Ref.Target
	target.ExternalTask = created.ID
	receipt.Target, receipt.RequestID = &target, created.RequestID
	receipt.State.ResourceID, receipt.State.RuntimeID = created.ID, created.ID
	receipt.Disposition = executionbackend.OperationConfirmed
	receipt.Operation.Receipt = append([]byte(nil), created.Payload...)
	receipt.Implementation, err = l.targetEnvelope(request.Snapshot, target, request.Snapshot.Ref.External, state)
	return receipt, err
}

func (l *ExecutionLifecycle) reconcileConversation(ctx context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, executionbackend.LifecycleObserver, error) {
	receipt := nativeReceipt(request, executionbackend.OperationUnknown)
	intent := request.Input.Resource
	state, _, err := l.readState(request.Snapshot)
	if err != nil || state.Coordinator.Terminal == "" || request.Snapshot.Ref.Target.OrchestrationRun == "" {
		return receipt, nil, errors.Join(err, errors.New("native Orca Task reconciliation lacks its original Run"))
	}
	spec := l.taskSpec(request.Snapshot, intent.TurnID, intent.Objective)
	taskID, err := FindTask(ctx, l.client.Call, request.Snapshot.Ref.Target.OrchestrationRun, state.Coordinator.Terminal, spec)
	if err != nil || taskID == "" {
		return receipt, nil, err
	}
	turn := nativeTurn(&state, intent.TurnID)
	if turn == nil {
		now := request.Operation.StartedAt.UTC()
		if now.IsZero() {
			now = time.Now().UTC()
		}
		state.Turns = append(state.Turns, NativeTurnState{TurnID: intent.TurnID, TaskID: taskID, Purpose: intent.TurnPurpose, IntentAt: now})
		turn = &state.Turns[len(state.Turns)-1]
	} else if turn.TaskID != "" && turn.TaskID != taskID {
		return receipt, nil, &executionbackend.BackendError{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "reconciled Orca Task differs from the saved turn"}
	}
	turn.TaskID = taskID
	target := request.Snapshot.Ref.Target
	target.ExternalTask = taskID
	receipt.Target, receipt.State.ResourceID = &target, taskID
	receipt.Disposition = executionbackend.OperationConfirmed
	receipt.Implementation, err = l.targetEnvelope(request.Snapshot, target, request.Snapshot.Ref.External, state)
	return receipt, nil, err
}

func (l *ExecutionLifecycle) taskSpec(snapshot executionbackend.LifecycleSnapshot, turnID, prompt string) string {
	return prompt + OutputInstruction(snapshot.Session.WorkflowRunID, turnID)
}

func nativeTurn(state *NativeExecutionState, id string) *NativeTurnState {
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

func processPointer(identity executionbackend.ProcessIdentity) *executionbackend.ProcessIdentity {
	if identity.Kind == "" {
		return nil
	}
	copy := identity
	return &copy
}

func sameNativeTarget(a, b executionbackend.ExecutionRef) error {
	if a.Backend != b.Backend || a.Protocol != b.Protocol || a.Agent != b.Agent || a.Target.ID != b.Target.ID ||
		a.Target.ConnectionID != b.Target.ConnectionID || a.Target.ExternalRuntime != b.Target.ExternalRuntime {
		return &executionbackend.BackendError{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "native Orca configuration differs from the original execution target"}
	}
	return nil
}

func savedNativePayload(operation executionbackend.Operation) []byte {
	var receipt executionbackend.LifecycleReceipt
	if json.Unmarshal(operation.Receipt, &receipt) != nil {
		return nil
	}
	return append([]byte(nil), receipt.Operation.Receipt...)
}

func nativeCallError(err error) error {
	var platform *PlatformError
	if errors.As(err, &platform) {
		return &executionbackend.BackendError{Code: executionbackend.BackendErrorCode(platform.Code), Diagnostic: platform.Diagnostic, Cause: platform.Cause}
	}
	return err
}
