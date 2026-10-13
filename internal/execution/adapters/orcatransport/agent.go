package orcatransport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	native "github.com/erranliu/SteerLine/internal/execution/adapters/protocol/orca"
	b "github.com/erranliu/SteerLine/internal/execution/backend"
)

// A bare workspace can materialize its sole terminal asynchronously. Selection
// is bounded to this exact new workspace; no terminal is created or replaced.
func (l *Lifecycle) soleWorkspaceTerminal(ctx context.Context, worktree, runtime string) (native.Terminal, error) {
	for {
		result, err := l.workspaceTerminalQuery(ctx, "terminal", "list", "--worktree", "id:"+worktree, "--json")
		if err != nil {
			return native.Terminal{}, err
		}
		var listing struct {
			Terminals []native.Terminal
			Truncated bool
		}
		if json.Unmarshal(result.Payload, &listing) != nil || result.RuntimeID != runtime || listing.Truncated {
			return native.Terminal{}, errors.New("original workspace terminal inventory is unverified")
		}
		if len(listing.Terminals) > 1 {
			return native.Terminal{}, errors.New("workspace has more than one terminal; no startup command was sent")
		}
		if len(listing.Terminals) == 1 {
			term := listing.Terminals[0]
			if term.Handle == "" || term.IncarnationID == "" || term.WorktreeID != worktree || term.Orphaned {
				return native.Terminal{}, errors.New("original workspace terminal differs")
			}
			return term, nil
		}
		if err := waitWorkspaceTerminal(ctx); err != nil {
			return native.Terminal{}, fmt.Errorf("original workspace terminal is not available; none was created: %w", err)
		}
	}
}

// The center owns this persisted resource operation separately from business
// input. Reconciliation only observes an accepted launch, never resends it.
func (l *Lifecycle) launchAgent(ctx context.Context, r b.LifecycleRequest, reconcile bool) (b.LifecycleReceipt, error) {
	receipt := b.LifecycleReceipt{Operation: r.Operation, Disposition: b.OperationUnknown}
	if l.setup.AgentLaunchCommand == "" || r.Input.Resource == nil || r.Input.Resource.Purpose != "agent-start" || r.Input.Resource.Objective != l.setup.AgentLaunchCommand {
		return receipt, errors.New("agent launch differs from its frozen command")
	}
	state, err := l.state(r.Snapshot)
	if err != nil {
		return receipt, err
	}
	term, err := l.original(ctx, r.Snapshot)
	if err != nil {
		return receipt, err
	}
	receipt.Process = &state.Process
	receipt.State = b.LifecycleStateFacts{SessionID: SessionID(state.Process), SessionGeneration: state.Process.Incarnation, WorktreeID: state.Process.WorktreeID, ResourceID: state.Process.Terminal, ResourcePending: true}
	if !reconcile {
		if state.LaunchAttempted {
			return receipt, errors.New("original agent launch was already attempted; reconcile only")
		}
		state.LaunchDigest = digest([]byte(l.setup.AgentLaunchCommand))
		state.LaunchRequest = nativeRequest(r.Operation.ID)
		state.LaunchAttempted = true
		receipt.Implementation, err = l.envelope(r.Snapshot, state)
		if err != nil {
			return receipt, err
		}
		result, callErr := l.client.MetaCall(ctx, "terminal", "send", "--terminal", state.Process.Terminal, "--text", l.setup.AgentLaunchCommand, "--enter", "--retry-request", state.LaunchRequest, "--json")
		receipt.Operation.Receipt = append([]byte(nil), result.Payload...)
		var value struct {
			Send struct {
				Handle   string
				Accepted bool
				Prompt   struct {
					RequestID          string
					ProcessIncarnation string
				}
			}
		}
		if json.Unmarshal(result.Payload, &value) != nil || result.RuntimeID != state.Process.RuntimeID || value.Send.Handle != state.Process.Terminal || !value.Send.Accepted || value.Send.Prompt.RequestID != state.LaunchRequest || value.Send.Prompt.ProcessIncarnation != state.Process.Incarnation || result.CallUncertain || result.ErrorCode != "" {
			return receipt, errors.Join(callErr, errors.New("original agent launch outcome is unknown; do not resend"))
		}
		state.LaunchAccepted = true
		term, err = l.original(ctx, r.Snapshot)
		if err != nil {
			receipt.Implementation, _ = l.envelope(r.Snapshot, state)
			return receipt, err
		}
	}
	if state.LaunchAccepted && term.AgentIdentity == r.Snapshot.Ref.Agent.Name {
		state.LaunchConfirmed = true
		receipt.Disposition = b.OperationConfirmed
		receipt.State.ResourcePending = false
		receipt.State.SessionAvailable, receipt.State.WorkerLive = true, true
	} else if state.LaunchAccepted {
		receipt.Disposition = b.OperationAccepted
		receipt.Diagnostic = "Original startup command accepted; waiting for the original native agent identity"
	} else {
		receipt.Diagnostic = "Original startup acceptance is unverified; no command or terminal was recreated"
	}
	receipt.RequestID = state.LaunchRequest
	receipt.Implementation, err = l.envelope(r.Snapshot, state)
	return receipt, err
}
