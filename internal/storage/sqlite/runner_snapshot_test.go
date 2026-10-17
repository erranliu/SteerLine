package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/product/query"
	"github.com/erranliu/SteerLine/internal/runnerbridge"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func TestRunnerApplicationSnapshotSharesTransactionAcrossWriterCommit(t *testing.T) {
	ctx := context.Background()
	writer, project, tasks := nativeAuditStore(t)
	require(t, writer.transactSQLite(ctx, func(tx *sqliteTx) error {
		_, err := tx.tx.ExecContext(ctx, `UPDATE project_task_authorities SET dispatch_paused=0 WHERE project_identity=?`, project)
		return err
	}))
	graph, err := writer.TaskGraphSnapshot(ctx, project)
	require(t, err)
	contract, err := writer.TaskContract(ctx, project, tasks[0])
	require(t, err)
	base := strings.Repeat("a", 40)
	input := runnerclient.RunInput{Title: tasks[0].Title, Repository: runnerclient.Repository{URL: contract.Repository, Remote: contract.Remote, BaseBranch: contract.Branch, BaseRevision: base}, Documents: runnerclient.Documents{Spec: runnerclient.Document{Kind: "git", Path: contract.SpecRef.Path, Commit: contract.SpecRef.Commit}, Plan: runnerclient.Document{Kind: "git", Path: contract.PlanRef.Path, Commit: contract.PlanRef.Commit}}, Publication: runnerclient.Publication{BaseBranch: contract.Branch}}
	body, _ := json.Marshal(input)
	candidate := runnerbridge.Candidate{Project: project, TaskID: tasks[0].TaskID, ID: "snapshot-candidate", Revision: 1, Explicit: true, StateDir: filepath.Join(t.TempDir(), "runner"), Claimant: "operator", ContractDigest: contract.Digest, BaseRevision: base, Participation: "active", SubmitState: "pending", Submit: runnerclient.Request{Protocol: runnerclient.Protocol, RequestID: "snapshot-submit", Input: body}}
	require(t, writer.ReserveRunnerCandidate(ctx, taskmodel.FixedWork{Record: tasks[0], Contract: contract, Authority: graph.TaskGraph().Authority}, candidate, false))
	oldCheckpoint, err := writer.SaveCheckpoint(ctx, storageports.ServiceCheckpoint{SchemaVersion: storageports.WorkerFlowRuntimeSchema, Service: storageports.CheckpointServiceState{InstanceID: "old-instance", Status: "running"}, Repository: storageports.CheckpointRepository{Identity: project}})
	require(t, err)
	var sequence int
	var name, path string
	require(t, writer.db.QueryRowContext(ctx, "PRAGMA database_list").Scan(&sequence, &name, &path))
	reader, err := OpenSQLiteReader(path)
	require(t, err)
	defer reader.Close()
	old, err := reader.ReadRunnerSnapshot(ctx, project)
	require(t, err)
	if old.Checkpoint == nil || old.Checkpoint.Revision != oldCheckpoint.Revision || len(old.Candidates) != 1 {
		t.Fatal("initial Runner sample unavailable")
	}
	updated := old.Candidates[0]
	updated.Revision++
	updated.RunID = "public-run"
	updated.SubmitState = "accepted"
	updated.Projection = &runnerclient.Run{RunID: "public-run", Revision: 1, Status: "running"}
	tx, err := reader.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	require(t, err)
	defer tx.Rollback()
	sampled, err := readApplicationProjectSnapshotTx(ctx, tx, reader.Reader, project, tasks[0].TaskID, 500, func() error {
		if err := writer.SaveCandidate(ctx, updated, old.Candidates[0].Revision, nil); err != nil {
			return err
		}
		next := oldCheckpoint
		next.Service.InstanceID = "new-instance"
		if _, err := writer.SaveCheckpoint(ctx, next); err != nil {
			return err
		}
		authority, err := writer.NativeAuthority(ctx, project)
		if err != nil {
			return err
		}
		authority.AuthorityVersion++
		authority.DispatchPaused = true
		return writer.transactSQLite(ctx, func(tx *sqliteTx) error {
			return tx.UpdateProjectTaskAuthorityCAS(authority, authority.AuthorityVersion-1)
		})
	})
	require(t, err)
	if sampled.RunnerError != nil || sampled.ProjectView.Tasks == nil || sampled.ProjectView.Tasks.AuthorityVersion != 1 || sampled.RunnerState.Checkpoint == nil || sampled.RunnerState.Checkpoint.Service.InstanceID != "old-instance" || sampled.RunnerState.Graph.Authority.AuthorityVersion != 1 || len(sampled.RunnerState.Candidates) != 1 || sampled.RunnerState.Candidates[0].RunID != "" {
		t.Fatal("writer commit leaked into pinned product/Runner snapshot", sampled.RunnerError)
	}
	require(t, tx.Commit())
	// Projection happens after the read transaction ends and a different writer
	// has committed. History and selected-task candidates must still use only
	// the original sampled facts, without querying the latest Run.
	oldPage := query.ProjectPage(sampled.ProjectView, tasks[0].TaskID, 0, api.ObservationSnapshot{}, api.ServiceConnection{}, nil, nil)
	if oldPage.History == nil || len(oldPage.History.WorkflowRuns) != 1 || oldPage.History.WorkflowRuns[0].SubmissionStatus != "pending" || oldPage.History.WorkflowRuns[0].WorkflowStatus != "pending" || oldPage.SelectedNative == nil || len(oldPage.SelectedNative.WorkflowRuns) != 1 || oldPage.SelectedNative.WorkflowRuns[0].WorkflowStatus != "pending" {
		t.Fatal("post-transaction history/detail projection lost its original sample", oldPage)
	}
	next, err := reader.ReadApplicationProjectSnapshot(ctx, project, tasks[0].TaskID, 500)
	require(t, err)
	if next.RunnerError != nil || next.ProjectView.Tasks.AuthorityVersion != 2 || next.RunnerState.Graph.Authority.AuthorityVersion != 2 || next.RunnerState.Checkpoint.Service.InstanceID != "new-instance" || next.RunnerState.Candidates[0].Projection.RunID != "public-run" {
		t.Fatal("next sample omitted committed Runner facts", next.RunnerError)
	}
	newPage := query.ProjectPage(next.ProjectView, tasks[0].TaskID, 0, api.ObservationSnapshot{}, api.ServiceConnection{}, nil, nil)
	if newPage.History == nil || len(newPage.History.WorkflowRuns) != 1 || newPage.SelectedNative == nil || len(newPage.SelectedNative.WorkflowRuns) != 1 || newPage.History.WorkflowRuns[0].WorkflowStatus != "running" || newPage.SelectedNative.WorkflowRuns[0].WorkflowStatus != "running" {
		t.Fatal("new history/detail projection omitted committed facts", newPage)
	}
}
