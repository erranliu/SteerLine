package orca_remote_worker

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func (b Client) FindRun(ctx context.Context, objective, centerRuntime, coordinator string) (string, error) {
	out, callErr := b.config.Call(ctx, positiveDuration(b.config.CommandTimeout, 30*time.Second), "orchestration", "run-current", "--from", coordinator, "--json")
	env, parseErr := parseEnvelope(out)
	if callErr != nil || parseErr != nil {
		return "", errors.Join(callErr, parseErr)
	}
	if env.Meta.RuntimeID != centerRuntime {
		return "", errors.New("coordinator runtime changed while reconciling run-create")
	}
	var result struct {
		Run struct {
			ID        string `json:"id"`
			Objective string `json:"objective"`
		} `json:"run"`
	}
	if err := json.Unmarshal(env.Result, &result); err != nil || result.Run.ID == "" || result.Run.Objective != objective {
		return "", errors.New("current coordinator Run does not uniquely match the fixed objective")
	}
	return result.Run.ID, nil
}

func (b Client) CreateCoordinator(ctx context.Context, workflowRunID, centerRuntime string) (string, error) {
	out, callErr := b.config.Call(ctx, positiveDuration(b.config.CommandTimeout, 30*time.Second), "terminal", "create", "--worktree", "path:"+b.config.Repository, "--shell", b.config.CoordinatorShell, "--title", "SteerLine coordinator "+workflowRunID, "--json")
	env, parseErr := parseEnvelope(out)
	if callErr != nil || parseErr != nil {
		return "", &Error{Code: executionbackend.BackendOutcomeUnknown, Diagnostic: "per-Run coordinator creation is uncertain; inspect the original WorkflowRun before retry", Cause: errors.Join(callErr, parseErr)}
	}
	var result struct {
		Terminal struct {
			Handle string `json:"handle"`
		} `json:"terminal"`
	}
	if err := json.Unmarshal(env.Result, &result); err != nil || env.Meta.RuntimeID != centerRuntime || result.Terminal.Handle == "" {
		return "", &Error{Code: executionbackend.BackendOutcomeUnknown, Diagnostic: "per-Run coordinator receipt lacks an exact handle or center runtime", Cause: err}
	}
	return result.Terminal.Handle, nil
}

func (b Client) VerifyCoordinator(ctx context.Context, ref Ref) (string, error) {
	coordinator := ref.Target.CoordinatorTerminal
	if coordinator == "" {
		coordinator = b.config.Settings.CoordinatorTerminal
	}
	if !stableExecutionID(ref.RunID) || ref.External.OrchestrationRun == "" || ref.External.CoordinatorRuntimeID == "" || coordinator == "" {
		return "", &Error{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "supervised Run lacks its pinned coordinator binding"}
	}
	client := Client{config: b.config}
	out, callErr := client.config.Call(ctx, positiveDuration(b.config.CommandTimeout, 30*time.Second), "orchestration", "run-show", "--id", ref.External.OrchestrationRun, "--json")
	env, parseErr := parseEnvelope(out)
	if callErr != nil || parseErr != nil {
		return "", &Error{Code: executionbackend.BackendUnreachable, Diagnostic: "original Orca Run cannot be inspected", Cause: errors.Join(callErr, parseErr)}
	}
	var result struct {
		Run struct {
			ID          string `json:"id"`
			Coordinator string `json:"coordinator_handle"`
		} `json:"run"`
	}
	if err := json.Unmarshal(env.Result, &result); err != nil || env.Meta.RuntimeID != ref.External.CoordinatorRuntimeID || result.Run.ID != ref.External.OrchestrationRun || result.Run.Coordinator != coordinator {
		return "", &Error{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "Orca Run is bound to another coordinator", Cause: err}
	}
	return coordinator, nil
}
