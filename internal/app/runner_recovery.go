package app

import (
	"context"
	"errors"
	"time"

	api "github.com/erranliu/SteerLine/internal/product/api"
)

// Recovery inspection samples public Runner facts without opening its database,
// reconnecting native sessions or interpreting private adapter bindings.
func inspectRunnerRecovery(ctx context.Context, config Config, id string) (api.RecoveryInspection, error) {
	view := api.RecoveryInspection{Version: 1, RunID: id, SampledAt: time.Now().UTC(), ProcessCheck: "owned by SteerRunner", WorkspaceError: "the public Runner contract does not provide a fresh workspace inspection"}
	reader, err := OpenNativeTaskReader(ctx, &config)
	if err != nil {
		return view, err
	}
	defer reader.Close()
	candidate, err := reader.Candidate(ctx, config.Identity(), id)
	if err != nil {
		return view, err
	}
	snapshot, err := reader.ReadRunnerSnapshot(ctx, config.Identity())
	if err != nil {
		return view, err
	}
	view.TaskID, view.RunRevision = candidate.TaskID, candidate.Revision
	view.DispatchMode, view.DispatchPaused = snapshot.Graph.Authority.DispatchMode, snapshot.Graph.Authority.DispatchPaused
	if snapshot.Checkpoint != nil {
		view.ServiceInstanceID = snapshot.Checkpoint.Service.InstanceID
		view.ServiceStatus = snapshot.Checkpoint.Service.Status
	}
	if candidate.RunID == "" {
		view.ContinuationCheck = "original submission is " + candidate.SubmitState
		return view, nil
	}
	cli, err := config.runnerClient(ctx)
	if err != nil {
		return view, err
	}
	inspection, err := cli.Inspect(ctx, "run-id", candidate.RunID, false)
	if err != nil {
		return view, err
	}
	run := inspection.Run
	if run == nil || candidate.InputDigest != "" && run.InputDigest != candidate.InputDigest || candidate.Branch != "" && run.Execution.Branch != candidate.Branch {
		return view, errors.New("Runner recovery response differs from the fixed candidate")
	}
	view.ExecutionStatus, view.WorkflowStatus, view.WorkflowStage = run.Status, run.Status, run.Step
	view.Backend, view.Protocol = run.Execution.Backend, run.Execution.Protocol
	view.SessionCheck = run.Execution.SessionID
	view.ContinuationCheck = run.Controls.Reason
	if run.Controls.CanResume {
		view.ContinuationCheck = "requires a fresh explicit resume to the original Runner Run"
	}
	if run.Question != nil {
		view.DecisionID = run.Question.ID
		view.DecisionStatus = "pending"
	}
	if run.PR != nil {
		view.PR = &api.RecoveryPRSummary{URL: run.PR.URL, State: run.PR.State, Repository: run.PR.Repository, BaseBranch: run.PR.BaseBranch}
		view.PRError = run.PR.QueryError
	}
	return view, nil
}
