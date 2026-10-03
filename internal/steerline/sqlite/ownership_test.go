package sqlite

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
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
	require(t, writer.Close())
	before := databaseFiles(t, path)
	reader, err := OpenSQLiteReader(path)
	require(t, err)
	id, version, err := reader.DatabaseVersion(ctx)
	require(t, err)
	_, err = reader.Query(ctx, "project")
	require(t, err)
	if _, _, err = reader.EnqueueCommand(ctx, CommandRecord{ProjectIdentity: "project", CommandID: newID(), Kind: "test", IdempotencyKey: newID(), Status: "pending"}); err == nil {
		t.Fatal("read-only connection admitted a command")
	}
	if _, err = reader.CommitConversation(ctx, nil, ConversationCheckpoint{ProjectID: "project"}); err == nil {
		t.Fatal("read-only connection admitted source progress")
	}
	afterID, afterVersion, err := reader.DatabaseVersion(ctx)
	require(t, err)
	if id != afterID || version != afterVersion {
		t.Fatal("read-only connection advanced durable state")
	}
	require(t, reader.Close())
	after := databaseFiles(t, path)
	if len(before) != len(after) {
		t.Fatal("read-only open changed the database file set")
	}
	for name, body := range before {
		if !bytes.Equal(body, after[name]) {
			t.Fatalf("read-only open changed %q", name)
		}
	}
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
