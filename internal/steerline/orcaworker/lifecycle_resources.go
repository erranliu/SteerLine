package orcaworker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

func (l *ExecutionLifecycle) prepareWorkspace(ctx context.Context, request executionapi.LifecycleRequest) (executionapi.LifecycleReceipt, error) {
	receipt := nativeReceipt(request, executionapi.OperationUnknown)
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
	var result Result
	if l.fixedAgentLaunch() {
		result, err = CreateFixedWorktree(ctx, l.client.MetaCall, repoID, name, intent.BaseCommit, comment)
	} else {
		result, err = CreateNativeWorktree(ctx, l.client.MetaCall, repoID, name, intent.BaseCommit, "codex", comment)
	}
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
	receipt.Disposition = executionapi.OperationConfirmed
	receipt.Operation.Receipt = append([]byte(nil), result.Payload...)
	receipt.Implementation, err = l.targetEnvelope(request.Snapshot, target, process, state)
	return receipt, err
}

func (l *ExecutionLifecycle) reconcileWorkspace(ctx context.Context, request executionapi.LifecycleRequest) (executionapi.LifecycleReceipt, executionapi.LifecycleObserver, error) {
	receipt := nativeReceipt(request, executionapi.OperationUnknown)
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
	if err := VerifyWorktreeOwner(worktree, request.Snapshot.Session.ExecutionID, "codex", l.fixedAgentLaunch()); err != nil {
		return receipt, nil, err
	}
	var process executionapi.ProcessIdentity
	bound := false
	if !l.fixedAgentLaunch() {
		terminals, _, listErr := ListTerminals(ctx, l.client.MetaCall, "id:"+worktree.ID)
		if listErr != nil || len(terminals) != 1 {
			return receipt, nil, errors.Join(listErr, errors.New("native Orca worktree has no unique worker terminal"))
		}
		process, bound, err = l.agentOnWorktree(ctx, worktree, terminals[0].Handle)
		if err != nil {
			return receipt, nil, err
		}
	}
	return l.workspaceBoundReceipt(request, worktree, repoID, process, bound)
}

func (l *ExecutionLifecycle) workspaceBoundReceipt(request executionapi.LifecycleRequest, worktree Worktree, repoID string,
	process executionapi.ProcessIdentity, bound bool) (executionapi.LifecycleReceipt, executionapi.LifecycleObserver, error) {
	receipt := nativeReceipt(request, executionapi.OperationConfirmed)
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

func (l *ExecutionLifecycle) workspaceResult(ctx context.Context, snapshot executionapi.LifecycleSnapshot, result Result,
	repoID, name, base string, saved []byte) (Worktree, executionapi.ProcessIdentity, bool, error) {
	var worktree Worktree
	var handle string
	var err error
	if l.fixedAgentLaunch() {
		worktree, err = ParseFixedWorktreeReceipt(result, name, repoID, base)
	} else {
		var parsed NativeWorktreeReceipt
		parsed, err = ParseNativeWorktreeReceipt(result, name, repoID, base)
		worktree, handle = parsed.Worktree, parsed.Handle
	}
	if err != nil {
		return Worktree{}, executionapi.ProcessIdentity{}, false, err
	}
	if err := VerifyWorktreeOwner(worktree, snapshot.Session.ExecutionID, "codex", l.fixedAgentLaunch()); err != nil {
		return Worktree{}, executionapi.ProcessIdentity{}, false, err
	}
	if l.fixedAgentLaunch() {
		return worktree, executionapi.ProcessIdentity{}, false, nil
	}
	process, bound, err := l.agentOnWorktree(ctx, worktree, handle)
	if err != nil {
		return Worktree{}, executionapi.ProcessIdentity{}, false, err
	}
	return worktree, process, bound, nil
}

func (l *ExecutionLifecycle) agentOnWorktree(ctx context.Context, worktree Worktree, handle string) (executionapi.ProcessIdentity, bool, error) {
	identity, err := BindAgent(ctx, l.client.MetaCall, worktree, handle, "codex", l.orca.Executable)
	if err == nil {
		identity.Kind = "orca"
		return identity, true, nil
	}
	if !errors.Is(err, ErrAgentStarting) {
		return executionapi.ProcessIdentity{}, false, err
	}
	if handle == "" {
		return executionapi.ProcessIdentity{}, false, err
	}
	terminal, showErr := ShowTerminal(ctx, l.client.MetaCall, handle)
	if showErr != nil {
		return executionapi.ProcessIdentity{}, false, showErr
	}
	if terminal.WorktreeID != worktree.ID || terminal.RuntimeID == "" || terminal.IncarnationID == "" {
		return executionapi.ProcessIdentity{}, false, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "new Orca agent terminal differs from the created worktree"}
	}
	process := TerminalProcess(terminal, l.orca.Executable)
	process.Kind, process.WorktreeID = "orca", worktree.ID
	return process, false, nil
}

func (l *ExecutionLifecycle) createResource(ctx context.Context, request executionapi.LifecycleRequest) (executionapi.LifecycleReceipt, error) {
	intent := request.Input.Resource
	if intent == nil {
		return nativeReceipt(request, executionapi.OperationRejected), errors.New("native Orca resource intent is missing")
	}
	switch intent.Purpose {
	case "agent-terminal":
		return l.createAgentTerminal(ctx, request)
	case "agent-bind":
		return l.bindAgent(ctx, request), nil
	case "coordinator":
		return l.createCoordinator(ctx, request)
	case "workflow":
		return l.createWorkflow(ctx, request)
	case "relay-ignore":
		return l.prepareRelayIgnore(ctx, request)
	case "conversation":
		return l.createConversation(ctx, request)
	default:
		return nativeReceipt(request, executionapi.OperationRejected), fmt.Errorf("unsupported native Orca resource %q", intent.Purpose)
	}
}

func (l *ExecutionLifecycle) reconcileResource(ctx context.Context, request executionapi.LifecycleRequest) (executionapi.LifecycleReceipt, executionapi.LifecycleObserver, error) {
	intent := request.Input.Resource
	if intent == nil {
		return nativeReceipt(request, executionapi.OperationRejected), nil, errors.New("native Orca resource reconciliation lost its original intent")
	}
	switch intent.Purpose {
	case "agent-terminal":
		return l.reconcileAgentTerminal(ctx, request)
	case "agent-bind":
		return l.bindAgent(ctx, request), nil, nil
	case "coordinator":
		return l.reconcileCoordinator(ctx, request)
	case "workflow":
		return l.reconcileWorkflow(ctx, request)
	case "relay-ignore":
		return l.reconcileRelayIgnore(ctx, request)
	case "conversation":
		return l.reconcileConversation(ctx, request)
	default:
		return nativeReceipt(request, executionapi.OperationUnknown), nil, nil
	}
}

func (l *ExecutionLifecycle) createAgentTerminal(ctx context.Context, request executionapi.LifecycleRequest) (executionapi.LifecycleReceipt, error) {
	receipt := nativeReceipt(request, executionapi.OperationUnknown)
	if !l.fixedAgentLaunch() || request.Snapshot.Ref.Target.WorktreeID == "" {
		return nativeReceipt(request, executionapi.OperationRejected), errors.New("fixed Orca agent launch has no fixed Worktree")
	}
	worktree, err := l.findBoundWorktree(ctx, request.Snapshot)
	if err != nil {
		return receipt, err
	}
	command, err := l.agentLaunchCommand(request.Snapshot, worktree.Path)
	if err != nil {
		return receipt, err
	}
	wrapped, err := WrapAgentCommand(command, l.orca.Shell)
	if err != nil {
		return receipt, err
	}
	result, err := CreateFixedAgentTerminal(ctx, l.client.MetaCall, worktree.ID, FixedAgentTitle(request.Snapshot.Session.ExecutionID), l.orca.Shell, wrapped)
	if err != nil {
		receipt.Diagnostic = err.Error()
		return receipt, err
	}
	terminal, err := ParseFixedAgentTerminalReceipt(result, worktree.ID, FixedAgentTitle(request.Snapshot.Session.ExecutionID))
	if err != nil {
		receipt.Diagnostic = err.Error()
		return receipt, err
	}
	process, bound, err := l.processForTerminal(ctx, terminal.Handle, worktree, false)
	if err != nil {
		return receipt, err
	}
	state, _, err := l.readState(request.Snapshot)
	if err != nil {
		return receipt, err
	}
	state.AgentBound = bound
	state.AgentLaunchCommand = wrapped
	receipt.State.ResourcePending = !bound
	receipt.State.LaunchCommand = wrapped
	receipt.Process = &process
	receipt.State.ResourceID = process.Terminal
	receipt.State.SessionID, receipt.State.SessionGeneration = SessionID(process), process.Incarnation
	receipt.RequestID = result.RuntimeID
	receipt.Disposition = executionapi.OperationAccepted
	receipt.Operation.Receipt = append([]byte(nil), result.Payload...)
	receipt.Implementation, err = l.targetEnvelope(request.Snapshot, request.Snapshot.Ref.Target, process, state)
	return receipt, err
}

func (l *ExecutionLifecycle) reconcileAgentTerminal(ctx context.Context, request executionapi.LifecycleRequest) (executionapi.LifecycleReceipt, executionapi.LifecycleObserver, error) {
	receipt := nativeReceipt(request, executionapi.OperationUnknown)
	if !l.fixedAgentLaunch() || request.Snapshot.Ref.Target.WorktreeID == "" {
		return nativeReceipt(request, executionapi.OperationRejected), nil, errors.New("fixed Orca agent reconciliation has no Worktree")
	}
	worktree, err := l.findBoundWorktree(ctx, request.Snapshot)
	if err != nil {
		return receipt, nil, err
	}
	terminals, _, err := ListTerminals(ctx, l.client.MetaCall, "id:"+worktree.ID)
	if err != nil {
		return receipt, nil, err
	}
	title := FixedAgentTitle(request.Snapshot.Session.ExecutionID)
	saved := savedNativePayload(request.Operation)
	terminal, err := FixedAgentTerminal(terminals, saved, title, worktree.ID)
	if err != nil || terminal.Handle == "" {
		return receipt, nil, err
	}
	process, bound, err := l.processForTerminal(ctx, terminal.Handle, worktree, false)
	if err != nil {
		return receipt, nil, err
	}
	state, _, err := l.readState(request.Snapshot)
	if err != nil {
		return receipt, nil, err
	}
	state.AgentBound = bound
	receipt.State.ResourcePending = !bound
	command, err := l.agentLaunchCommand(request.Snapshot, worktree.Path)
	if err != nil {
		return receipt, nil, err
	}
	wrapper, err := WrapAgentCommand(command, l.orca.Shell)
	if err != nil {
		return receipt, nil, err
	}
	state.AgentLaunchCommand = wrapper
	receipt.State.LaunchCommand = wrapper
	receipt.Process = &process
	receipt.RequestID, receipt.Disposition = process.RuntimeID, executionapi.OperationAccepted
	receipt.State.SessionID, receipt.State.SessionGeneration = SessionID(process), process.Incarnation
	receipt.State.ResourceID = process.Terminal
	receipt.Implementation, err = l.targetEnvelope(request.Snapshot, request.Snapshot.Ref.Target, process, state)
	return receipt, nil, err
}

func (l *ExecutionLifecycle) processForTerminal(ctx context.Context, handle string, worktree Worktree, requireAgent bool) (executionapi.ProcessIdentity, bool, error) {
	terminal, err := ShowTerminal(ctx, l.client.MetaCall, handle)
	if err != nil {
		return executionapi.ProcessIdentity{}, false, err
	}
	// Title is allowed to change after startup; worktree/handle/incarnation are fixed.
	if terminal.WorktreeID != worktree.ID || terminal.Handle != handle || terminal.IncarnationID == "" {
		return executionapi.ProcessIdentity{}, false, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "fixed Orca agent terminal changed identity"}
	}
	if requireAgent && terminal.AgentIdentity != "codex" {
		return executionapi.ProcessIdentity{}, false, ErrAgentStarting
	}
	process := TerminalProcess(terminal, l.orca.Executable)
	process.Kind, process.WorktreeID = "orca", worktree.ID
	return process, terminal.AgentIdentity == "codex", nil
}

func (l *ExecutionLifecycle) bindAgent(ctx context.Context, request executionapi.LifecycleRequest) executionapi.LifecycleReceipt {
	receipt := nativeReceipt(request, executionapi.OperationUnknown)
	worktree, err := l.findBoundWorktree(ctx, request.Snapshot)
	if err != nil {
		receipt.Diagnostic = err.Error()
		return receipt
	}
	process := request.Snapshot.Ref.External
	if process.Terminal == "" {
		return nativeReceipt(request, executionapi.OperationRejected)
	}
	err = VerifyAgentTerminal(ctx, l.client.MetaCall, process, worktree.Path, strings.TrimPrefix(worktree.Branch, "refs/heads/"), "codex")
	if err != nil && !errors.Is(err, ErrAgentStarting) {
		receipt.Diagnostic = err.Error()
		return receipt
	}
	bound := err == nil
	state, _, err := l.readState(request.Snapshot)
	if err != nil {
		receipt.Diagnostic = err.Error()
		return receipt
	}
	state.AgentBound = bound
	receipt.Process = &process
	receipt.RequestID = process.Incarnation
	receipt.Disposition = executionapi.OperationConfirmed
	if !bound {
		receipt.Disposition = executionapi.OperationAccepted
		receipt.State.ResourcePending = true
	}
	receipt.State.ResourceID = process.Terminal
	receipt.State.SessionID, receipt.State.SessionGeneration = SessionID(process), process.Incarnation
	receipt.Implementation, err = l.targetEnvelope(request.Snapshot, request.Snapshot.Ref.Target, process, state)
	if err != nil {
		receipt.Disposition = executionapi.OperationUnknown
	}
	return receipt
}

func (l *ExecutionLifecycle) createCoordinator(ctx context.Context, request executionapi.LifecycleRequest) (executionapi.LifecycleReceipt, error) {
	receipt := nativeReceipt(request, executionapi.OperationUnknown)
	if request.Snapshot.Ref.External.Terminal == "" {
		return nativeReceipt(request, executionapi.OperationRejected), errors.New("native Orca coordinator has no fixed agent")
	}
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
	state.Coordinator = coordinator
	target := request.Snapshot.Ref.Target
	target.CoordinatorTerminal = coordinator.Terminal
	receipt.Target, receipt.RequestID = &target, result.RuntimeID
	receipt.State.ResourceID = coordinator.Terminal
	receipt.Disposition = executionapi.OperationConfirmed
	receipt.Operation.Receipt = append([]byte(nil), result.Payload...)
	receipt.Implementation, err = l.targetEnvelope(request.Snapshot, target, request.Snapshot.Ref.External, state)
	return receipt, err
}

func (l *ExecutionLifecycle) reconcileCoordinator(ctx context.Context, request executionapi.LifecycleRequest) (executionapi.LifecycleReceipt, executionapi.LifecycleObserver, error) {
	receipt := nativeReceipt(request, executionapi.OperationUnknown)
	state, _, err := l.readState(request.Snapshot)
	if err != nil {
		return receipt, nil, err
	}
	marker := "SLCOORD:" + request.Snapshot.Session.ExecutionID
	handle, err := FindOriginalCoordinator(ctx, l.client.MetaCall, l.setup.RepositoryPath, request.Snapshot.Ref.External.RuntimeID, marker)
	if err != nil || handle == "" {
		return receipt, nil, err
	}
	terminal, err := ShowTerminal(ctx, l.client.MetaCall, handle)
	if err != nil {
		return receipt, nil, err
	}
	coordinator, err := BindCoordinator(terminal, l.setup.RepositoryPath, request.Snapshot.Ref.External.RuntimeID, l.orca.Executable)
	if err != nil {
		return receipt, nil, err
	}
	state.Coordinator = coordinator
	target := request.Snapshot.Ref.Target
	target.CoordinatorTerminal = coordinator.Terminal
	receipt.Target, receipt.RequestID = &target, coordinator.Incarnation
	receipt.State.ResourceID = coordinator.Terminal
	receipt.Disposition = executionapi.OperationConfirmed
	receipt.Implementation, err = l.targetEnvelope(request.Snapshot, target, request.Snapshot.Ref.External, state)
	return receipt, nil, err
}

func (l *ExecutionLifecycle) createWorkflow(ctx context.Context, request executionapi.LifecycleRequest) (executionapi.LifecycleReceipt, error) {
	receipt := nativeReceipt(request, executionapi.OperationUnknown)
	state, _, err := l.readState(request.Snapshot)
	if err != nil || state.Coordinator.Terminal == "" || state.Coordinator.RuntimeID == "" {
		return receipt, errors.Join(err, errors.New("native Orca Run requires its committed coordinator"))
	}
	objective := request.Input.Resource.Objective
	if objective == "" {
		return nativeReceipt(request, executionapi.OperationRejected), errors.New("native Orca Run has no fixed objective")
	}
	created, err := CreateRun(ctx, l.client.MetaCall, objective, state.Coordinator.Terminal, state.Coordinator.RuntimeID)
	if err != nil {
		return receipt, err
	}
	target := request.Snapshot.Ref.Target
	target.OrchestrationRun, target.OrchestrationRunRequestID = created.ID, created.RequestID
	receipt.Target, receipt.RequestID = &target, created.RequestID
	receipt.State.ResourceID = created.ID
	receipt.Disposition = executionapi.OperationConfirmed
	receipt.Operation.Receipt = append([]byte(nil), created.Payload...)
	receipt.Implementation, err = l.targetEnvelope(request.Snapshot, target, request.Snapshot.Ref.External, state)
	return receipt, err
}

func (l *ExecutionLifecycle) reconcileWorkflow(ctx context.Context, request executionapi.LifecycleRequest) (executionapi.LifecycleReceipt, executionapi.LifecycleObserver, error) {
	receipt := nativeReceipt(request, executionapi.OperationUnknown)
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
	receipt.Disposition = executionapi.OperationConfirmed
	receipt.Implementation, err = l.targetEnvelope(request.Snapshot, target, request.Snapshot.Ref.External, state)
	return receipt, nil, err
}

func (l *ExecutionLifecycle) prepareRelayIgnore(ctx context.Context, request executionapi.LifecycleRequest) (executionapi.LifecycleReceipt, error) {
	receipt := nativeReceipt(request, executionapi.OperationUnknown)
	gitDir, err := l.repositoryGitDir(ctx)
	if err != nil {
		return receipt, err
	}
	if err := EnsureRelayIgnored(gitDir); err != nil {
		return receipt, err
	}
	state, _, err := l.readState(request.Snapshot)
	if err != nil {
		return receipt, err
	}
	state.RelayIgnored = true
	receipt.Disposition = executionapi.OperationConfirmed
	receipt.State.ResourceID = "relay-ignore"
	receipt.Implementation, err = l.targetEnvelope(request.Snapshot, request.Snapshot.Ref.Target, request.Snapshot.Ref.External, state)
	return receipt, err
}

func (l *ExecutionLifecycle) reconcileRelayIgnore(ctx context.Context, request executionapi.LifecycleRequest) (executionapi.LifecycleReceipt, executionapi.LifecycleObserver, error) {
	receipt := nativeReceipt(request, executionapi.OperationUnknown)
	gitDir, err := l.repositoryGitDir(ctx)
	if err != nil {
		return receipt, nil, err
	}
	ignored, err := RelayIgnored(gitDir)
	if err != nil || !ignored {
		return receipt, nil, err
	}
	state, _, err := l.readState(request.Snapshot)
	if err != nil {
		return receipt, nil, err
	}
	state.RelayIgnored = true
	receipt.Disposition = executionapi.OperationConfirmed
	receipt.State.ResourceID = "relay-ignore"
	receipt.Implementation, err = l.targetEnvelope(request.Snapshot, request.Snapshot.Ref.Target, request.Snapshot.Ref.External, state)
	return receipt, nil, err
}

func (l *ExecutionLifecycle) repositoryGitDir(ctx context.Context) (string, error) {
	executable := l.setup.GitExecutable
	if executable == "" {
		executable = "git"
	}
	args := append([]string(nil), l.setup.GitArgs...)
	args = append(args, "rev-parse", "--git-common-dir")
	out, err := l.setup.ProcessHost.OutputCommand(ctx, executionapi.LaunchCommand{Executable: executable, Args: args, WorkingDirectory: l.setup.RepositoryPath})
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(string(out))
	if value == "" {
		return "", errors.New("native Orca relay has no repository git directory")
	}
	if !filepath.IsAbs(value) {
		value = filepath.Join(l.setup.RepositoryPath, value)
	}
	return filepath.Abs(value)
}

func (l *ExecutionLifecycle) findBoundWorktree(ctx context.Context, snapshot executionapi.LifecycleSnapshot) (Worktree, error) {
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
		return Worktree{}, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "native Orca Worktree differs from its committed binding"}
	}
	return worktree, nil
}

func (l *ExecutionLifecycle) createConversation(ctx context.Context, request executionapi.LifecycleRequest) (executionapi.LifecycleReceipt, error) {
	receipt := nativeReceipt(request, executionapi.OperationUnknown)
	intent := request.Input.Resource
	if intent == nil || intent.TurnID == "" || intent.Objective == "" {
		return nativeReceipt(request, executionapi.OperationRejected), errors.New("native Orca Task lacks its fixed input")
	}
	state, _, err := l.readState(request.Snapshot)
	if err != nil || state.Coordinator.Terminal == "" || request.Snapshot.Ref.Target.OrchestrationRun == "" {
		return receipt, errors.Join(err, errors.New("native Orca Task requires its committed Run and coordinator"))
	}
	if nativeTurn(&state, intent.TurnID) != nil {
		return nativeReceipt(request, executionapi.OperationRejected), errors.New("native Orca Task input already has a fixed turn record")
	}
	if err := l.verifyAgent(ctx, request.Snapshot, request.Snapshot.Ref.External); err != nil {
		return nativeReceipt(request, executionapi.OperationRejected), err
	}
	absent, err := InspectCleanupTerminal(ctx, l.client.MetaCall, state.Coordinator, l.setup.RepositoryPath, "coordinator", false)
	if err != nil || absent {
		return nativeReceipt(request, executionapi.OperationRejected), errors.Join(err, errors.New("original Task coordinator is unavailable"))
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
	receipt.Disposition = executionapi.OperationConfirmed
	receipt.Operation.Receipt = append([]byte(nil), created.Payload...)
	receipt.Implementation, err = l.targetEnvelope(request.Snapshot, target, request.Snapshot.Ref.External, state)
	return receipt, err
}

func (l *ExecutionLifecycle) reconcileConversation(ctx context.Context, request executionapi.LifecycleRequest) (executionapi.LifecycleReceipt, executionapi.LifecycleObserver, error) {
	receipt := nativeReceipt(request, executionapi.OperationUnknown)
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
		return receipt, nil, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "reconciled Orca Task differs from the saved turn"}
	}
	turn.TaskID = taskID
	target := request.Snapshot.Ref.Target
	target.ExternalTask = taskID
	receipt.Target, receipt.State.ResourceID = &target, taskID
	receipt.Disposition = executionapi.OperationConfirmed
	receipt.Implementation, err = l.targetEnvelope(request.Snapshot, target, request.Snapshot.Ref.External, state)
	return receipt, nil, err
}

func (l *ExecutionLifecycle) taskSpec(snapshot executionapi.LifecycleSnapshot, turnID, prompt string) string {
	return prompt + RelayInstruction(snapshot.Session.WorkflowRunID, turnID)
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

func processPointer(identity executionapi.ProcessIdentity) *executionapi.ProcessIdentity {
	if identity.Kind == "" {
		return nil
	}
	copy := identity
	return &copy
}

func sameNativeTarget(a, b executionapi.ExecutionRef) error {
	if a.Backend != b.Backend || a.Protocol != b.Protocol || a.Agent != b.Agent || a.Target.ID != b.Target.ID ||
		a.Target.ConnectionID != b.Target.ConnectionID || a.Target.ExternalRuntime != b.Target.ExternalRuntime {
		return &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "native Orca configuration differs from the original execution target"}
	}
	return nil
}

func savedNativePayload(operation executionapi.Operation) []byte {
	var receipt executionapi.LifecycleReceipt
	if json.Unmarshal(operation.Receipt, &receipt) != nil {
		return nil
	}
	return append([]byte(nil), receipt.Operation.Receipt...)
}

func nativeCallError(err error) error {
	var platform *PlatformError
	if errors.As(err, &platform) {
		return &executionapi.BackendError{Code: executionapi.BackendErrorCode(platform.Code), Diagnostic: platform.Diagnostic, Cause: platform.Cause}
	}
	return err
}

func parseTerminalFromSaved(payload []byte, worktreeID, title string) (Terminal, error) {
	if len(payload) == 0 {
		return Terminal{}, errors.New("saved Orca terminal receipt is missing")
	}
	var envelope struct {
		Terminal Terminal `json:"terminal"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil || envelope.Terminal.Handle == "" || envelope.Terminal.WorktreeID != worktreeID || envelope.Terminal.Title != title {
		return Terminal{}, errors.Join(err, errors.New("saved Orca terminal receipt differs from its fixed intent"))
	}
	return envelope.Terminal, nil
}
