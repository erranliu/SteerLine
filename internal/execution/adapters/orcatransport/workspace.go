package orcatransport

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	native "github.com/erranliu/SteerLine/internal/execution/adapters/protocol/orca"
	b "github.com/erranliu/SteerLine/internal/execution/backend"
	"github.com/erranliu/SteerLine/internal/execution/workerreturn"
)

func (l *Lifecycle) prepareWorkspace(ctx context.Context, r b.LifecycleRequest, reconcile bool) (b.LifecycleReceipt, error) {
	receipt := b.LifecycleReceipt{Operation: r.Operation, Disposition: b.OperationUnknown}
	s := r.Snapshot
	if r.Input.Workspace == nil || *r.Input.Workspace != s.WorkspacePlan || !b.StableExecutionID(s.Session.WorkflowRunID) {
		return receipt, errors.New("workspace differs from original plan")
	}
	if reconcile {
		return receipt, errors.New("original workspace creation has no exact saved agent receipt; no worktree/agent was recreated")
	}
	name := "sl-" + s.Session.TaskID + "-" + s.Session.WorkflowRunID
	result, err := l.client.MetaCall(ctx, "worktree", "create", "--repo", l.profile.RepositorySelector, "--name", name, "--base-branch", r.Input.Workspace.BaseCommit, "--no-parent", "--setup", "skip", "--agent", s.Ref.Agent.Name, "--json")
	if err != nil {
		return receipt, err
	}
	var created struct {
		Worktree            native.Worktree `json:"worktree"`
		AgentTerminalHandle string          `json:"agentTerminalHandle"`
		StartupTerminal     native.Terminal `json:"startupTerminal"`
	}
	if err = json.Unmarshal(result.Payload, &created); err != nil {
		return receipt, err
	}
	worktree := created.Worktree
	if worktree.ID == "" || worktree.Path == "" || worktree.Head != r.Input.Workspace.BaseCommit || worktree.BaseRef != r.Input.Workspace.BaseCommit || worktree.DisplayName != name {
		return receipt, errors.New("Orca worktree differs from fixed name/base")
	}
	handle := created.AgentTerminalHandle
	if handle == "" {
		handle = created.StartupTerminal.Handle
	}
	if handle == "" {
		return receipt, errors.New("agent launch did not provide a supported terminal handle; no task was injected")
	}
	term, err := l.client.ShowTerminal(ctx, handle)
	if err != nil {
		return receipt, err
	}
	if term.RuntimeID != result.RuntimeID || term.WorktreeID != worktree.ID || term.WorktreePath != worktree.Path || term.IncarnationID == "" || term.AgentIdentity != s.Ref.Agent.Name || strings.TrimPrefix(term.Branch, "refs/heads/") != strings.TrimPrefix(worktree.Branch, "refs/heads/") {
		return receipt, errors.New("original agent/worktree binding unverified")
	}
	p := b.ProcessIdentity{Kind: "orca_terminal", Executable: l.profile.Executable, RuntimeID: term.RuntimeID, Terminal: handle, Incarnation: term.IncarnationID, WorktreeID: worktree.ID, ExecutionHost: l.profile.Target().ExecutionHost}
	workspace := b.WorkspaceInspection{InspectionScope: "revision", Repository: s.Session.ProjectID, Worktree: worktree.Path, Branch: strings.TrimPrefix(worktree.Branch, "refs/heads/"), Base: r.Input.Workspace.BaseCommit, Head: worktree.Head, CheckedAt: time.Now().UTC()}
	receipt.Workspace = &workspace
	receipt.Process = &p
	target := s.Ref.Target
	target.WorktreeID = worktree.ID
	target.Branch = workspace.Branch
	target.BaseRevision = workspace.Base
	receipt.Target = &target
	receipt.State = b.LifecycleStateFacts{SessionID: SessionID(p), SessionGeneration: p.Incarnation, SessionAvailable: true, WorkerLive: true, WorktreeID: worktree.ID}
	receipt.Implementation, err = l.envelope(s, State{Process: p})
	if err == nil {
		receipt.Disposition = b.OperationConfirmed
	}
	return receipt, err
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
	wire, _ := json.Marshal(struct{ Scope, Worktree, Branch, Base, Head string }{value.InspectionScope, ref.External.WorktreeID, value.Branch, value.Base, value.Head})
	value.ContentDigest = workerreturn.Digest(wire)
	return value, nil
}
