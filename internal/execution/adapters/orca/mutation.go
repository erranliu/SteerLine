package orcaworker

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

var ErrAgentHandleOmitted = errors.New("Orca did not return the created agent terminal handle")

var ErrCoordinatorHandleOmitted = errors.New("Orca coordinator creation has no exact handle")

type NativeWorktreeReceipt struct {
	Worktree Worktree
	Handle   string
}

func ParseNativeWorktreeReceipt(envelope Result, name, repoID, base string) (NativeWorktreeReceipt, error) {
	var result struct {
		Worktree            Worktree `json:"worktree"`
		AgentTerminalHandle string   `json:"agentTerminalHandle"`
		StartupTerminal     struct {
			Handle string `json:"handle"`
		} `json:"startupTerminal"`
	}
	if err := json.Unmarshal(envelope.Payload, &result); err != nil {
		return NativeWorktreeReceipt{}, err
	}
	if err := ValidateWorktree(result.Worktree, name, repoID, base); err != nil {
		return NativeWorktreeReceipt{}, err
	}
	handle := result.AgentTerminalHandle
	if handle == "" {
		handle = result.StartupTerminal.Handle
	}
	if handle == "" {
		return NativeWorktreeReceipt{}, ErrAgentHandleOmitted
	}
	return NativeWorktreeReceipt{Worktree: result.Worktree, Handle: handle}, nil
}

func ParseFixedWorktreeReceipt(envelope Result, name, repoID, base string) (Worktree, error) {
	var result struct {
		Worktree Worktree `json:"worktree"`
	}
	if err := json.Unmarshal(envelope.Payload, &result); err != nil {
		return Worktree{}, err
	}
	if err := ValidateWorktree(result.Worktree, name, repoID, base); err != nil || result.Worktree.CreatedWithAgent != "" {
		return Worktree{}, errors.New("fixed Orca worktree receipt is incomplete")
	}
	return result.Worktree, nil
}

func ParseCoordinatorTerminalReceipt(envelope Result) (string, error) {
	var result struct {
		Terminal Terminal `json:"terminal"`
	}
	if err := json.Unmarshal(envelope.Payload, &result); err != nil || result.Terminal.Handle == "" {
		return "", ErrCoordinatorHandleOmitted
	}
	return result.Terminal.Handle, nil
}

func ParseFixedAgentTerminalReceipt(envelope Result, worktreeID, title string) (Terminal, error) {
	var result struct {
		Terminal Terminal `json:"terminal"`
	}
	if err := json.Unmarshal(envelope.Payload, &result); err != nil || result.Terminal.Handle == "" || result.Terminal.IncarnationID == "" || result.Terminal.WorktreeID != worktreeID || result.Terminal.Title != title {
		return Terminal{}, errors.New("fixed Orca agent launch receipt is incomplete")
	}
	return result.Terminal, nil
}

func CreateNativeWorktree(ctx context.Context, call MetaCall, repoID, name, base, agent, comment string) (Result, error) {
	return call(ctx, "worktree", "create", "--repo", "id:"+repoID, "--name", name, "--base-branch", base, "--no-parent", "--setup", "skip", "--agent", agent, "--comment", comment, "--json")
}

func CreateFixedWorktree(ctx context.Context, call MetaCall, repoID, name, base, comment string) (Result, error) {
	return call(ctx, "worktree", "create", "--repo", "id:"+repoID, "--name", name, "--base-branch", base, "--no-parent", "--setup", "skip", "--comment", comment, "--json")
}

func CreateCoordinatorTerminal(ctx context.Context, call MetaCall, repository, title, marker string) (Result, error) {
	return call(ctx, "terminal", "create", "--worktree", "path:"+filepath.ToSlash(repository), "--title", title, "--shell", "cmd.exe", "--command", "echo "+marker, "--json")
}

func CreateFixedAgentTerminal(ctx context.Context, call MetaCall, worktreeID, title, shell, command string) (Result, error) {
	return call(ctx, "terminal", "create", "--worktree", "id:"+worktreeID, "--title", title, "--shell", shell, "--command", command, "--json")
}

func Dispatch(ctx context.Context, call MetaCall, taskID, terminal, runID, coordinator string) (Result, error) {
	return call(ctx, "orchestration", "dispatch", "--task", taskID, "--to", terminal, "--run", runID, "--from", coordinator, "--inject", "--json")
}

func NoAgentDetected(result Result) bool {
	if result.CallUncertain || result.ErrorCode != "inject_rejected" {
		return false
	}
	var rejected struct {
		Reason string `json:"reason"`
	}
	return json.Unmarshal(result.ErrorData, &rejected) == nil && rejected.Reason == "no_agent_detected"
}

func FeedbackDispatchInactive(err error) bool {
	var callErr *CallError
	return errors.As(err, &callErr) && !callErr.Result.CallUncertain && callErr.Result.ErrorCode == "dispatch_inactive"
}

func RecoverDispatch(ctx context.Context, call MetaCall, taskID, runID string, identity executionbackend.ProcessIdentity) (string, error) {
	payload := func(ctx context.Context, args ...string) (json.RawMessage, error) {
		v, e := call(ctx, args...)
		return v.Payload, e
	}
	id, actual, handle, incarnation, err := FindDispatch(ctx, payload, taskID)
	if err != nil {
		return "", &executionbackend.BackendError{Code: executionbackend.BackendOutcomeUnknown, Diagnostic: "original Orca Dispatch creation is unverified", Cause: err}
	}
	if actual != runID || handle != identity.Terminal || !strings.HasSuffix(incarnation, ":"+identity.Incarnation) {
		return "", &executionbackend.BackendError{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "recovered Orca Dispatch targets a different worker or Run"}
	}
	err = VerifyRecoveredDispatch(ctx, call, DispatchIdentity{DispatchID: id, TaskID: taskID, RunID: runID, RuntimeID: identity.RuntimeID, Terminal: identity.Terminal, Incarnation: identity.Incarnation, WorktreeID: identity.WorktreeID})
	if errors.Is(err, ErrDispatchIdentity) {
		err = &executionbackend.BackendError{Code: executionbackend.BackendIdentityMismatch, Diagnostic: err.Error()}
	}
	return id, err
}
