package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
)

func requireProjectView(t *testing.T, view ProjectView) {
	t.Helper()
	for _, err := range []error{view.TaskError, view.SelectedTaskError, view.DomainError, view.RuntimeError} {
		require(t, err)
	}
	if view.Tasks == nil || view.Ready == nil || view.Domain == nil {
		t.Fatal("project view omitted a successful query section")
	}
	if view.Tasks.DatabaseID != view.Ready.DatabaseID || view.Tasks.DatabaseRevision != view.Ready.DatabaseRevision ||
		view.Tasks.AuthorityVersion != view.Ready.AuthorityVersion || view.Tasks.GraphRevision != view.Ready.GraphRevision ||
		!view.Tasks.SampledAt.Equal(view.SampledAt) || !view.Ready.SampledAt.Equal(view.SampledAt) {
		t.Fatal("project sections have different sampling versions or times")
	}
}

func TestProjectViewRejectsNilReader(t *testing.T) {
	var store *SQLiteStore
	view, err := NewQueryService(store, "project").ReadProjectView(context.Background(), "", 1)
	if err == nil || view.Tasks != nil || view.Domain != nil || view.Runtime != nil {
		t.Fatal("nil reader exposed a successful project view")
	}
}

func TestProjectViewSharesTransactionAcrossWriterCommit(t *testing.T) {
	ctx := context.Background()
	writer, project, tasks := nativeAuditStore(t)
	checkpoint := ServiceCheckpoint{SchemaVersion: WorkerFlowRuntimeSchema, Repository: Repository{Identity: project}, HeartbeatAt: now(), Service: contracts.ServiceState{InstanceID: "old-instance"}}
	checkpoint, err := writer.SaveCheckpoint(ctx, checkpoint)
	require(t, err)
	var sequence int
	var name, path string
	require(t, writer.db.QueryRowContext(ctx, "PRAGMA database_list").Scan(&sequence, &name, &path))
	reader, err := OpenSQLiteReader(path)
	require(t, err)
	defer reader.Close()
	tx, err := reader.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	require(t, err)
	defer tx.Rollback()
	// Pin the WAL snapshot before a different connection commits new business
	// records and heartbeat metadata. No sleeps or scheduler timing are needed.
	page, err := reader.Reader.NativeTaskPageTx(ctx, tx, project, "", "", 0, 0, 1)
	require(t, err)
	authority, err := writer.NativeAuthority(ctx, project)
	require(t, err)
	authority.AuthorityVersion++
	authority.DispatchPaused = false
	claimed := tasks[2]
	claimed.Status, claimed.Claimant, claimed.Version = "in_progress", "operator", 2
	checkpoint.Service.InstanceID, checkpoint.Revision = "new-instance", checkpoint.Revision+1
	checkpoint.HeartbeatAt = now()
	body, err := json.Marshal(checkpoint)
	require(t, err)
	require(t, writer.transactSQLite(ctx, func(tx *sqliteTx) error {
		if err := tx.UpdateNativeTaskCAS(claimed, 1); err != nil {
			return err
		}
		if err := tx.UpdateProjectTaskAuthorityCAS(authority, authority.AuthorityVersion-1); err != nil {
			return err
		}
		if err := tx.PutTaskDependency(TaskDependencyRecord{ProjectIdentity: project, PredecessorID: "2", SuccessorID: "10"}); err != nil {
			return err
		}
		if _, err := tx.AdvanceTaskGraphRevision(project, authority.AuthorityVersion, authority.GraphRevision); err != nil {
			return err
		}
		return tx.PutProjection(project, checkpoint.Revision, body)
	}))
	view := reader.Reader.ReadProjectViewTx(ctx, tx, project, "99", 1)
	requireProjectView(t, view)
	if view.Tasks.DatabaseRevision != page.DatabaseRevision || view.Tasks.NextCursor == "" || view.SelectedTask == nil || view.SelectedTask.Status != "open" ||
		len(view.Ready.Tasks) != 3 || view.Domain.Tasks[2].Status != "open" || view.Runtime == nil ||
		view.Runtime.Service.InstanceID != "old-instance" || view.Runtime.Tasks[2].Status != "open" ||
		view.Runtime.Control.Revision != page.AuthorityVersion || !view.Runtime.Control.DispatchPaused {
		t.Fatalf("writer commit leaked into the pinned project view: %+v", view)
	}
	require(t, tx.Commit())
	next, err := (NewQueryService(reader, project)).ReadProjectView(ctx, "99", 1)
	require(t, err)
	requireProjectView(t, next)
	if next.Tasks.DatabaseRevision <= page.DatabaseRevision || next.Tasks.GraphRevision != page.GraphRevision+1 ||
		next.SelectedTask.Status != "in_progress" || len(next.Ready.Tasks) != 1 || next.Ready.Tasks[0].TaskID != "2" ||
		next.Domain.Tasks[2].Status != "in_progress" || next.Runtime.Tasks[2].Status != "in_progress" ||
		next.Runtime.Service.InstanceID != "new-instance" || next.Runtime.Control.DispatchPaused || next.Runtime.Control.Revision != authority.AuthorityVersion {
		t.Fatalf("next project refresh did not adopt the new committed snapshot: %+v", next)
	}
}

func TestProjectViewKeepsReadOnlySectionsWithoutCheckpointOrSelection(t *testing.T) {
	ctx := context.Background()
	store, project, _ := nativeAuditStore(t)
	query := NewQueryService(store, project)
	view, err := query.ReadProjectView(ctx, "2", 1)
	require(t, err)
	requireProjectView(t, view)
	if view.SelectedTask != &view.Tasks.Tasks[0] || view.Runtime != nil {
		t.Fatal("in-page selection was reloaded or a missing checkpoint became a runtime")
	}
	missing, err := query.ReadProjectView(ctx, "100", 1)
	require(t, err)
	requireNativeAuditCode(t, missing.SelectedTaskError, "task_not_found")
	if missing.Tasks == nil || missing.Ready == nil || missing.Domain == nil || missing.SelectedTask != nil {
		t.Fatal("missing selection discarded usable project data")
	}
	if _, err := query.ReadProjectView(ctx, "", 1); err != nil {
		t.Fatal(err)
	}
	require(t, store.transactSQLite(ctx, func(tx *sqliteTx) error { return tx.PutProjection(project, 1, []byte("invalid JSON")) }))
	corrupt, err := query.ReadProjectView(ctx, "", 1)
	require(t, err)
	if corrupt.RuntimeError == nil || corrupt.Runtime != nil || corrupt.Tasks == nil || corrupt.Ready == nil || corrupt.Domain == nil {
		t.Fatal("corrupt checkpoint hid task data or became a usable runtime")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	failed, err := query.ReadProjectView(canceled, "", 1)
	if err == nil || failed.Tasks != nil || failed.Domain != nil {
		t.Fatal("canceled transaction exposed a successful partial snapshot")
	}
}
