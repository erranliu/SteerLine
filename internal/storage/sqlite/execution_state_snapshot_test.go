package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

func TestSQLiteApplicationProjectSnapshotSharesTransactionAcrossWriterCommit(t *testing.T) {
	ctx := context.Background()
	writer, project, tasks := nativeAuditStore(t)
	task := tasks[0]
	task.Status, task.Claimant, task.Version = "in_progress", "operator", task.Version+1
	created := time.Now().UTC()
	workflow := executionmodel.WorkflowRunRecord{ProjectIdentity: project, WorkflowRunID: "snapshot-run", TaskID: task.TaskID, WorkflowVersion: "minimal-v1", Status: "running", SelectedResult: "snapshot-execution", CreatedAt: created, UpdatedAt: created}
	execution := executionmodel.ExecutionRecord{ProjectIdentity: project, ExecutionID: "snapshot-execution", CurrentAttemptID: "snapshot-attempt", WorkflowRunID: workflow.WorkflowRunID, Status: "active", CurrentStep: "worker", CreatedAt: created, UpdatedAt: created}
	attempt := executionmodel.StepAttemptRecord{ProjectIdentity: project, AttemptID: "snapshot-attempt", ExecutionID: execution.ExecutionID, StepID: "worker", AttemptNumber: 1, Status: "running", CreatedAt: created, UpdatedAt: created}
	require(t, writer.transactSQLite(ctx, func(tx *sqliteTx) error {
		if err := tx.UpdateNativeTaskCAS(task, task.Version-1); err != nil {
			return err
		}
		if err := tx.PutWorkflowRun(workflow); err != nil {
			return err
		}
		if err := tx.PutExecution(execution); err != nil {
			return err
		}
		return tx.PutStepAttempt(attempt)
	}))
	oldCheckpoint, err := writer.SaveCheckpoint(ctx, storageports.ServiceCheckpoint{
		SchemaVersion: storageports.WorkerFlowRuntimeSchema,
		Service:       storageports.CheckpointServiceState{InstanceID: "old-instance", Status: "running"},
		Repository:    storageports.CheckpointRepository{Identity: project},
	})
	require(t, err)
	var sequence int
	var name, path string
	require(t, writer.db.QueryRowContext(ctx, "PRAGMA database_list").Scan(&sequence, &name, &path))
	reader, err := OpenSQLiteReader(filepath.Clean(path))
	require(t, err)
	defer reader.Close()
	old, err := reader.ReadExecutionStateSnapshot(ctx, project)
	require(t, err)
	if old.Checkpoint == nil || old.Checkpoint.Revision != oldCheckpoint.Revision || old.Graph.Authority.AuthorityVersion != 1 || !old.Graph.Authority.DispatchPaused || old.Records.Executions[0].Status != "active" {
		t.Fatalf("unexpected initial sample: checkpoint=%+v authority=%+v execution=%+v", old.Checkpoint, old.Graph.Authority, old.Records.Executions)
	}
	updatedExecution := execution
	updatedExecution.Status, updatedExecution.CurrentStep, updatedExecution.UpdatedAt = "waiting_decision", "feedback", time.Now().UTC()
	newCheckpoint := oldCheckpoint
	newCheckpoint.Service.InstanceID = "new-instance"
	var committed storageports.WorkerCommitResult
	tx, err := reader.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	require(t, err)
	defer tx.Rollback()
	sampled, err := readApplicationProjectSnapshotTx(ctx, tx, reader.Reader, project, task.TaskID, 500, func() error {
		committed, err = writer.CommitWorker(ctx, storageports.WorkerCommit{
			Project: project, TaskID: task.TaskID, WorkflowID: workflow.WorkflowRunID, ExecutionID: execution.ExecutionID, AttemptID: attempt.AttemptID,
			ExpectedAttemptID: attempt.AttemptID, ControlRevision: old.Records.WorkflowRuns[0].ControlRevision,
			Execution: &storageports.RecordWrite[executionmodel.ExecutionRecord]{Before: &execution, After: updatedExecution}, Checkpoint: newCheckpoint,
		})
		if err != nil {
			return err
		}
		authority, err := writer.NativeAuthority(ctx, project)
		if err != nil {
			return err
		}
		authority.AuthorityVersion++
		authority.DispatchPaused = false
		return writer.transactSQLite(ctx, func(tx *sqliteTx) error {
			return tx.UpdateProjectTaskAuthorityCAS(authority, authority.AuthorityVersion-1)
		})
	})
	require(t, err)
	if committed.Checkpoint.Revision != oldCheckpoint.Revision+1 {
		t.Fatalf("WorkerCommit did not publish the next checkpoint: old=%d new=%d", oldCheckpoint.Revision, committed.Checkpoint.Revision)
	}
	if sampled.ProjectView.Tasks == nil || sampled.ProjectView.Ready == nil || sampled.ProjectView.Tasks.AuthorityVersion != 1 || sampled.ProjectView.Ready.AuthorityVersion != 1 ||
		sampled.ExecutionState.Checkpoint == nil || sampled.ExecutionState.Checkpoint.Revision != oldCheckpoint.Revision || sampled.ExecutionState.Checkpoint.Service.InstanceID != "old-instance" ||
		sampled.ExecutionState.Graph.Authority.AuthorityVersion != 1 || !sampled.ExecutionState.Graph.Authority.DispatchPaused || sampled.ExecutionState.Records.Executions[0].Status != "active" {
		t.Fatalf("writer commit leaked into the combined project sample: product=%+v checkpoint=%+v authority=%+v execution=%+v", sampled.ProjectView.Tasks, sampled.ExecutionState.Checkpoint, sampled.ExecutionState.Graph.Authority, sampled.ExecutionState.Records.Executions)
	}
	require(t, tx.Commit())
	next, err := reader.ReadApplicationProjectSnapshot(ctx, project, task.TaskID, 500)
	require(t, err)
	if next.ProjectView.Tasks == nil || next.ProjectView.Ready == nil || next.ProjectView.Tasks.AuthorityVersion != 2 || next.ProjectView.Ready.AuthorityVersion != 2 ||
		next.ExecutionState.Checkpoint == nil || next.ExecutionState.Checkpoint.Revision != committed.Checkpoint.Revision || next.ExecutionState.Checkpoint.Service.InstanceID != "new-instance" ||
		next.ExecutionState.Graph.Authority.AuthorityVersion != 2 || next.ExecutionState.Graph.Authority.DispatchPaused || next.ExecutionState.Records.Executions[0].Status != "waiting_decision" {
		t.Fatalf("next combined sample did not adopt the complete committed state: product=%+v checkpoint=%+v authority=%+v execution=%+v", next.ProjectView.Tasks, next.ExecutionState.Checkpoint, next.ExecutionState.Graph.Authority, next.ExecutionState.Records.Executions)
	}
}
