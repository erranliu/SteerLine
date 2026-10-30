package sqlite

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestLegacyCheckRetirementSchemaKeepsProjectPreparation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	store, err := OpenSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, table := range []string{"environment_checks", "environment_check_requests"} {
		var count int
		if err := store.db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&count); err != nil || count != 0 {
			t.Fatal("retired table remains", table, count, err)
		}
	}
	for _, table := range []string{"project_environments", "project_environment_current", "runner_candidates"} {
		var count int
		if err := store.db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&count); err != nil || count != 1 {
			t.Fatal("current preparation or execution table lost", table, count, err)
		}
	}
	if _, err := store.db.Exec("UPDATE schema_migrations SET version=?", SchemaVersion-1); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenSQLiteStore(path); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatal("old schema must be rejected without migration", err)
	}
}
