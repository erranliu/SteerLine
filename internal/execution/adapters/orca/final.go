package orcaworker

import (
	"context"
	"errors"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	"path/filepath"
	"slices"
)

// Consume the same frozen, original-Dispatch response as conversation history.
// The generic native output protocol declares final content; a worker_done
// summary, terminal text or inferred provider phase cannot replace its body.
func (l *ExecutionLifecycle) readNativeFinal(ctx context.Context, snapshot executionbackend.LifecycleSnapshot, turn *NativeTurnState, state *NativeExecutionState) error {
	turn.FinalVerified = false
	if err := ctx.Err(); err != nil {
		return err
	}
	path := filepath.Join(l.setup.RunDirectory, "runs", snapshot.Session.WorkflowRunID, "turns", snapshot.Session.ExecutionID, "orca-output."+turn.TurnID+".json")
	binding, err := readOutputBinding(path)
	if err != nil {
		return err
	}
	if binding.ProjectID != snapshot.Session.ProjectID || binding.WorkflowID != snapshot.Session.WorkflowRunID || binding.ExecutionID != snapshot.Session.ExecutionID || binding.AttemptID != snapshot.Session.AttemptID || binding.TurnID != turn.TurnID || !samePath(binding.Workspace, snapshot.Ref.WorkspacePath) || binding.Command.Executable != l.orca.Executable || !slices.Equal(binding.Command.Args, l.orca.Args) || binding.Process != snapshot.Ref.External || binding.Process.Task != turn.TaskID || binding.Process.Dispatch != turn.DispatchID {
		return errors.New("settled final source differs from the original execution/Command/Task/Dispatch/terminal binding")
	}
	material, err := readSettledOutput(path, binding)
	if err != nil {
		return err
	}
	if material == nil {
		return errors.New("original Dispatch lacks immutable settled final response material")
	}
	turn.FinalSource = outputSource
	turn.FinalItemID = "response:" + settledOutputDigest(*material, material.Outcome)
	turn.FinalNativeTurnID = binding.Process.Dispatch
	turn.FinalText, turn.FinalVerified, turn.FinalReadError = material.Body, true, ""
	if material.Outcome == "failed" {
		turn.Outcome = "failed"
	}
	return nil
}
