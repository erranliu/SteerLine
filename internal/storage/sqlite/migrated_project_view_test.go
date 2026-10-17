package sqlite

import (
	"context"
	"database/sql"
	"testing"

	productquery "github.com/erranliu/SteerLine/internal/product/query"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func requireProjectView(t *testing.T, view storageports.ProjectView) {
	t.Helper()
	for _, err := range []error{view.TaskError, view.SelectedTaskError, view.HistoryError} {
		require(t, err)
	}
	if view.Tasks == nil || view.Ready == nil || view.Candidates == nil {
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
	view, err := productquery.NewService(store, "project").ReadProjectView(context.Background(), "", 1)
	if err == nil || view.Tasks != nil || view.Candidates != nil {
		t.Fatal("nil reader exposed a successful project view")
	}
}

func TestProjectViewSharesTransactionAcrossWriterCommit(t *testing.T) {
	ctx := context.Background()
	writer, project, tasks := nativeAuditStore(t)
	var sequence int
	var name, path string
	require(t, writer.db.QueryRowContext(ctx, "PRAGMA database_list").Scan(&sequence, &name, &path))
	reader, err := OpenSQLiteReader(path)
	require(t, err)
	defer reader.Close()
	tx, err := reader.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	require(t, err)
	defer tx.Rollback()
	// Pin the WAL snapshot before a different connection commits task and graph
	// changes. Every project section must keep the older sampled revision.
	page, err := reader.Reader.NativeTaskPageTx(ctx, tx, project, "", "", 0, 0, 1)
	require(t, err)
	authority, err := writer.NativeAuthority(ctx, project)
	require(t, err)
	authority.AuthorityVersion++
	authority.DispatchPaused = false
	claimed := tasks[2]
	claimed.Status, claimed.Claimant, claimed.Version = "in_progress", "operator", 2
	require(t, writer.transactSQLite(ctx, func(tx *sqliteTx) error {
		if err := tx.UpdateNativeTaskCAS(claimed, 1); err != nil {
			return err
		}
		if err := tx.UpdateProjectTaskAuthorityCAS(authority, authority.AuthorityVersion-1); err != nil {
			return err
		}
		if err := tx.PutTaskDependency(taskmodel.TaskDependencyRecord{ProjectIdentity: project, PredecessorID: "2", SuccessorID: "10"}); err != nil {
			return err
		}
		_, err := tx.AdvanceTaskGraphRevision(project, authority.AuthorityVersion, authority.GraphRevision)
		return err
	}))
	view := reader.Reader.ReadProjectViewTx(ctx, tx, project, "99", 1)
	requireProjectView(t, view)
	if view.Tasks.DatabaseRevision != page.DatabaseRevision || !view.Tasks.HasMore || view.Tasks.NextCursor != "" || view.SelectedTask == nil || view.SelectedTask.Status != "open" ||
		len(view.Ready.Tasks) != 3 {
		t.Fatalf("writer commit leaked into the pinned product view: %+v", view)
	}
	require(t, tx.Commit())
	next, err := (productquery.NewService(reader, project)).ReadProjectView(ctx, "99", 1)
	require(t, err)
	requireProjectView(t, next)
	if next.Tasks.DatabaseRevision <= page.DatabaseRevision || next.Tasks.GraphRevision != page.GraphRevision+1 ||
		next.SelectedTask.Status != "in_progress" || len(next.Ready.Tasks) != 1 || next.Ready.Tasks[0].TaskID != "2" {
		t.Fatalf("next project refresh did not adopt the new committed snapshot: %+v", next)
	}
}

func TestProjectViewKeepsReadOnlySectionsWithoutCheckpointOrSelection(t *testing.T) {
	ctx := context.Background()
	store, project, _ := nativeAuditStore(t)
	query := productquery.NewService(store, project)
	view, err := query.ReadProjectView(ctx, "2", 1)
	require(t, err)
	requireProjectView(t, view)
	if view.SelectedTask != &view.Tasks.Tasks[0] {
		t.Fatal("in-page selection was reloaded")
	}
	missing, err := query.ReadProjectView(ctx, "100", 1)
	require(t, err)
	requireNativeAuditCode(t, missing.SelectedTaskError, "task_not_found")
	if missing.Tasks == nil || missing.Ready == nil || missing.Candidates == nil || missing.SelectedTask != nil {
		t.Fatal("missing selection discarded usable product data")
	}
	if _, err := query.ReadProjectView(ctx, "", 1); err != nil {
		t.Fatal(err)
	}
	// Product query doesn't read service checkpoint state. A malformed service
	// checkpoint is reported by the host observer without hiding task/history data.
	require(t, store.transactSQLite(ctx, func(tx *sqliteTx) error { return tx.PutProjection(project, 1, []byte("invalid JSON")) }))
	independent, err := query.ReadProjectView(ctx, "", 1)
	require(t, err)
	requireProjectView(t, independent)
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	failed, err := query.ReadProjectView(canceled, "", 1)
	if err == nil || failed.Tasks != nil || failed.Candidates != nil {
		t.Fatal("canceled transaction exposed a successful partial snapshot")
	}
}
