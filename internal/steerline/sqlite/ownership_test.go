package sqlite

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/erranliu/SteerLine/internal/steerline/persistence"
)

func databaseFiles(t *testing.T, path string) map[string][]byte {
	t.Helper()
	files := map[string][]byte{}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		b, err := os.ReadFile(path + suffix)
		if os.IsNotExist(err) {
			continue
		}
		require(t, err)
		files[suffix] = b
	}
	return files
}

func TestCM70ReadOnlyOpenPreservesFiles(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	writer, err := OpenSQLiteStore(path)
	require(t, err)
	require(t, writer.InitializeProject(ctx, ProjectTaskAuthority{ProjectIdentity: "project", Authority: "native", AuthorityVersion: 1, Repository: "repo", Remote: "origin", Branch: "main", DispatchMode: "manual", DispatchPaused: true}))
	command, _, err := writer.EnqueueCommand(ctx, CommandRecord{ProjectIdentity: "project", CommandID: newID(), Kind: "test", TargetID: "project", RequestedBy: "operator", IdempotencyKey: newID(), Status: "pending", Payload: []byte(`{}`)})
	require(t, err)
	checkpoint, err := writer.SaveCheckpoint(ctx, ServiceCheckpoint{SchemaVersion: WorkerFlowRuntimeSchema, Repository: Repository{Identity: "project"}})
	require(t, err)
	cursor := ConversationCheckpoint{ProjectID: "project", ExecutionID: "execution", AttemptID: "attempt", Source: "source", SourceSegment: "segment", Cursor: "first", LocalSeq: 1}
	_, err = writer.CommitConversation(ctx, nil, cursor)
	require(t, err)
	// Capture persisted values before opening the reader, including nonempty
	// command/checkpoint/cursor records so consumption and progress are observable.
	durableState := func(store *SQLiteStore) []string {
		t.Helper()
		var state []string
		for _, query := range []string{
			`SELECT group_concat(type || ':' || name || ':' || COALESCE(sql,''), char(10)) FROM (SELECT * FROM sqlite_schema ORDER BY type,name)`,
			`SELECT json_array(database_id,revision) FROM database_state`,
			`SELECT json_array(authority_version,graph_revision,dispatch_mode,dispatch_paused) FROM project_task_authorities`,
			`SELECT json_array(revision,hex(payload),updated_at) FROM projection`,
			`SELECT json_array(command_id,status,hex(result),updated_at) FROM commands`,
			`SELECT json_array(cursor,local_seq,source_identity,generation,updated_at) FROM conversation_source_cursors`,
		} {
			var value string
			require(t, store.db.QueryRowContext(ctx, query).Scan(&value))
			state = append(state, value)
		}
		return state
	}
	beforeState := durableState(writer)
	require(t, writer.Close())
	before := databaseFiles(t, path)
	reader, err := OpenSQLiteReader(path)
	require(t, err)
	id, version, err := reader.DatabaseVersion(ctx)
	require(t, err)
	beforeDomain, err := reader.Query(ctx, "project")
	require(t, err)
	if _, _, err = reader.EnqueueCommand(ctx, CommandRecord{ProjectIdentity: "project", CommandID: newID(), Kind: "test", TargetID: "project", RequestedBy: "operator", IdempotencyKey: newID(), Status: "pending", Payload: []byte(`{}`)}); err == nil {
		t.Fatal("read-only connection admitted a command")
	}
	command.Status = "completed"
	if err = reader.UpdateCommand(ctx, command); err == nil {
		t.Fatal("read-only connection consumed a pending command")
	}
	cursor.Cursor, cursor.LocalSeq = "second", 2
	if _, err = reader.CommitConversation(ctx, nil, cursor); err == nil {
		t.Fatal("read-only connection admitted source progress")
	}
	if _, err = reader.SaveCheckpoint(ctx, checkpoint); err == nil {
		t.Fatal("read-only connection advanced checkpoint")
	}
	for _, query := range []string{`UPDATE database_state SET revision=revision+1`, `CREATE TABLE forbidden_write (id INTEGER)`} {
		if _, err = reader.db.ExecContext(ctx, query); err == nil {
			t.Fatalf("read-only driver admitted mutation: %s", query)
		}
	}
	afterID, afterVersion, err := reader.DatabaseVersion(ctx)
	require(t, err)
	if id != afterID || version != afterVersion {
		t.Fatal("read-only connection advanced durable state")
	}
	afterDomain, err := reader.Query(ctx, "project")
	require(t, err)
	if !reflect.DeepEqual(beforeDomain, afterDomain) || !reflect.DeepEqual(beforeState, durableState(reader)) {
		t.Fatal("read-only connection changed schema, business, versions, checkpoint, commands or source cursor")
	}
	require(t, reader.Close())
	after := databaseFiles(t, path)
	if len(before[""]) == 0 || !bytes.Equal(before[""], after[""]) {
		t.Fatal("read-only open changed the main database")
	}
	// CM70-2 permits SQLite's own WAL/SHM coordination files. In this closed
	// writer fixture the WAL must stay empty: no business frame or checkpoint.
	if len(before["-wal"]) != 0 || len(after["-wal"]) != 0 {
		t.Fatal("read-only open appended WAL frames")
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	require(t, err)
	for _, entry := range entries {
		if entry.Name() != "state.db" && entry.Name() != "state.db-wal" && entry.Name() != "state.db-shm" {
			t.Fatalf("read-only open created an unexpected file: %s", entry.Name())
		}
	}
	t.Logf("main before/after: bytes=%d/%d sha256=%x/%x; WAL bytes=%d/%d; SHM bytes=%d/%d", len(before[""]), len(after[""]), sha256.Sum256(before[""]), sha256.Sum256(after[""]), len(before["-wal"]), len(after["-wal"]), len(before["-shm"]), len(after["-shm"]))
}

func TestCM70WindowsCloseReleasesHandlesAndMissingReaderCannotCreate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	writer, err := OpenSQLiteStore(path)
	require(t, err)
	require(t, writer.Close())
	reader, err := OpenSQLiteReader(path)
	require(t, err)
	require(t, reader.Close())
	// Windows rejects deletion while SQLite retains a live handle.
	for suffix := range databaseFiles(t, path) {
		require(t, os.Remove(path+suffix))
	}
	if _, err = (Factory{}).OpenReader(path); err == nil {
		t.Fatal("reader recreated a missing database")
	}
	if files := databaseFiles(t, path); len(files) != 0 {
		t.Fatal("missing read-only open created files")
	}
	missingParent := filepath.Join(filepath.Dir(path), "absent")
	if _, err = (Factory{}).OpenReader(filepath.Join(missingParent, "state.db")); err == nil {
		t.Fatal("reader admitted a missing parent/database")
	}
	if _, err = os.Stat(missingParent); !os.IsNotExist(err) {
		t.Fatalf("missing read-only open created a directory: %v", err)
	}
}

func TestCM70StorageFailuresRemainDistinctFromMissingRecords(t *testing.T) {
	store := openDomainTestStore(t)
	_, err := store.NativeAuthority(context.Background(), "missing")
	if !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("missing authority lost pure no-row classification: %v", err)
	}
	require(t, store.Close())
	_, err = store.Query(context.Background(), "project")
	if err == nil || errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("closed connection became a missing record: %v", err)
	}
	_, err = (Factory{}).OpenReader(filepath.Join(t.TempDir(), "missing.db"))
	var failure *persistence.StorageError
	if !errors.As(err, &failure) || failure.Code != persistence.StorageIO {
		t.Fatalf("open failure lost storage classification: %v", err)
	}
}

func TestCM70WriterLockHasPureClassificationAndNoCommandWrite(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	holder, err := OpenSQLiteStore(path)
	require(t, err)
	defer holder.Close()
	blocked, err := OpenSQLiteStore(path)
	require(t, err)
	defer blocked.Close()
	blocked.db.SetMaxIdleConns(1)
	_, err = blocked.db.ExecContext(ctx, "PRAGMA busy_timeout=1")
	require(t, err)
	conn, err := holder.db.Conn(ctx)
	require(t, err)
	defer conn.Close()
	_, err = conn.ExecContext(ctx, "BEGIN IMMEDIATE")
	require(t, err)
	defer conn.ExecContext(ctx, "ROLLBACK")
	_, _, err = blocked.EnqueueCommand(ctx, CommandRecord{ProjectIdentity: "project", CommandID: newID(), Kind: "test", IdempotencyKey: newID(), Status: "pending"})
	var failure *persistence.StorageError
	if !errors.As(err, &failure) || failure.Code != persistence.StorageLocked || errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("writer lock became another outcome: %v", err)
	}
	var count int
	require(t, blocked.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM commands").Scan(&count))
	if count != 0 {
		t.Fatal("blocked transaction persisted a command")
	}
}
