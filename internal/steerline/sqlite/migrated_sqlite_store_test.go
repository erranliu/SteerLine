package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	productquery "github.com/erranliu/SteerLine/internal/product/query"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func openDomainTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	s, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "steerline.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

const oldActiveWorkflowIndex = `CREATE UNIQUE INDEX one_active_workflow_per_task ON workflow_runs(project_identity,task_id) WHERE phase <> 'completed'`

func TestSQLiteV3IsRejectedWithoutUpgrade(t *testing.T) {
	path := filepath.Join(t.TempDir(), "steerline.db")
	raw, err := sql.Open("sqlite", path)
	require(t, err)
	tx, err := raw.Begin()
	require(t, err)
	_, err = tx.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`)
	require(t, err)
	for _, stmt := range append(append([]string{}, domainExecutionSchema...), nativeTaskSchema...) {
		_, err = tx.Exec(stmt)
		require(t, err)
	}
	_, err = tx.Exec(oldActiveWorkflowIndex)
	require(t, err)
	_, err = tx.Exec(`INSERT INTO schema_migrations(version,applied_at) VALUES(3,?)`, time.Now().UTC().Format(time.RFC3339Nano))
	require(t, err)
	require(t, tx.Commit())
	require(t, raw.Close())
	assertOldSQLiteRejectedUnchanged(t, path, 3)
}

func TestSQLiteV4IsRejectedWithoutUpgrade(t *testing.T) {
	path := filepath.Join(t.TempDir(), "steerline.db")
	store, err := OpenSQLiteStore(path)
	require(t, err)
	require(t, store.Close())
	raw, err := sql.Open("sqlite", path)
	require(t, err)
	_, err = raw.Exec(oldActiveWorkflowIndex)
	require(t, err)
	_, err = raw.Exec(`DELETE FROM schema_migrations WHERE version=?`, domainSchemaVersion)
	require(t, err)
	_, err = raw.Exec(`INSERT INTO schema_migrations(version,applied_at) VALUES(4,?)`, now().Format(time.RFC3339Nano))
	require(t, err)
	require(t, raw.Close())
	assertOldSQLiteRejectedUnchanged(t, path, 4)
	reader, err := openSQLiteInspector(path)
	require(t, err)
	defer reader.Close()
	var definition string
	require(t, reader.db.QueryRow(`SELECT sql FROM sqlite_master WHERE name='one_active_workflow_per_task'`).Scan(&definition))
	if strings.Join(strings.Fields(definition), " ") != oldActiveWorkflowIndex {
		t.Fatalf("old workflow index changed during rejection: %s", definition)
	}
}

func assertOldSQLiteRejectedUnchanged(t *testing.T, path string, version int) {
	t.Helper()
	before, err := os.ReadFile(path)
	require(t, err)
	for _, open := range []func(string) (*SQLiteStore, error){OpenSQLiteReader, OpenSQLiteStore} {
		store, openErr := open(path)
		if store != nil {
			_ = store.Close()
		}
		if openErr == nil || !strings.Contains(openErr.Error(), fmt.Sprintf("unsupported SQLite schema %d", version)) {
			t.Fatalf("schema %d was admitted: %v", version, openErr)
		}
	}
	if err = validateExistingDatabase(path); err == nil || !strings.Contains(err.Error(), fmt.Sprintf("unsupported SQLite schema %d", version)) {
		t.Fatalf("schema %d passed preflight: %v", version, err)
	}
	after, err := os.ReadFile(path)
	require(t, err)
	if !bytes.Equal(before, after) {
		t.Fatalf("schema %d database changed during rejection", version)
	}
	reader, err := openSQLiteInspector(path)
	require(t, err)
	defer reader.Close()
	var got int
	require(t, reader.db.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&got))
	if got != version {
		t.Fatalf("schema version changed: got %d, want %d", got, version)
	}
}

func TestSQLiteV5IsRejectedWithoutUpgrade(t *testing.T) {
	path := filepath.Join(t.TempDir(), "steerline.db")
	store, err := OpenSQLiteStore(path)
	require(t, err)
	require(t, store.Close())
	raw, err := sql.Open("sqlite", path)
	require(t, err)
	_, err = raw.Exec(`UPDATE schema_migrations SET version=5 WHERE version=?`, domainSchemaVersion)
	require(t, err)
	require(t, raw.Close())
	assertOldSQLiteRejectedUnchanged(t, path, 5)
}

func TestSQLiteV12IsRejectedWithoutUpgrade(t *testing.T) {
	path := filepath.Join(t.TempDir(), "steerline.db")
	store, err := OpenSQLiteStore(path)
	require(t, err)
	require(t, store.Close())
	raw, err := sql.Open("sqlite", path)
	require(t, err)
	_, err = raw.Exec(`UPDATE schema_migrations SET version=12 WHERE version=?`, domainSchemaVersion)
	require(t, err)
	require(t, raw.Close())
	assertOldSQLiteRejectedUnchanged(t, path, 12)
}

func TestSQLiteRejectsParallelExecutionsInOneWorkflow(t *testing.T) {
	ctx := context.Background()
	s := openDomainTestStore(t)
	project := "repo:test"
	err := s.transactSQLite(ctx, func(tx *sqliteTx) error {
		if err := tx.PutTask(taskmodel.TaskRecord{ProjectIdentity: project, TaskID: "17", Status: "in_progress", Version: 1}); err != nil {
			return err
		}
		if err := tx.PutWorkflowRun(executionmodel.WorkflowRunRecord{ProjectIdentity: project, WorkflowRunID: "wf-1", TaskID: "17", WorkflowVersion: "minimal-v1", Phase: "executing"}); err != nil {
			return err
		}
		for _, id := range []string{"exec-a"} {
			if err := tx.PutExecution(executionmodel.ExecutionRecord{ProjectIdentity: project, ExecutionID: id, WorkflowRunID: "wf-1", Agent: id, Status: "active", CurrentStep: "implement", Target: executionbackend.ExecutionTarget{WorktreeID: "worktree-" + id, Branch: "branch-" + id, Dispatch: "dispatch-" + id}}); err != nil {
				return err
			}
			if err := tx.PutStepAttempt(executionmodel.StepAttemptRecord{ProjectIdentity: project, AttemptID: "attempt-" + id, ExecutionID: id, StepID: "implement", AttemptNumber: 1, Status: "running"}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	err = s.transactSQLite(ctx, func(tx *sqliteTx) error {
		return tx.PutExecution(executionmodel.ExecutionRecord{ProjectIdentity: project, ExecutionID: "exec-b", WorkflowRunID: "wf-1", Status: "active", CurrentStep: "verify"})
	})
	if err == nil {
		t.Fatal("second active execution was accepted")
	}
	snapshot, err := s.Query(ctx, project)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.WorkflowRuns) != 1 || len(snapshot.Executions) != 1 || len(snapshot.Attempts) != 1 {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
}

func TestSQLiteTransactionRollsBackAllDomainWrites(t *testing.T) {
	ctx := context.Background()
	s := openDomainTestStore(t)
	project := "repo:rollback"
	err := s.transactSQLite(ctx, func(tx *sqliteTx) error {
		if err := tx.PutTask(taskmodel.TaskRecord{ProjectIdentity: project, TaskID: "task-a", Status: "open", Version: 1}); err != nil {
			return err
		}
		return tx.PutWorkflowRun(executionmodel.WorkflowRunRecord{ProjectIdentity: project, WorkflowRunID: "wf-bad", TaskID: "missing-task", Phase: "queued"})
	})
	if err == nil {
		t.Fatal("expected FK violation")
	}
	snapshot, queryErr := s.Query(ctx, project)
	if queryErr != nil {
		t.Fatal(queryErr)
	}
	if len(snapshot.Tasks) != 0 || len(snapshot.WorkflowRuns) != 0 {
		t.Fatalf("partial state survived rollback: %+v", snapshot)
	}
}

func TestPreflightRejectsUnknownSQLiteSchemaWithoutMutation(t *testing.T) {
	runDir := t.TempDir()
	path := filepath.Join(runDir, "steerline.db")
	db, err := sql.Open("sqlite", path)
	require(t, err)
	if _, err = db.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO schema_migrations(version,applied_at) VALUES(99,'unknown')`); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	if err = validateExistingDatabase(path); err == nil || !strings.Contains(err.Error(), "unsupported SQLite schema 99") {
		t.Fatalf("unknown database schema was accepted: %v", err)
	}
	reader, err := openSQLiteInspector(path)
	require(t, err)
	defer reader.Close()
	var version, tableCount int
	require(t, reader.db.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&version))
	require(t, reader.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table'`).Scan(&tableCount))
	if version != 99 || tableCount != 1 {
		t.Fatalf("preflight modified unknown database: version=%d tables=%d", version, tableCount)
	}
}

func TestPreflightRejectsRetiredWholeRunResultCommandWithoutMutation(t *testing.T) {
	ctx := context.Background()
	runDir := t.TempDir()
	databasePath := filepath.Join(runDir, "steerline.db")
	store, err := OpenSQLiteStore(databasePath)
	require(t, err)
	commandID, idempotencyKey := newID(), newID()
	command := storageports.CommandRecord{ProjectIdentity: "repo:retired-result", CommandID: commandID, Kind: "accept_result", TargetID: "retired-run", ExpectedVersion: 1, IdempotencyKey: idempotencyKey, RequestedBy: "operator", Payload: []byte(`{}`), Status: "pending", CreatedAt: now()}
	err = store.transactSQLite(ctx, func(tx *sqliteTx) error {
		_, _, enqueueErr := tx.EnqueueCommand(command)
		return enqueueErr
	})
	require(t, err)
	require(t, store.Close())

	reader, err := OpenSQLiteReader(databasePath)
	require(t, err)
	defer reader.Close()
	if err = validateRetiredWholeRunResultCommands(ctx, reader.db); err == nil || !strings.Contains(err.Error(), "retired whole-run result commands") {
		t.Fatalf("retired result command was not rejected during read-only preflight: %v", err)
	}
	saved, err := reader.Command(ctx, command.ProjectIdentity, commandID)
	require(t, err)
	if saved.Kind != command.Kind || saved.Status != command.Status || string(saved.Payload) != string(command.Payload) {
		t.Fatalf("preflight changed retired command state: %+v", saved)
	}
}

func TestQueryEventCursorAndLargeBodyPaging(t *testing.T) {
	ctx := context.Background()
	store := openDomainTestStore(t)
	project := "repo:event-query"
	body := make([]byte, (1<<20)+12345)
	for i := range body {
		body[i] = byte(i % 251)
	}
	var firstID int64
	err := store.transactSQLite(ctx, func(tx *sqliteTx) error {
		var err error
		firstID, err = tx.AppendEvent(storageports.DomainEvent{ProjectIdentity: project, ExecutionID: "exec", AttemptID: "attempt", ExternalSource: "test", ExternalID: "1", Type: "conversation.message", Payload: body})
		if err != nil {
			return err
		}
		_, err = tx.AppendEvent(storageports.DomainEvent{ProjectIdentity: project, ExecutionID: "exec", AttemptID: "attempt", ExternalSource: "test", ExternalID: "2", Type: "observation", Payload: []byte("small")})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	query := productquery.NewService(store, project)
	page, err := query.ReadEvents(ctx, productquery.EventFilter{ExecutionID: "exec", AttemptID: "attempt"}, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Events) != 1 || page.Events[0].EventID != firstID || !page.Events[0].PayloadOmitted || !page.HasMore || page.NextCursor == "" {
		t.Fatalf("large event page was not bounded: %+v", page)
	}
	firstBody, err := query.ReadEventBody(ctx, firstID, 0, 256<<10)
	if err != nil {
		t.Fatal(err)
	}
	if firstBody.Complete || firstBody.TotalBytes != int64(len(body)) || firstBody.Digest != operationDigest(body) {
		t.Fatalf("bad first body page: %+v", firstBody)
	}
	secondBody, err := query.ReadEventBody(ctx, firstID, firstBody.NextOffset, 256<<10)
	if err != nil {
		t.Fatal(err)
	}
	thirdBody, err := query.ReadEventBody(ctx, firstID, secondBody.NextOffset, 256<<10)
	if err != nil {
		t.Fatal(err)
	}
	fourthBody, err := query.ReadEventBody(ctx, firstID, thirdBody.NextOffset, 256<<10)
	if err != nil {
		t.Fatal(err)
	}
	fifthBody, err := query.ReadEventBody(ctx, firstID, fourthBody.NextOffset, 256<<10)
	if err != nil {
		t.Fatal(err)
	}
	joined := append(append(append(append(append([]byte{}, firstBody.Data...), secondBody.Data...), thirdBody.Data...), fourthBody.Data...), fifthBody.Data...)
	if !fifthBody.Complete || operationDigest(joined) != operationDigest(body) {
		t.Fatal("event body pages did not reconstruct the original payload")
	}
	secondPage, err := query.ReadEvents(ctx, productquery.EventFilter{ExecutionID: "exec", AttemptID: "attempt"}, page.NextCursor, 1)
	if err != nil || len(secondPage.Events) != 1 || string(secondPage.Events[0].Payload) != "small" {
		t.Fatalf("second event page: %+v %v", secondPage, err)
	}
	if _, err = query.ReadEvents(ctx, productquery.EventFilter{ExecutionID: "other"}, page.NextCursor, 1); err == nil {
		t.Fatal("cursor was reused with a different filter")
	} else if cursorErr, ok := err.(*productquery.CursorError); !ok || cursorErr.Code != "cursor_invalidated" {
		t.Fatalf("bad cursor error: %v", err)
	}
}

func TestOperationIdempotencyKeepsOneDurableIntent(t *testing.T) {
	ctx := context.Background()
	store := openDomainTestStore(t)
	project := "repo:ops"
	request := []byte(`{"attempt_id":"attempt-1"}`)
	digest := operationDigest(request)
	base := executionmodel.OperationRecord{ProjectIdentity: project, OperationID: "operation-1", IdempotencyKey: "start:attempt-1", Kind: "start_execution", Status: "pending", PayloadDigest: digest, Request: request}
	err := store.transactSQLite(ctx, func(tx *sqliteTx) error {
		_, created, err := tx.PutOperation(base)
		if err == nil && !created {
			t.Fatal("initial operation was not created")
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	updated := base
	updated.Status = "in_flight"
	err = store.transactSQLite(ctx, func(tx *sqliteTx) error {
		_, created, err := tx.PutOperation(updated)
		if err == nil && created {
			t.Fatal("retry created a second operation")
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	conflict := base
	conflict.Request = []byte(`{"attempt_id":"other"}`)
	conflict.PayloadDigest = operationDigest(conflict.Request)
	err = store.transactSQLite(ctx, func(tx *sqliteTx) error { _, _, err := tx.PutOperation(conflict); return err })
	if err == nil {
		t.Fatal("same idempotency key with a different payload was accepted")
	}
	var status string
	if err = store.db.QueryRowContext(ctx, `SELECT status FROM operations WHERE project_identity=? AND operation_id=?`, project, base.OperationID).Scan(&status); err != nil || status != "in_flight" {
		t.Fatalf("operation status was not retained: %s %v", status, err)
	}
}
