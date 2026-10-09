package orca_remote_worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func (b Client) StartTask(ctx context.Context, runID, taskID, worktreeID string, previous *executionbackend.ProcessIdentity) (StartReceipt, error) {
	pair, err := Preflight(ctx, b.config)
	if err != nil {
		return StartReceipt{}, err
	}
	if runID == "" || taskID == "" || !strings.HasPrefix(worktreeID, pair.RepositoryID+"::") {
		return StartReceipt{}, &Error{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "remote start lacks fixed Run/Task/worktree"}
	}
	if b.config.Settings.CoordinatorTerminal == "" {
		return StartReceipt{}, &Error{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "worker start requires the Execution's committed coordinator"}
	}
	args := []string{"orchestration", "worker-start", "--task", taskID, "--worktree", "id:" + worktreeID, "--run", runID, "--from", b.config.Settings.CoordinatorTerminal, "--json"}
	if !b.config.local() {
		args = append(args, "--on", b.config.Settings.EnvironmentID)
	}
	reuse := previous != nil
	if reuse {
		if previous.Kind != "orca_supervised" || previous.RuntimeID != pair.RemoteRuntime || previous.CoordinatorRuntimeID != pair.CenterRuntime || previous.ExecutionHost != pair.EnvironmentID || previous.WorktreeID != worktreeID || previous.Terminal == "" || previous.Incarnation == "" {
			return StartReceipt{}, &Error{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "remote continuation changed original agent binding"}
		}
		original, err := b.Show(ctx, StartReceipt{RunID: previous.OrchestrationRun, TaskID: previous.Task, DispatchID: previous.Dispatch, WorktreeID: previous.WorktreeID, CenterRuntimeID: previous.CoordinatorRuntimeID})
		if err != nil {
			return StartReceipt{}, err
		}
		if original.Identity != *previous || original.Liveness != "live" {
			return StartReceipt{}, &Error{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "remote continuation requires the original live agent incarnation"}
		}
		target := b.config.Target()
		target.OrchestrationRun, target.ExternalTask, target.Dispatch, target.ExecutionHost, target.WorktreeID = previous.OrchestrationRun, previous.Task, previous.Dispatch, previous.ExecutionHost, worktreeID
		if _, err := b.Settlement(ctx, Ref{Backend: executionbackend.BackendOrca, Protocol: executionbackend.ProtocolOrcaSupervised, Target: target, External: *previous}, ""); err != nil {
			return StartReceipt{}, err
		}
		args = append(args, "--terminal", previous.Terminal)
	} else {
		args = append(args, "--agent", b.config.Agent)
		if b.config.Model != "" {
			args = append(args, "--model", b.config.Model)
		}
		if b.config.ReasoningEffort != "" {
			args = append(args, "--effort", b.config.ReasoningEffort)
		}
	}
	return b.start(ctx, args, pair, runID, worktreeID, taskID, reuse)
}

func (b Client) start(ctx context.Context, args []string, pair PreflightResult, workflowRunID, expectedWorktreeID, expectedTask string, reuse bool) (StartReceipt, error) {
	// Orca may spend its setup window waiting for the agent. Keep the CLI
	// alive long enough to receive that identity on a healthy connection.
	out, callErr := b.config.Call(ctx, max(positiveDuration(b.config.CommandTimeout, 30*time.Second), 2*time.Minute), args...)
	env, parseErr := parseEnvelope(out)
	if callErr != nil || parseErr != nil {
		var failure struct {
			State, Stage, FailedStage string
			Mutation                  struct{ RequestID string }
			RunID, TaskID, DispatchID string
			Worker                    struct{ WorktreeID string }
		}
		_ = json.Unmarshal(env.Result, &failure)
		receipt := StartReceipt{RunID: failure.RunID, TaskID: failure.TaskID, DispatchID: failure.DispatchID, WorktreeID: failure.Worker.WorktreeID, CenterRuntimeID: pair.CenterRuntime, PlatformRequestID: failure.Mutation.RequestID, State: failure.State, Stage: failure.Stage}
		return receipt, &Error{Code: executionbackend.BackendOutcomeUnknown, Diagnostic: fmt.Sprintf("supervised worker start may have been applied (state=%s stage=%s failedStage=%s request=%s); inspect the exact Run and do not issue another start", failure.State, failure.Stage, failure.FailedStage, failure.Mutation.RequestID), Cause: errors.Join(callErr, parseErr)}
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
		return StartReceipt{}, &Error{Code: executionbackend.BackendOutcomeUnknown, Diagnostic: "supervised worker start receipt is malformed", Cause: err}
	}
	startReceipt := StartReceipt{RunID: result.RunID, TaskID: result.TaskID, DispatchID: result.DispatchID, WorktreeID: result.Worker.WorktreeID, CenterRuntimeID: pair.CenterRuntime, PlatformRequestID: result.Mutation.RequestID, State: result.State, Stage: result.Stage}
	if result.RunID != workflowRunID || result.TaskID == "" || result.DispatchID == "" || result.Server.EnvironmentID != b.config.Settings.EnvironmentID || (!reuse && (result.Launch.Effective.Agent != b.config.Agent || (b.config.Model != "" && result.Launch.Effective.Model != b.config.Model) || (b.config.ReasoningEffort != "" && result.Launch.Effective.Effort != b.config.ReasoningEffort))) || (expectedTask != "" && result.TaskID != expectedTask) {
		return startReceipt, &Error{Code: executionbackend.BackendOutcomeUnknown, Diagnostic: "supervised worker start receipt does not match the fixed Run/environment/launch identity"}
	}
	if result.Mutation.RequestID == "" {
		return startReceipt, &Error{Code: executionbackend.BackendOutcomeUnknown, Diagnostic: "worker-start receipt omitted Orca's platform request UUID; preserve the exact Task/Dispatch and do not retry with a generated ID"}
	}
	if env.Meta.RuntimeID != pair.CenterRuntime {
		return startReceipt, &Error{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "worker-start receipt came from a different center runtime"}
	}
	worktreeID := result.Worker.WorktreeID
	for _, effect := range append(result.Effects, result.Worker.Effects...) {
		if worktreeID == "" && effect.Kind == "worktree" && (effect.Action == "created" || effect.Action == "created_top_level" || effect.Action == "reused") {
			worktreeID = effect.ID
		}
	}
	if worktreeID == "" {
		return startReceipt, &Error{Code: executionbackend.BackendOutcomeUnknown, Diagnostic: "supervised worker start omitted exact worktree identity"}
	}
	startReceipt.WorktreeID = worktreeID
	if worktreeID != expectedWorktreeID {
		return startReceipt, &Error{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "remote worker started in a different worktree"}
	}

	if result.State != "ready" || result.Stage != "input_accepted" {
		return StartReceipt{RunID: result.RunID, TaskID: result.TaskID, DispatchID: result.DispatchID, WorktreeID: worktreeID, PlatformRequestID: result.Mutation.RequestID, State: result.State, Stage: result.Stage}, &Error{Code: executionbackend.BackendOutcomeUnknown, Diagnostic: "supervised worker did not reach input_accepted; reconcile exact Dispatch before retry"}
	}
	return StartReceipt{RunID: result.RunID, TaskID: result.TaskID, DispatchID: result.DispatchID, WorktreeID: worktreeID, CenterRuntimeID: pair.CenterRuntime, PlatformRequestID: result.Mutation.RequestID, State: result.State, Stage: result.Stage}, nil
}
