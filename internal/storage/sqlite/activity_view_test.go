package sqlite

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
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
	initial := executionmodel.ExecutionRecord{ProjectIdentity: project, WorkflowRunID: "run", ExecutionID: "execution", Status: "active", CurrentStep: "implement"}
	require(t, writer.transactSQLite(ctx, func(tx *sqliteTx) error {
		if err := tx.PutWorkflowRun(executionmodel.WorkflowRunRecord{ProjectIdentity: project, WorkflowRunID: "run", TaskID: tasks[0].TaskID, WorkflowVersion: "minimal-v1", Phase: "executing"}); err != nil {
			return err
		}
		return tx.PutExecution(initial)
	}))
	var sequence int
	var name, path string
	require(t, writer.db.QueryRowContext(ctx, "PRAGMA database_list").Scan(&sequence, &name, &path))
	reader, err := OpenSQLiteReader(path)
	require(t, err)
	defer reader.Close()
	tx, err := reader.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	require(t, err)
	defer tx.Rollback()
	updated := initial
	updated.Status = "succeeded"
	committed := false
	snapshot, events, err := readActivityView(ctx, activityCommitBetweenReads{tx: tx, commit: func() {
		if committed {
			t.Fatal("unexpected duplicate event read")
		}
		committed = true
		require(t, writer.transactSQLite(ctx, func(tx *sqliteTx) error {
			if err := tx.PutExecution(updated); err != nil {
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
	for _, execution := range snapshot.Executions {
		if execution.ExecutionID == updated.ExecutionID {
			foundBefore = execution.Status == initial.Status
		}
	}
	if !foundBefore {
		t.Fatal("activity snapshot omitted the fixed task")
	}
	require(t, tx.Commit())
	next, events, err := reader.ReadActivityView(ctx, project, "execution", "attempt", 0, 200, 1<<20)
	require(t, err)
	found := false
	for _, execution := range next.Executions {
		if execution.ExecutionID == updated.ExecutionID && execution.Status == updated.Status {
			found = true
		}
	}
	if !found || len(events) != 1 || events[0].ExternalID != "new" {
		t.Fatalf("next snapshot did not adopt complete commit: found=%t events=%+v", found, events)
	}
}
