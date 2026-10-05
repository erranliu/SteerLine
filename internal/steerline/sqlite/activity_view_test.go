package sqlite

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

type activityCommitBetweenReads struct {
	tx     *sql.Tx
	commit func()
}

func (q activityCommitBetweenReads) QueryContext(ctx context.Context, statement string, args ...any) (*sql.Rows, error) {
	if strings.HasPrefix(statement, "WITH batch AS") {
		q.commit()
	}
	return q.tx.QueryContext(ctx, statement, args...)
}

func TestTM05ActivitySharesTransactionAcrossWriterCommit(t *testing.T) {
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
	updated := tasks[0]
	updated.Status, updated.Claimant, updated.Version = "in_progress", "operator", updated.Version+1
	committed := false
	snapshot, events, err := readActivityView(ctx, activityCommitBetweenReads{tx: tx, commit: func() {
		if committed {
			t.Fatal("unexpected duplicate event read")
		}
		committed = true
		require(t, writer.transactSQLite(ctx, func(tx *sqliteTx) error {
			if err := tx.UpdateNativeTaskCAS(updated, tasks[0].Version); err != nil {
				return err
			}
			_, err := tx.AppendEvent(storageports.DomainEvent{ProjectIdentity: project, WorkflowRunID: "run", ExecutionID: "execution", AttemptID: "attempt", ExternalSource: "activity-snapshot", ExternalID: "new", Type: "activity.worker", Payload: []byte(`{"title":"new"}`)})
			return err
		}))
	}}, project, "execution", "attempt", 0, 200, 1<<20)
	require(t, err)
	if !committed || len(events) != 0 {
		t.Fatalf("new event leaked into old snapshot: committed=%t events=%+v", committed, events)
	}
	foundBefore := false
	for _, task := range snapshot.Tasks {
		if task.TaskID == updated.TaskID {
			foundBefore = true
		}
		if task.TaskID == updated.TaskID && task.Status != tasks[0].Status {
			t.Fatal("new task state leaked into old snapshot")
		}
	}
	if !foundBefore {
		t.Fatal("activity snapshot omitted the fixed task")
	}
	require(t, tx.Commit())
	next, events, err := reader.ReadActivityView(ctx, project, "execution", "attempt", 0, 200, 1<<20)
	require(t, err)
	found := false
	for _, task := range next.Tasks {
		if task.TaskID == updated.TaskID && task.Status == updated.Status {
			found = true
		}
	}
	if !found || len(events) != 1 || events[0].ExternalID != "new" {
		t.Fatalf("next snapshot did not adopt complete commit: found=%t events=%+v", found, events)
	}
}
