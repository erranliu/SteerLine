package orca_remote_worker

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func (b Client) Show(ctx context.Context, expected StartReceipt) (WorkerObservation, error) {
	if expected.RunID == "" || expected.TaskID == "" || expected.DispatchID == "" || expected.WorktreeID == "" {
		return WorkerObservation{}, &Error{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "worker inspection requires exact Run/Task/Dispatch/worktree identity"}
	}
	out, callErr := b.config.Call(ctx, positiveDuration(b.config.CommandTimeout, 30*time.Second), "orchestration", "worker-show", "--dispatch", expected.DispatchID, "--json")
	env, parseErr := parseEnvelope(out)
	if callErr != nil || parseErr != nil {
		return WorkerObservation{}, &Error{Code: executionbackend.BackendUnreachable, Diagnostic: "exact supervised Dispatch inspection failed", Cause: errors.Join(callErr, parseErr)}
	}
	var result struct {
		Dispatch struct {
			ID     string `json:"id"`
			RunID  string `json:"runId"`
			TaskID string `json:"taskId"`
			Status string `json:"status"`
		} `json:"dispatch"`
		Worker struct {
			DispatchID string `json:"dispatchId"`
			WorktreeID string `json:"worktreeId"`
		} `json:"worker"`
		Projection struct {
			Liveness struct {
				Verdict string `json:"verdict"`
			} `json:"liveness"`
			Host struct {
				ID string `json:"id"`
			} `json:"host"`
		} `json:"projection"`
		Server struct {
			EnvironmentID string `json:"environmentId"`
		} `json:"server"`
		RemoteRuntimeEpoch string `json:"remoteRuntimeEpoch"`
		Terminal           struct {
			Handle        string          `json:"handle"`
			IncarnationID string          `json:"incarnationId"`
			WorktreeID    string          `json:"worktreeId"`
			WorktreePath  string          `json:"worktreePath"`
			Branch        string          `json:"branch"`
			AgentWait     json.RawMessage `json:"agentWait"`
		} `json:"terminal"`
		Observation struct {
			ExactWorker bool            `json:"exactWorker"`
			AgentWait   json.RawMessage `json:"agentWait"`
		} `json:"observation"`
	}
	if err := json.Unmarshal(env.Result, &result); err != nil {
		return WorkerObservation{}, &Error{Code: executionbackend.BackendProtocolInvalid, Diagnostic: "supervised worker-show response is malformed", Cause: err}
	}
	if env.Meta.RuntimeID != expected.CenterRuntimeID || result.Dispatch.ID != expected.DispatchID || result.Dispatch.RunID != expected.RunID || result.Dispatch.TaskID != expected.TaskID || result.Worker.DispatchID != expected.DispatchID || result.Worker.WorktreeID != expected.WorktreeID || result.Terminal.WorktreeID != expected.WorktreeID || result.Server.EnvironmentID != b.config.Settings.EnvironmentID || result.Projection.Host.ID != b.config.Settings.EnvironmentID || result.RemoteRuntimeEpoch == "" || (b.config.ExpectedRemoteRuntime != "" && result.RemoteRuntimeEpoch != b.config.ExpectedRemoteRuntime) || !result.Observation.ExactWorker {
		return WorkerObservation{}, &Error{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "worker-show does not match the fixed Run/Task/Dispatch/host/worktree identity"}
	}
	if result.Terminal.Handle == "" || result.Terminal.IncarnationID == "" || result.Terminal.WorktreePath == "" || result.Terminal.Branch == "" {
		return WorkerObservation{}, &Error{Code: executionbackend.BackendProtocolInvalid, Diagnostic: "supervised agent terminal or workspace identity is incomplete"}
	}
	identity := executionbackend.ProcessIdentity{Kind: "orca_supervised", RuntimeID: result.RemoteRuntimeEpoch, CoordinatorRuntimeID: expected.CenterRuntimeID, Terminal: result.Terminal.Handle, Incarnation: result.Terminal.IncarnationID, OrchestrationRun: expected.RunID, Task: expected.TaskID, Dispatch: expected.DispatchID, ExecutionHost: result.Projection.Host.ID, WorktreeID: expected.WorktreeID}
	wait := result.Observation.AgentWait
	if len(wait) == 0 {
		wait = result.Terminal.AgentWait
	}
	waitState := "unknown"
	if len(wait) > 0 {
		waitState = "present"
		if string(wait) == "null" {
			waitState = "none"
		}
	}
	return WorkerObservation{Identity: identity, Liveness: result.Projection.Liveness.Verdict, DispatchState: result.Dispatch.Status, AgentWaitState: waitState, WorktreePath: result.Terminal.WorktreePath, Branch: strings.TrimPrefix(result.Terminal.Branch, "refs/heads/"), ObservedAt: now()}, nil
}

func (b Client) Settlement(ctx context.Context, ref Ref, outcome string) (string, error) {
	if err := validateSupervisedRef(b.config, ref); err != nil {
		return "", err
	}
	out, callErr := b.config.Call(ctx, positiveDuration(b.config.CommandTimeout, 30*time.Second), "orchestration", "worker-show", "--dispatch", ref.External.Dispatch, "--json")
	env, parseErr := parseEnvelope(out)
	if callErr != nil || parseErr != nil {
		return "", &Error{Code: executionbackend.BackendUnreachable, Diagnostic: "settled supervised Dispatch inspection failed", Cause: errors.Join(callErr, parseErr)}
	}
	var result struct {
		Dispatch struct {
			ID, RunID, TaskID, Status string
		} `json:"dispatch"`
		Worker struct {
			DispatchID, WorktreeID, AgentTerminalHandle, State string
		} `json:"worker"`
		Projection struct {
			Host struct {
				ID string `json:"id"`
			} `json:"host"`
		} `json:"projection"`
		Server struct {
			EnvironmentID string `json:"environmentId"`
		} `json:"server"`
		RemoteRuntimeEpoch string `json:"remoteRuntimeEpoch"`
	}
	if err := json.Unmarshal(env.Result, &result); err != nil {
		return "", &Error{Code: executionbackend.BackendProtocolInvalid, Diagnostic: "settled supervised worker-show response is malformed", Cause: err}
	}
	if env.Meta.RuntimeID != ref.External.CoordinatorRuntimeID || result.Dispatch.ID != ref.External.Dispatch || result.Dispatch.RunID != ref.External.OrchestrationRun || result.Dispatch.TaskID != ref.External.Task || result.Worker.DispatchID != ref.External.Dispatch || result.Worker.WorktreeID != ref.External.WorktreeID || result.Worker.AgentTerminalHandle != ref.External.Terminal || result.Server.EnvironmentID != ref.External.ExecutionHost || result.Projection.Host.ID != ref.External.ExecutionHost || result.RemoteRuntimeEpoch != ref.External.RuntimeID {
		return "", &Error{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "settled Dispatch differs from the original Run, worker, host, or terminal binding"}
	}
	state := ""
	switch {
	case result.Dispatch.Status == "completed" && result.Worker.State == "succeeded":
		state = "succeeded"
	case result.Dispatch.Status == "failed" && result.Worker.State == "failed":
		state = "failed"
	default:
		return "", &Error{Code: executionbackend.BackendOutcomeUnknown, Diagnostic: "original supervised Dispatch has not settled conclusively"}
	}
	if outcome != "" && state != outcome {
		return "", &Error{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "settled Dispatch outcome differs from accepted worker_done"}
	}
	return state, nil
}

func (b Client) InspectWorktree(ctx context.Context, pair PreflightResult, worktreeID, observedPath, observedBranch, base, expectedHead string) (Workspace, error) {
	if worktreeID == "" || !strings.HasPrefix(worktreeID, pair.RepositoryID+"::") || base == "" {
		return Workspace{}, &Error{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "remote workspace inspection lacks exact worktree/repository/base identity"}
	}
	out, callErr := b.config.Call(ctx, positiveDuration(b.config.CommandTimeout, 30*time.Second), "worktree", "show", "--environment", pair.EnvironmentID, "--worktree", "id:"+worktreeID, "--json")
	env, parseErr := parseEnvelope(out)
	if callErr != nil || parseErr != nil {
		return Workspace{}, &Error{Code: executionbackend.BackendUnreachable, Diagnostic: "remote worktree metadata could not be inspected", Cause: errors.Join(callErr, parseErr)}
	}
	if env.Meta.RuntimeID != pair.RemoteRuntime {
		return Workspace{}, &Error{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "remote worktree metadata came from a different runtime epoch"}
	}
	var result struct {
		Worktree struct {
			ID     string `json:"id"`
			RepoID string `json:"repoId"`
			Path   string `json:"path"`
			Head   string `json:"head"`
			Branch string `json:"branch"`
		} `json:"worktree"`
	}
	if err := json.Unmarshal(env.Result, &result); err != nil {
		return Workspace{}, &Error{Code: executionbackend.BackendProtocolInvalid, Diagnostic: "remote worktree metadata is malformed", Cause: err}
	}
	branch := strings.TrimPrefix(result.Worktree.Branch, "refs/heads/")
	if result.Worktree.ID != worktreeID || result.Worktree.RepoID != pair.RepositoryID || result.Worktree.Path == "" || result.Worktree.Path != observedPath || branch == "" || branch != observedBranch || !commitHash.MatchString(result.Worktree.Head) || expectedHead != "" && result.Worktree.Head != expectedHead {
		return Workspace{}, &Error{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "remote worktree metadata differs from the exact assigned repository/path/branch/revision"}
	}
	return Workspace{Repository: b.config.ProjectID, Worktree: result.Worktree.Path, Branch: branch, Base: base, Head: result.Worktree.Head, Dirty: false, CheckedAt: now()}, nil
}

func (b Client) VerifyWorker(ctx context.Context, ref Ref) error {
	if err := validateSupervisedRef(b.config, ref); err != nil {
		return err
	}
	if _, err := b.VerifyCoordinator(ctx, ref); err != nil {
		return err
	}
	show, err := b.Show(ctx, StartReceipt{RunID: ref.External.OrchestrationRun, TaskID: ref.External.Task, DispatchID: ref.External.Dispatch, WorktreeID: ref.External.WorktreeID, CenterRuntimeID: ref.External.CoordinatorRuntimeID})
	if err != nil {
		return err
	}
	if show.Identity != ref.External || show.Liveness != "live" || show.WorktreePath != ref.WorkspacePath || show.Branch != ref.WorkspaceBranch {
		return &Error{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "remote agent/session/workspace identity changed or is unverified"}
	}
	return nil
}

func (b Client) BindStart(ctx context.Context, started StartReceipt, previous *executionbackend.ProcessIdentity, path, branch string) (executionbackend.ProcessIdentity, error) {
	show, err := b.Show(ctx, started)
	if err != nil {
		return executionbackend.ProcessIdentity{}, err
	}
	if show.Liveness != "live" || show.WorktreePath != path || show.Branch != branch {
		return executionbackend.ProcessIdentity{}, errors.New("remote worker is not live on its fixed workspace/branch")
	}
	if previous != nil && SessionID(*previous) != SessionID(show.Identity) {
		return executionbackend.ProcessIdentity{}, errors.New("remote continuation replaced the original agent session")
	}
	return show.Identity, nil
}
