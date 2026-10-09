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

func (q activityCommitBetweenReads) QueryRowContext(ctx context.Context, statement string, args ...any) *sql.Row {
	return q.tx.QueryRowContext(ctx, statement, args...)
}

func TestTM05ActivitySharesTransactionAcrossWriterCommit(t *testing.T) {
	ctx := context.Background()
	writer, project, tasks := nativeAuditStore(t)
	initial := executionmodel.ExecutionRecord{ProjectIdentity: project, WorkflowRunID: "run", ExecutionID: "execution", Status: "active", CurrentStep: "implement"}
	require(t, writer.transactSQLite(ctx, func(tx *sqliteTx) error {
		if err := tx.PutWorkflowRun(executionmodel.WorkflowRunRecord{ProjectIdentity: project, WorkflowRunID: "run", TaskID: tasks[0].TaskID, WorkflowVersion: "minimal-v1", Status: "running"}); err != nil {
			return err
		}
		if err := tx.PutExecution(initial); err != nil {
			return err
		}
		return tx.PutStepAttempt(executionmodel.StepAttemptRecord{ProjectIdentity: project, AttemptID: "attempt", ExecutionID: "execution", StepID: "worker", AttemptNumber: 1, Status: "running"})
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
	binding, events, err := readActivityView(ctx, activityCommitBetweenReads{tx: tx, commit: func() {
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
	}}, project, "run", "execution", "attempt", 0, 200, 1<<20)
	require(t, err)
	if !committed || len(events) != 0 || binding.ProjectIdentity != project || binding.ExecutionWorkflowRunID != "run" || binding.ExecutionID != "execution" || binding.AttemptID != "attempt" || binding.AttemptExecutionID != "execution" {
		t.Fatalf("new event leaked into old snapshot or activity binding changed: committed=%t binding=%+v events=%+v", committed, binding, events)
	}
	require(t, tx.Commit())
	next, events, err := reader.ReadActivityView(ctx, project, "run", "execution", "attempt", 0, 200, 1<<20)
	require(t, err)
	if next.ExecutionWorkflowRunID != "run" || len(events) != 1 || events[0].ExternalID != "new" {
		t.Fatalf("next snapshot did not adopt complete commit: binding=%+v events=%+v", next, events)
	}
}
