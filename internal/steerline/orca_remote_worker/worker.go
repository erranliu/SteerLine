package orca_remote_worker

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
)

// CenterCall never routes the coordinator inbox or lifecycle to the peer.
func (b Client) CenterCall(ctx context.Context, args ...string) (orcaworker.Result, error) {
	out, callErr := b.config.Call(ctx, positiveDuration(b.config.CommandTimeout, 30*time.Second), args...)
	env, parseErr := parseEnvelope(out)
	result := orcaworker.Result{Payload: env.Result, RuntimeID: env.Meta.RuntimeID}
	if env.Error != nil {
		result.ErrorCode, result.ErrorData = env.Error.Code, env.Error.Data
	}
	return result, errors.Join(callErr, parseErr)
}

func (b Client) centerPayload(runtimeID string) orcaworker.Call {
	return func(ctx context.Context, args ...string) (json.RawMessage, error) {
		env, err := b.CenterCall(ctx, args...)
		if err == nil && env.RuntimeID != runtimeID {
			err = &Error{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "center runtime changed"}
		}
		return env.Payload, err
	}
}

func (b Client) FindCoordinator(ctx context.Context, title, runtimeID string) (string, error) {
	raw, err := b.centerPayload(runtimeID)(ctx, "terminal", "list", "--worktree", "path:"+b.config.Repository, "--json")
	if err != nil {
		return "", err
	}
	var listing struct {
		Terminals []struct {
			Handle, Title       string
			Connected, Orphaned bool
		}
	}
	if err := json.Unmarshal(raw, &listing); err != nil {
		return "", err
	}
	found := ""
	for _, term := range listing.Terminals {
		if term.Title == title && term.Connected && !term.Orphaned {
			if found != "" {
				return "", errors.New("remote coordinator creation has multiple matches")
			}
			found = term.Handle
		}
	}
	return found, nil
}

func (b Client) FindTask(ctx context.Context, runID, spec, runtimeID string) (string, error) {
	return b.FindTaskFrom(ctx, runID, b.config.Settings.CoordinatorTerminal, spec, runtimeID)
}

func (b Client) FindTaskFrom(ctx context.Context, runID, coordinator, spec, runtimeID string) (string, error) {
	return orcaworker.FindTask(ctx, b.centerPayload(runtimeID), runID, coordinator, spec)
}

// RecoverStart only reads the original Task. Absence never authorizes a second start.
func (b Client) RecoverStart(ctx context.Context, runID, taskID, worktreeID, runtimeID string) (StartReceipt, error) {
	id, run, _, _, err := orcaworker.FindDispatch(ctx, b.centerPayload(runtimeID), taskID)
	if err != nil {
		return StartReceipt{}, err
	}
	if id == "" || run != runID {
		return StartReceipt{}, &Error{Code: executionbackend.BackendOutcomeUnknown, Diagnostic: "original remote Task has no unique Dispatch"}
	}
	return StartReceipt{RunID: runID, TaskID: taskID, DispatchID: id, WorktreeID: worktreeID, CenterRuntimeID: runtimeID}, nil
}

func SessionID(identity executionbackend.ProcessIdentity) string {
	return "orca-remote:" + identity.ExecutionHost + ":" + identity.RuntimeID + ":" + identity.Terminal + ":" + identity.Incarnation
}

// Remote filesystem metadata is never normalized using the center's OS.
func (b Client) workspaceCall(pair PreflightResult) orcaworker.Call {
	return func(ctx context.Context, args ...string) (json.RawMessage, error) {
		if pair.EnvironmentID != b.config.Settings.EnvironmentID || pair.RemoteRuntime == "" || (b.config.ExpectedRemoteRuntime != "" && pair.RemoteRuntime != b.config.ExpectedRemoteRuntime) {
			return nil, &Error{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "workspace target differs from fixed peer"}
		}
		args = append(args, "--environment", pair.EnvironmentID)
		env, err := b.CenterCall(ctx, args...)
		if err == nil && env.RuntimeID != pair.RemoteRuntime {
			err = &Error{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "remote workspace runtime changed"}
		}
		return env.Payload, err
	}
}

func (b Client) ShowCoordinator(ctx context.Context, handle, runtimeID string) (executionbackend.ProcessIdentity, error) {
	term, err := orcaworker.ShowTerminal(ctx, b.CenterCall, handle)
	if err != nil {
		return executionbackend.ProcessIdentity{}, err
	}
	if term.RuntimeID != runtimeID || term.AgentIdentity != "" {
		return executionbackend.ProcessIdentity{}, &Error{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "center coordinator runtime or owner changed"}
	}
	return executionbackend.ProcessIdentity{Kind: "orca-remote-coordinator", Terminal: term.Handle, RuntimeID: term.RuntimeID, Incarnation: term.IncarnationID}, nil
}

func (b Client) CloseCoordinator(ctx context.Context, ref Ref, original executionbackend.ProcessIdentity, uncertain bool) (json.RawMessage, error) {
	if _, err := b.VerifyCoordinator(ctx, ref); err != nil {
		return nil, err
	}
	shown, err := b.ShowCoordinator(ctx, original.Terminal, original.RuntimeID)
	if err != nil && uncertain {
		raw, listErr := b.centerPayload(original.RuntimeID)(ctx, "terminal", "list", "--worktree", "path:"+b.config.Repository, "--json")
		if listErr != nil {
			return nil, listErr
		}
		var listing struct {
			Terminals []struct{ Handle string }
			Truncated bool
		}
		if listErr := json.Unmarshal(raw, &listing); listErr != nil || listing.Truncated {
			return nil, errors.Join(listErr, errors.New("coordinator inventory incomplete"))
		}
		for _, term := range listing.Terminals {
			if term.Handle == original.Terminal {
				return nil, err
			}
		}
		return json.Marshal(map[string]string{"handle": original.Terminal, "proof": "absent_from_original_center"})
	}
	if err != nil {
		return nil, err
	}
	if shown != original {
		return nil, &Error{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "original coordinator incarnation changed"}
	}
	inbox, err := orcaworker.CheckInbox(ctx, b.CenterCall, original.Terminal, ref.External.OrchestrationRun)
	if err != nil {
		return nil, err
	}
	if inbox.RuntimeID != original.RuntimeID || inbox.RunID != ref.External.OrchestrationRun || inbox.ConnectionLost || inbox.DeliveryID != "" || len(inbox.Messages) > 0 {
		return nil, errors.New("remote coordinator inbox is not proven drained")
	}
	return orcaworker.CloseTerminal(ctx, b.CenterCall, original.Terminal, original.RuntimeID)
}

func validateWorktree(value orcaworker.Worktree, pair PreflightResult, name, base string) error {
	if value.ID != pair.RepositoryID+"::"+value.Path || value.RepoID != pair.RepositoryID || value.DisplayName != name || value.Path == "" || strings.TrimPrefix(value.Branch, "refs/heads/") == "" || value.Head != base || value.BaseRef != base {
		return &Error{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "remote worktree differs from fixed repository/name/base"}
	}
	return nil
}

func (b Client) FindWorktree(ctx context.Context, pair PreflightResult, name, base string) (orcaworker.Worktree, bool, error) {
	raw, err := b.workspaceCall(pair)(ctx, "worktree", "list", "--repo", "id:"+pair.RepositoryID, "--limit", "1000", "--json")
	if err != nil {
		return orcaworker.Worktree{}, false, err
	}
	var listing struct {
		Worktrees []orcaworker.Worktree
		Truncated bool
	}
	if err := json.Unmarshal(raw, &listing); err != nil || listing.Truncated {
		return orcaworker.Worktree{}, false, errors.Join(err, errors.New("remote worktree listing incomplete"))
	}
	var found *orcaworker.Worktree
	for i := range listing.Worktrees {
		if listing.Worktrees[i].DisplayName == name {
			if found != nil {
				return orcaworker.Worktree{}, false, errors.New("remote worktree name is ambiguous")
			}
			found = &listing.Worktrees[i]
		}
	}
	if found == nil {
		return orcaworker.Worktree{}, false, nil
	}
	if err := validateWorktree(*found, pair, name, base); err != nil {
		return orcaworker.Worktree{}, false, err
	}
	return *found, true, nil
}

func (b Client) CreateWorktree(ctx context.Context, pair PreflightResult, name, base string) (orcaworker.Worktree, error) {
	if !commitHash.MatchString(base) || name == "" {
		return orcaworker.Worktree{}, errors.New("remote workspace requires fixed name and commit")
	}
	raw, err := b.workspaceCall(pair)(ctx, "worktree", "create", "--repo", "id:"+pair.RepositoryID, "--name", name, "--base-branch", base, "--no-parent", "--setup", "skip", "--json")
	if err != nil {
		return orcaworker.Worktree{}, &Error{Code: executionbackend.BackendOutcomeUnknown, Diagnostic: "remote worktree creation receipt unavailable", Cause: err}
	}
	var result struct{ Worktree orcaworker.Worktree }
	if err := json.Unmarshal(raw, &result); err != nil {
		return orcaworker.Worktree{}, err
	}
	return result.Worktree, validateWorktree(result.Worktree, pair, name, base)
}

// Release is post-settlement cleanup, never cancellation.
func (b Client) Release(ctx context.Context, ref Ref) (orcaworker.MutationReceipt, error) {
	if _, err := b.Settlement(ctx, ref, ""); err != nil {
		return orcaworker.MutationReceipt{}, err
	}
	env, err := b.CenterCall(ctx, "orchestration", "worker-release", "--dispatch", ref.External.Dispatch, "--json")
	if err != nil {
		return orcaworker.MutationReceipt{}, err
	}
	var result struct {
		DispatchID, State string
		ResidualResources []json.RawMessage
		Mutation          struct{ RequestID string }
	}
	if err := json.Unmarshal(env.Payload, &result); err != nil {
		return orcaworker.MutationReceipt{}, err
	}
	if env.RuntimeID != ref.External.CoordinatorRuntimeID || result.DispatchID != ref.External.Dispatch || (result.State != "released" && result.State != "already_released") || len(result.ResidualResources) > 0 || result.Mutation.RequestID == "" {
		return orcaworker.MutationReceipt{}, &Error{Code: executionbackend.BackendOutcomeUnknown, Diagnostic: "remote release did not confirm exact settled Dispatch"}
	}
	return orcaworker.MutationReceipt{RequestID: result.Mutation.RequestID, Payload: env.Payload}, nil
}

// ReleaseAcknowledged inspects the worker inventory only. A missing or
// uncertain row never authorizes a replacement release request.
func (b Client) ReleaseAcknowledged(ctx context.Context, ref Ref) (bool, error) {
	if err := validateSupervisedRef(b.config, ref); err != nil {
		return false, err
	}
	cursor := ""
	for page := 0; page < 100; page++ {
		args := []string{"orchestration", "worker-list", "--run", ref.External.OrchestrationRun, "--include-remote", "--limit", "100", "--json"}
		if cursor != "" {
			args = append(args, "--cursor", cursor)
		}
		result, err := b.CenterCall(ctx, args...)
		if err != nil {
			return false, err
		}
		if result.RuntimeID != ref.External.CoordinatorRuntimeID {
			return false, &Error{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "worker inventory came from a different coordinator runtime"}
		}
		var listing struct {
			Workers []struct {
				RunID, TaskID, DispatchID, WorktreeID, State, TerminalState string
			} `json:"workers"`
			NextCursor string `json:"nextCursor"`
			Truncated  bool   `json:"truncated"`
		}
		if err := json.Unmarshal(result.Payload, &listing); err != nil || listing.Truncated {
			return false, errors.Join(err, errors.New("worker inventory is incomplete"))
		}
		for _, worker := range listing.Workers {
			if worker.DispatchID != ref.External.Dispatch {
				continue
			}
			if worker.RunID != ref.External.OrchestrationRun || worker.TaskID != ref.External.Task || worker.WorktreeID != ref.External.WorktreeID {
				return false, &Error{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "worker inventory changed the original Run/Task/worktree identity"}
			}
			if worker.State == "released" || worker.TerminalState == "released" {
				return true, nil
			}
			return false, nil
		}
		if listing.NextCursor == "" {
			return false, nil
		}
		if listing.NextCursor == cursor {
			return false, errors.New("worker inventory cursor did not advance")
		}
		cursor = listing.NextCursor
	}
	return false, errors.New("worker inventory exceeded its bounded page count")
}
