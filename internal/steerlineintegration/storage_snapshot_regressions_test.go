package steerlineintegration

import (
	"errors"
	"testing"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/steerlineapp"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func TestTM07PersistedExecutionPreflight(t *testing.T) {
	f := newAppCandidateFixture(t)
	ctx := f.ctx
	claim, _ := f.start(t, "")
	state, err := f.store.ReadExecutionStateSnapshot(ctx, f.f.config.Identity())
	require(t, err)
	var workflow *executionmodel.WorkflowRunRecord
	var persisted executionmodel.ExecutionRecord
	for i := range state.Records.WorkflowRuns {
		if state.Records.WorkflowRuns[i].WorkflowRunID == claim.WorkflowRunID {
			workflow = &state.Records.WorkflowRuns[i]
			break
		}
	}
	for _, record := range state.Records.Executions {
		if record.ExecutionID == claim.ExecutionID {
			persisted = record
			break
		}
	}
	if workflow == nil || workflow.WorkflowVersion != executionmodel.Version || workflow.ExecutionProfile == nil || persisted.ExecutionID == "" {
		t.Fatal("fixture did not create a current-format execution with its frozen profile")
	}
	before := persisted
	persisted.Environment = &executionmodel.WorkerEnvironment{}
	_, err = f.store.CommitWorker(ctx, storageports.WorkerCommit{
		Project: f.f.config.Identity(), TaskID: workflow.TaskID, WorkflowID: workflow.WorkflowRunID,
		ExecutionID: persisted.ExecutionID, AttemptID: persisted.CurrentAttemptID, ExpectedAttemptID: persisted.CurrentAttemptID,
		ControlRevision: workflow.ControlRevision,
		Execution:       &storageports.RecordWrite[executionmodel.ExecutionRecord]{Before: &before, After: persisted},
		Checkpoint: storageports.ServiceCheckpoint{SchemaVersion: storageports.WorkerFlowRuntimeSchema,
			Service:    storageports.CheckpointServiceState{InstanceID: "preflight-fixture", Status: "running"},
			Repository: storageports.CheckpointRepository{Path: f.f.config.Repository, Remote: f.f.config.Remote, Branch: f.f.config.Branch, Identity: f.f.config.Identity()}},
	})
	require(t, err)
	_, beforeRevision, err := f.store.DatabaseVersion(ctx)
	require(t, err)
	service, err := steerlineapp.NewServiceWithLifecycleResolver(f.f.config, &integrationLifecycleResolver{})
	if service != nil {
		_ = service.Close()
	}
	var stateErr *taskmodel.Error
	if !errors.As(err, &stateErr) || stateErr.Code != "unsupported_schema" {
		t.Fatalf("real app construction accepted an execution without preparation intent: %v", err)
	}
	_, afterRevision, err := f.store.DatabaseVersion(ctx)
	require(t, err)
	if afterRevision != beforeRevision {
		t.Fatalf("failed persisted execution preflight wrote to the database: revision %d -> %d", beforeRevision, afterRevision)
	}
}

func TestTM07RuntimeUsesSampledAuthorityAndCheckpoint(t *testing.T) {
	f := newAppCandidateFixture(t)
	claim, _ := f.start(t, "")
	checkpoint, err := f.store.SaveCheckpoint(f.ctx, storageports.ServiceCheckpoint{
		SchemaVersion: storageports.WorkerFlowRuntimeSchema,
		Service:       storageports.CheckpointServiceState{InstanceID: "sampled-runtime-instance", Status: "running", Ready: true},
		Repository:    storageports.CheckpointRepository{Path: f.f.config.Repository, Remote: f.f.config.Remote, Branch: f.f.config.Branch, Identity: f.f.config.Identity()},
	})
	require(t, err)
	state, err := f.store.ReadExecutionStateSnapshot(f.ctx, f.f.config.Identity())
	require(t, err)
	runtimeState, err := steerlineapp.LoadDatabaseRuntime(f.f.config)
	require(t, err)
	if state.Checkpoint == nil || state.Checkpoint.Revision != checkpoint.Revision || runtimeState.Revision != checkpoint.Revision || runtimeState.Service.InstanceID != checkpoint.Service.InstanceID {
		t.Fatalf("runtime did not use the checkpoint sampled with execution facts: checkpoint=%+v runtime=%+v", checkpoint, runtimeState.Service)
	}
	if runtimeState.Control.DispatchMode != state.Graph.Authority.DispatchMode || runtimeState.Control.DispatchPaused != state.Graph.Authority.DispatchPaused || runtimeState.Control.Revision != state.Graph.Authority.AuthorityVersion {
		t.Fatalf("runtime authority differs from the sampled task graph: sampled=%+v runtime=%+v", state.Graph.Authority, runtimeState.Control)
	}
	if len(runtimeState.WorkflowRuns) != 1 || runtimeState.WorkflowRuns[0].WorkflowRunID != claim.WorkflowRunID || len(runtimeState.Executions) != 1 || runtimeState.Executions[0].ExecutionID != claim.ExecutionID {
		t.Fatalf("app runtime did not use the same execution state sample: workflows=%+v executions=%+v", runtimeState.WorkflowRuns, runtimeState.Executions)
	}
}
