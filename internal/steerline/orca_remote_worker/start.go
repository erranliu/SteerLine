package orca_remote_worker

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

func (b Client) StartTask(ctx context.Context, runID, taskID, worktreeID string, previous *Identity) (StartReceipt, error) {
	pair, err := Preflight(ctx, b.config)
	if err != nil {
		return StartReceipt{}, err
	}
	if runID == "" || taskID == "" || !strings.HasPrefix(worktreeID, pair.RepositoryID+"::") {
		return StartReceipt{}, &Error{Code: BackendIdentityMismatch, Diagnostic: "remote start lacks fixed Run/Task/worktree"}
	}
	args := []string{"orchestration", "worker-start", "--task", taskID, "--on", b.config.Settings.EnvironmentID, "--worktree", "id:" + worktreeID, "--run", runID, "--from", b.config.Settings.CoordinatorTerminal, "--json"}
	reuse := previous != nil
	if reuse {
		if previous.Kind != "orca_supervised" || previous.RuntimeID != pair.RemoteRuntime || previous.CoordinatorRuntimeID != pair.CenterRuntime || previous.ExecutionHost != pair.EnvironmentID || previous.WorktreeID != worktreeID || previous.Terminal == "" || previous.Incarnation == "" {
			return StartReceipt{}, &Error{Code: BackendIdentityMismatch, Diagnostic: "remote continuation changed original agent binding"}
		}
		if _, err := b.Settlement(ctx, Ref{Backend: BackendOrca, Protocol: ProtocolOrcaSupervised, Target: Target{ID: "orca-supervised:" + b.config.Settings.ConnectionID, ConnectionID: b.config.Settings.ConnectionID, ExternalRuntime: pair.EnvironmentID, OrchestrationRun: previous.OrchestrationRun, ExternalTask: previous.Task, Dispatch: previous.Dispatch, ExecutionHost: previous.ExecutionHost, WorktreeID: worktreeID}, External: *previous}, ""); err != nil {
			return StartReceipt{}, err
		}
		args = append(args, "--terminal", previous.Terminal)
	} else {
		args = append(args, "--agent", "codex", "--model", b.config.Model, "--effort", b.config.ReasoningEffort)
	}
	return b.start(ctx, args, pair, runID, worktreeID, taskID, reuse)
}

func (b Client) start(ctx context.Context, args []string, pair PreflightResult, workflowRunID, expectedWorktreeID, expectedTask string, reuse bool) (StartReceipt, error) {
	// Orca may spend its setup window waiting for the agent. Keep the CLI
	// alive long enough to receive that identity on a healthy connection.
	out, callErr := b.config.Call(ctx, max(positiveDuration(b.config.CommandTimeout, 30*time.Second), 2*time.Minute), args...)
	env, parseErr := parseEnvelope(out)
	if callErr != nil || parseErr != nil {
		return StartReceipt{}, &Error{Code: BackendOutcomeUnknown, Diagnostic: "supervised worker start may have been applied; inspect the exact Run and do not issue another start without a platform request UUID", Cause: errors.Join(callErr, parseErr)}
	}
	var result struct {
		RunID      string `json:"runId"`
		TaskID     string `json:"taskId"`
		DispatchID string `json:"dispatchId"`
		State      string `json:"state"`
		Stage      string `json:"stage"`
		Server     struct {
			EnvironmentID string `json:"environmentId"`
		} `json:"server"`
		Worker struct {
			WorktreeID string `json:"worktreeId"`
			Effects    []struct {
				Kind   string `json:"kind"`
				Action string `json:"action"`
				ID     string `json:"id"`
			} `json:"effects"`
		} `json:"worker"`
		Launch struct {
			Effective struct {
				Agent  string `json:"agent"`
				Model  string `json:"model"`
				Effort string `json:"effort"`
			} `json:"effective"`
		} `json:"launch"`
		Effects []struct {
			Kind   string `json:"kind"`
			Action string `json:"action"`
			ID     string `json:"id"`
		} `json:"effects"`
		Mutation struct {
			RequestID string `json:"requestId"`
		} `json:"mutation"`
	}
	if err := json.Unmarshal(env.Result, &result); err != nil {
		return StartReceipt{}, &Error{Code: BackendOutcomeUnknown, Diagnostic: "supervised worker start receipt is malformed", Cause: err}
	}
	startReceipt := StartReceipt{RunID: result.RunID, TaskID: result.TaskID, DispatchID: result.DispatchID, WorktreeID: result.Worker.WorktreeID, CenterRuntimeID: pair.CenterRuntime, PlatformRequestID: result.Mutation.RequestID, State: result.State, Stage: result.Stage}
	if result.RunID != workflowRunID || result.TaskID == "" || result.DispatchID == "" || result.Server.EnvironmentID != b.config.Settings.EnvironmentID || (!reuse && (result.Launch.Effective.Agent != "codex" || result.Launch.Effective.Model != b.config.Model || result.Launch.Effective.Effort != b.config.ReasoningEffort)) || (expectedTask != "" && result.TaskID != expectedTask) {
		return startReceipt, &Error{Code: BackendOutcomeUnknown, Diagnostic: "supervised worker start receipt does not match the fixed Run/environment/launch identity"}
	}
	if result.Mutation.RequestID == "" {
		return startReceipt, &Error{Code: BackendOutcomeUnknown, Diagnostic: "worker-start receipt omitted Orca's platform request UUID; preserve the exact Task/Dispatch and do not retry with a generated ID"}
	}
	if env.Meta.RuntimeID != pair.CenterRuntime {
		return startReceipt, &Error{Code: BackendIdentityMismatch, Diagnostic: "worker-start receipt came from a different center runtime"}
	}
	worktreeID := result.Worker.WorktreeID
	for _, effect := range append(result.Effects, result.Worker.Effects...) {
		if worktreeID == "" && effect.Kind == "worktree" && (effect.Action == "created" || effect.Action == "created_top_level" || effect.Action == "reused") {
			worktreeID = effect.ID
		}
	}
	if worktreeID == "" {
		return startReceipt, &Error{Code: BackendOutcomeUnknown, Diagnostic: "supervised worker start omitted exact worktree identity"}
	}
	startReceipt.WorktreeID = worktreeID
	if worktreeID != expectedWorktreeID {
		return startReceipt, &Error{Code: BackendIdentityMismatch, Diagnostic: "remote worker started in a different worktree"}
	}

	if result.State != "ready" || result.Stage != "input_accepted" {
		return StartReceipt{RunID: result.RunID, TaskID: result.TaskID, DispatchID: result.DispatchID, WorktreeID: worktreeID, PlatformRequestID: result.Mutation.RequestID, State: result.State, Stage: result.Stage}, &Error{Code: BackendOutcomeUnknown, Diagnostic: "supervised worker did not reach input_accepted; reconcile exact Dispatch before retry"}
	}
	return StartReceipt{RunID: result.RunID, TaskID: result.TaskID, DispatchID: result.DispatchID, WorktreeID: worktreeID, CenterRuntimeID: pair.CenterRuntime, PlatformRequestID: result.Mutation.RequestID, State: result.State, Stage: result.Stage}, nil
}
