package sqlite

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func TestNativeDatabaseInitializesCurrentSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "steerline.db")
	// A fresh database contains the center model and native record constraints.
	db, err := OpenSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var version int
	if err = db.db.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != domainSchemaVersion {
		t.Fatalf("schema version=%d", version)
	}
	for _, table := range []string{"project_task_authorities", "task_contracts", "task_batches", "task_dependencies", "task_deliveries", "database_state"} {
		var count int
		if err = db.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&count); err != nil || count != 1 {
			t.Fatalf("missing table %s: count=%d err=%v", table, count, err)
		}
	}
	var revision uint64
	if err = db.db.QueryRow(`SELECT revision FROM database_state WHERE singleton=1`).Scan(&revision); err != nil || revision != 0 {
		t.Fatalf("unexpected initial DB revision %d: %v", revision, err)
	}
}

func TestSchemaOneCenterDatabaseIsRejectedWithoutMutation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "center-v1.db")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := raw.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	for _, stmt := range domainExecutionSchema {
		if _, err = tx.Exec(stmt); err != nil {
			t.Fatalf("create center schema: %v: %s", err, stmt)
		}
	}
	created := time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)
	old := taskmodel.TaskRecord{ProjectIdentity: "repo:previous-center", TaskID: "legacy:stable", Title: "center task", Source: "beads", Status: "in_progress", Version: 2, CreatedAt: created, UpdatedAt: created}
	payload, err := json.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`INSERT INTO tasks(project_identity,task_id,status,version,payload) VALUES(?,?,?,?,?)`, old.ProjectIdentity, old.TaskID, old.Status, old.Version, payload); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`INSERT INTO schema_migrations(version,applied_at) VALUES(1,?)`, created.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err = raw.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if store, err := OpenSQLiteStore(path); err == nil {
		store.Close()
		t.Fatal("previous database was automatically upgraded")
	} else {
		requireNativeAuditCode(t, err, "unsupported_schema")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("rejected database was modified: %v", err)
	}
}
