package steerline

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// preflightExistingState rejects obsolete or unknown state before config loading
// creates directories, probes task providers, or opens SQLite for writing.
func preflightExistingState(runDir, runtimeFile string) error {
	runtimePath := filepath.Join(runDir, runtimeFile)
	runtime, err := LoadRuntime(runtimePath)
	if err != nil {
		return err
	}
	databasePath := filepath.Join(runDir, "steerline.db")
	_, dbErr := os.Stat(databasePath)
	databaseExists := dbErr == nil
	if dbErr != nil && !errors.Is(dbErr, os.ErrNotExist) {
		return dbErr
	}
	if runtime != nil && !databaseExists {
		return errors.New("runtime v5 projection exists without its authoritative SQLite database; restore the database before starting")
	}
	if !databaseExists {
		return nil
	}
	reader, err := OpenSQLiteReader(databasePath)
	if err != nil {
		return fmt.Errorf("cannot inspect existing SQLite state without mutation: %w", err)
	}
	defer reader.Close()
	var version int
	if err = reader.db.QueryRowContext(context.Background(), `SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&version); err != nil {
		return fmt.Errorf("existing SQLite database has an unknown schema; preserve it for manual recovery: %w", err)
	}
	if version != domainSchemaVersion {
		return fmt.Errorf("unsupported SQLite schema %d; only schema %d is accepted", version, domainSchemaVersion)
	}
	var tables int
	if err = reader.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name IN ('tasks','workflow_runs','executions','step_attempts','decisions','decision_answers','events','operations','commands','projection')`).Scan(&tables); err != nil {
		return fmt.Errorf("cannot validate SQLite schema: %w", err)
	}
	if tables != 10 {
		return fmt.Errorf("existing SQLite database schema %d is incomplete; preserve it for manual recovery", version)
	}
	return nil
}
