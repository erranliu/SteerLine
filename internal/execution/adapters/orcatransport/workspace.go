package orcatransport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	native "github.com/erranliu/SteerLine/internal/execution/adapters/protocol/orca"
	b "github.com/erranliu/SteerLine/internal/execution/backend"
)

// Stored inside the existing operation receipt, including on terminal-query failure.
// No resource is rediscovered by name when its creation response was lost.
type workspaceCreation struct {
	RuntimeID           string          `json:"runtime_id"`
	Worktree            native.Worktree `json:"worktree"`
	AgentTerminalHandle string          `json:"agentTerminalHandle,omitempty"`
	StartupTerminal     native.Terminal `json:"startupTerminal"`
	Terminal            native.Terminal `json:"selected_terminal"`
}

func (l *Lifecycle) prepareWorkspace(ctx context.Context, r b.LifecycleRequest, reconcile bool) (receipt b.LifecycleReceipt, err error) {
	receipt = b.LifecycleReceipt{Operation: r.Operation, Disposition: b.OperationUnknown}
	receipt.Operation.Receipt = nil // This field contains the checkpoint, not another outer receipt.
	s := r.Snapshot
	if r.Input.Workspace == nil || *r.Input.Workspace != s.WorkspacePlan || !b.StableExecutionID(s.Session.WorkflowRunID) {
		return receipt, errors.New("workspace differs from original plan")
	}
	name := "sl-" + s.Session.TaskID + "-" + s.Session.WorkflowRunID
	var created workspaceCreation
	if reconcile {
		var saved b.LifecycleReceipt
		if json.Unmarshal(r.Operation.Receipt, &saved) != nil || saved.ValidateFor(r) != nil ||
			json.Unmarshal(saved.Operation.Receipt, &created) != nil {
			return receipt, errors.New("original workspace creation has no exact saved creation receipt; no worktree/agent was recreated")
		}
		// Keep the checkpoint even when this reconciliation cannot query Orca.
		receipt.Operation.Receipt = append([]byte(nil), saved.Operation.Receipt...)
	} else {
		args := []string{"worktree", "create", "--repo", l.profile.RepositorySelector, "--name", name, "--base-branch", r.Input.Workspace.BaseCommit, "--no-parent", "--setup", "skip", "--activate", "--json"}
		if l.setup.AgentLaunchCommand == "" {
			args = append(args, "--agent", s.Ref.Agent.Name)
		}
		result, callErr := l.client.MetaCall(ctx, args...)
		if callErr != nil {
			return receipt, callErr
		}
		if err = json.Unmarshal(result.Payload, &created); err != nil {
			return receipt, err
		}
		created.RuntimeID = result.RuntimeID
	}
	worktree := created.Worktree
	if created.RuntimeID == "" || worktree.ID == "" || worktree.InstanceID == "" || worktree.Path == "" || worktree.Head != r.Input.Workspace.BaseCommit || worktree.BaseRef != r.Input.Workspace.BaseCommit || worktree.DisplayName != name {
		return receipt, errors.New("Orca worktree differs from fixed name/base")
	}
	defer func() { receipt.Operation.Receipt, _ = json.Marshal(created) }()
	wait, _ := time.ParseDuration(l.profile.WaitTimeout)
	ctx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()
	if reconcile {
		result, queryErr := l.client.MetaCall(ctx, "worktree", "show", "--worktree", "id:"+worktree.ID, "--json")
		if queryErr != nil {
			return receipt, queryErr
		}
		var shown struct {
			Worktree native.Worktree `json:"worktree"`
		}
		if json.Unmarshal(result.Payload, &shown) != nil || result.RuntimeID != created.RuntimeID || shown.Worktree != worktree {
			return receipt, errors.New("original Orca workspace creation identity changed")
		}
	}
	handle := created.AgentTerminalHandle
	if handle == "" {
		handle = created.StartupTerminal.Handle
	}
	if handle == "" || l.setup.AgentLaunchCommand != "" {
		if l.setup.AgentLaunchCommand == "" {
			return receipt, errors.New("agent launch did not provide a supported terminal handle; no task was injected")
		}
		sole, inventoryErr := l.soleWorkspaceTerminal(ctx, worktree.ID, created.RuntimeID)
		err = inventoryErr
		if err != nil {
			return receipt, err
		}
		if handle != "" && handle != sole.Handle {
			return receipt, errors.New("creation handle differs from the sole original terminal")
		}
		if created.Terminal.Handle != "" && (sole.Handle != created.Terminal.Handle ||
			created.Terminal.IncarnationID != "" && sole.IncarnationID != created.Terminal.IncarnationID) {
			return receipt, errors.New("original workspace terminal identity changed")
		}
		handle = sole.Handle
		created.Terminal = sole
	}
	if created.Terminal.Handle != "" && handle != created.Terminal.Handle {
		return receipt, errors.New("original workspace terminal handle changed")
	}
	// Retain the selected original handle if show is temporarily unavailable.
	created.Terminal.Handle = handle
	result, err := l.workspaceTerminalQuery(ctx, "terminal", "show", "--terminal", handle, "--json")
	if err != nil {
		return receipt, err
	}
	var shown struct {
		Terminal native.Terminal `json:"terminal"`
	}
	if err = json.Unmarshal(result.Payload, &shown); err != nil {
		return receipt, err
	}
	term := shown.Terminal
	term.RuntimeID = result.RuntimeID
	initializing := l.setup.AgentLaunchCommand != ""
	if term.Handle != handle || term.Orphaned || term.RuntimeID != created.RuntimeID || term.WorktreeID != worktree.ID || term.WorktreePath != worktree.Path || term.IncarnationID == "" ||
		created.Terminal.IncarnationID != "" && created.Terminal.IncarnationID != term.IncarnationID ||
		created.StartupTerminal.IncarnationID != "" && created.StartupTerminal.IncarnationID != term.IncarnationID ||
		!initializing && term.AgentIdentity != s.Ref.Agent.Name || initializing && term.AgentIdentity != "" || strings.TrimPrefix(term.Branch, "refs/heads/") != strings.TrimPrefix(worktree.Branch, "refs/heads/") && !(initializing && term.Branch == "") {
		return receipt, fmt.Errorf("original agent/worktree binding unverified (runtime=%q/%q worktree=%q/%q path=%q/%q incarnation=%q agent=%q initializing=%t branch=%q/%q)", term.RuntimeID, created.RuntimeID, term.WorktreeID, worktree.ID, term.WorktreePath, worktree.Path, term.IncarnationID, term.AgentIdentity, initializing, term.Branch, worktree.Branch)
	}
	created.Terminal = term
	p := b.ProcessIdentity{Kind: "orca_terminal", Executable: l.profile.Executable, RuntimeID: term.RuntimeID, Terminal: handle, Incarnation: term.IncarnationID, WorktreeID: worktree.ID, ExecutionHost: l.profile.Target().ExecutionHost}
	workspace := b.WorkspaceInspection{InspectionScope: "revision", Repository: s.Session.ProjectID, Worktree: worktree.Path, Branch: strings.TrimPrefix(worktree.Branch, "refs/heads/"), Base: r.Input.Workspace.BaseCommit, Head: worktree.Head, CheckedAt: time.Now().UTC()}
	receipt.Workspace = &workspace
	receipt.Process = &p
	target := s.Ref.Target
	target.WorktreeID = worktree.ID
	target.Branch = workspace.Branch
	target.BaseRevision = workspace.Base
	receipt.Target = &target
	receipt.State = b.LifecycleStateFacts{SessionID: SessionID(p), SessionGeneration: p.Incarnation, SessionAvailable: !initializing, WorkerLive: !initializing, WorktreeID: worktree.ID}
	receipt.Implementation, err = l.envelope(s, State{Process: p})
	if err == nil {
		receipt.Disposition = b.OperationConfirmed
	}
	return receipt, err
}

// Only the known surface-registration race is retryable. The caller supplies
// one deadline shared by inventory and show, and never retries creation/input.
func (l *Lifecycle) workspaceTerminalQuery(ctx context.Context, args ...string) (native.Result, error) {
	for {
		if err := ctx.Err(); err != nil {
			return native.Result{}, err
		}
		result, err := l.client.MetaCall(ctx, args...)
		if err == nil || result.ErrorCode != "terminal_surface_ownership_unavailable" && result.ErrorMessage != "terminal_surface_ownership_unavailable" {
			return result, err
		}
		if waitErr := waitWorkspaceTerminal(ctx); waitErr != nil {
			return result, fmt.Errorf("original workspace terminal ownership did not become ready: %w", waitErr)
		}
	}
}

func waitWorkspaceTerminal(ctx context.Context) error {
	timer := time.NewTimer(200 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func InspectWorkspace(ctx context.Context, client native.Client, ref b.ExecutionRef, project string) (b.WorkspaceInspection, error) {
	result, err := client.MetaCall(ctx, "worktree", "show", "--worktree", "id:"+ref.External.WorktreeID, "--json")
	if err != nil {
		return b.WorkspaceInspection{}, err
	}
	var shown struct {
		Worktree native.Worktree `json:"worktree"`
	}
	if json.Unmarshal(result.Payload, &shown) != nil || result.RuntimeID != ref.External.RuntimeID || shown.Worktree.ID != ref.External.WorktreeID || shown.Worktree.Path != ref.WorkspacePath || strings.TrimPrefix(shown.Worktree.Branch, "refs/heads/") != ref.WorkspaceBranch || len(shown.Worktree.Head) != 40 {
		return b.WorkspaceInspection{}, errors.New("original Orca revision identity changed")
	}
	// This is deliberately revision evidence on BOTH hosts. No center-local
	// filesystem access or fictitious remote working-tree cleanliness is used.
	value := b.WorkspaceInspection{InspectionScope: "revision", Repository: project, Worktree: shown.Worktree.Path, Branch: ref.WorkspaceBranch, Base: ref.BaseCommit, Head: shown.Worktree.Head, CheckedAt: time.Now().UTC()}
	return value, nil
}
