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

// ErrAgentStarting is a live original terminal whose agent detection is pending.
// It never authorizes a replacement terminal or a new launch.
var ErrAgentStarting = errors.New("fixed Orca agent is still initializing")

// Orca may rename an agent terminal after it starts. The creation receipt is
// the immutable link between the launch intent and the live terminal.
func FixedAgentTerminal(terminals []Terminal, savedReceipt []byte, title, worktreeID string) (Terminal, error) {
	var expected Terminal
	if len(savedReceipt) != 0 {
		terminal, err := SavedTerminal(savedReceipt)
		if err != nil {
			return Terminal{}, &executionapi.BackendError{Code: executionapi.BackendOutcomeUnknown, Diagnostic: "fixed Orca agent launch receipt is invalid", Cause: err}
		}
		expected = terminal
		if expected.Handle == "" || expected.IncarnationID == "" || expected.WorktreeID != worktreeID || expected.Title != title {
			return Terminal{}, &executionapi.BackendError{Code: executionapi.BackendOutcomeUnknown, Diagnostic: "fixed Orca agent launch receipt does not match the original intent"}
		}
	}
	var matched Terminal
	for _, terminal := range terminals {
		if expected.Handle != "" && terminal.Handle != expected.Handle {
			continue
		}
		if expected.Handle == "" && terminal.Title != title {
			continue
		}
		if terminal.Handle == "" || matched.Handle != "" || terminal.WorktreeID != worktreeID {
			return Terminal{}, &executionapi.BackendError{Code: executionapi.BackendOutcomeUnknown, Diagnostic: "fixed Orca agent terminal is not unique"}
		}
		if expected.Handle != "" && terminal.IncarnationID != expected.IncarnationID {
			return Terminal{}, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "fixed Orca agent terminal incarnation changed"}
		}
		matched = terminal
	}
	return matched, nil
}

func ValidateWorkspaceTerminal(term Terminal, path, branch string) error {
	if path == "" || branch == "" || term.Handle == "" || term.RuntimeID == "" || term.IncarnationID == "" || !samePath(filepath.FromSlash(term.WorktreePath), path) || strings.TrimPrefix(term.Branch, "refs/heads/") != branch {
		return &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "Orca terminal changed the fixed worker workspace or branch"}
	}
	return nil
}
func SessionID(p executionapi.ProcessIdentity) string {
	return "orca-terminal:" + p.RuntimeID + ":" + p.Terminal + ":" + p.Incarnation
}
func TerminalProcess(term Terminal, executable string) executionapi.ProcessIdentity {
	return executionapi.ProcessIdentity{Kind: "orca", Terminal: term.Handle, RuntimeID: term.RuntimeID, Incarnation: term.IncarnationID, Executable: executable}
}
func VerifyAgentTerminal(ctx context.Context, call MetaCall, p executionapi.ProcessIdentity, path, branch, agent string) error {
	term, err := ShowTerminal(ctx, call, p.Terminal)
	if err != nil {
		return err
	}
	if err := ValidateWorkspaceTerminal(term, path, branch); err != nil {
		return err
	}
	if term.Handle != p.Terminal || term.RuntimeID != p.RuntimeID || term.IncarnationID != p.Incarnation || term.WorktreeID != p.WorktreeID {
		return &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "original Orca agent terminal or agent has changed"}
	}
	if term.AgentIdentity == "" && !term.Exited() && term.Connected && !term.Orphaned {
		return ErrAgentStarting
	}
	if term.AgentIdentity != agent {
		return &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "original Orca agent terminal or agent has changed"}
	}
	return nil
}

func VerifyCoordinatorTerminal(ctx context.Context, call MetaCall, p executionapi.ProcessIdentity, repository string) error {
	term, err := ShowTerminal(ctx, call, p.Terminal)
	if err != nil {
		return err
	}
	if term.RuntimeID != p.RuntimeID || term.IncarnationID != p.Incarnation || term.AgentIdentity != "" || !samePath(filepath.FromSlash(term.WorktreePath), repository) {
		return &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "original Orca coordinator changed"}
	}
	return nil
}

// AwaitAgentTerminal only observes; it never replaces or creates a terminal.
func AwaitAgentTerminal(ctx context.Context, call MetaCall, handle, path, branch, worktree, agent string, timeout time.Duration, wait bool) error {
	deadline := time.Now().Add(timeout)
	for {
		term, err := ShowTerminal(ctx, call, handle)
		if err == nil {
			err = ValidateWorkspaceTerminal(term, path, branch)
		}
		if err == nil && (term.WorktreeID != worktree || term.AgentIdentity != agent) {
			err = &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "Orca started a different worktree or agent"}
		}
		if err == nil {
			return nil
		}
		if !wait || time.Now().After(deadline) {
			return fmt.Errorf("fixed Orca agent did not become identifiable: %w", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}
func BindCoordinator(term Terminal, repository, runtime, executable string) (executionapi.ProcessIdentity, error) {
	if !samePath(filepath.FromSlash(term.WorktreePath), repository) || term.RuntimeID != runtime || term.AgentIdentity != "" {
		return executionapi.ProcessIdentity{}, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "Orca coordinator is not the fixed shell on the local runtime/repository"}
	}
	return executionapi.ProcessIdentity{Kind: "orca-native-coordinator", Terminal: term.Handle, RuntimeID: term.RuntimeID, Incarnation: term.IncarnationID, Executable: executable}, nil
}
func InspectCleanupTerminal(ctx context.Context, call MetaCall, identity executionapi.ProcessIdentity, repository, role string, uncertain bool) (bool, error) {
	term, err := ShowTerminal(ctx, call, identity.Terminal)
	if err != nil {
		if uncertain {
			selector := "path:" + filepath.ToSlash(repository)
			if role == "agent" {
				selector = "id:" + identity.WorktreeID
			}
			terms, runtime, listErr := ListTerminals(ctx, call, selector)
			if listErr == nil && runtime == identity.RuntimeID {
				for _, candidate := range terms {
					if candidate.Handle == identity.Terminal {
						return false, err
					}
				}
				return true, nil
			}
		}
		return false, err
	}
	if term.RuntimeID != identity.RuntimeID || term.IncarnationID != identity.Incarnation || role == "agent" && term.WorktreeID != identity.WorktreeID || role == "coordinator" && !samePath(filepath.FromSlash(term.WorktreePath), repository) {
		return false, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "Orca cleanup terminal was replaced or moved"}
	}
	return false, nil
}

func BindAgent(ctx context.Context, call MetaCall, worktree Worktree, handle, agent, executable string) (executionapi.ProcessIdentity, error) {
	if handle == "" {
		terms, runtime, err := ListTerminals(ctx, call, "id:"+worktree.ID)
		if err != nil {
			return executionapi.ProcessIdentity{}, err
		}
		if len(terms) != 1 || terms[0].AgentIdentity != agent || runtime == "" {
			return executionapi.ProcessIdentity{}, &executionapi.BackendError{Code: executionapi.BackendOutcomeUnknown, Diagnostic: "Orca worktree has no unique original agent terminal"}
		}
		handle = terms[0].Handle
	}
	term, err := ShowTerminal(ctx, call, handle)
	if err != nil {
		return executionapi.ProcessIdentity{}, err
	}
	if err := ValidateWorkspaceTerminal(term, worktree.Path, strings.TrimPrefix(worktree.Branch, "refs/heads/")); err != nil {
		return executionapi.ProcessIdentity{}, err
	}
	if term.WorktreeID != worktree.ID {
		return executionapi.ProcessIdentity{}, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "Orca started a different worktree or agent"}
	}
	if term.AgentIdentity == "" {
		return executionapi.ProcessIdentity{}, ErrAgentStarting
	}
	if term.AgentIdentity != agent {
		return executionapi.ProcessIdentity{}, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "Orca started a different worktree or agent"}
	}
	identity := TerminalProcess(term, executable)
	identity.Kind, identity.WorktreeID = "orca-native", worktree.ID
	return identity, nil
}
func FindOriginalCoordinator(ctx context.Context, call MetaCall, repository, runtime, marker string) (string, error) {
	terms, actual, err := ListTerminals(ctx, call, "path:"+filepath.ToSlash(repository))
	if err != nil {
		return "", err
	}
	if actual != runtime {
		return "", &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "Orca coordinator runtime changed"}
	}
	handle := ""
	payload := func(ctx context.Context, args ...string) (json.RawMessage, error) {
		v, e := call(ctx, args...)
		return v.Payload, e
	}
	for _, candidate := range terms {
		if candidate.AgentIdentity != "" {
			continue
		}
		found, err := TerminalHasMarker(ctx, payload, candidate.Handle, marker)
		if err != nil {
			return "", err
		}
		if found {
			if handle != "" {
				return "", &executionapi.BackendError{Code: executionapi.BackendOutcomeUnknown, Diagnostic: "original Orca coordinator is not uniquely identifiable"}
			}
			handle = candidate.Handle
		}
	}
	return handle, nil
}
func VerifyWorktreeOwner(worktree Worktree, executionID, agent string, fixed bool) error {
	expected := agent
	if fixed {
		expected = ""
	}
	if worktree.Comment != "SteerLine execution "+executionID || worktree.CreatedWithAgent != expected {
		return &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "Orca worktree has no matching execution/agent ownership marker"}
	}
	return nil
}
