package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// preflightExistingState rejects obsolete or unknown state before config loading
// creates directories, probes task providers, or opens SQLite for writing.
func preflightExistingState(runDir, runtimeFile string) error {
	for _, name := range []string{"bindings", "operations"} {
		entries, err := os.ReadDir(filepath.Join(runDir, "execution", "v1", name))
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if len(entries) != 0 {
			return errors.New("file-based execution state is unsupported; preserve external executions for manual recovery")
		}
	}
	runtimePath := filepath.Join(runDir, runtimeFile)
	databasePath := filepath.Join(runDir, "steerline.db")
	_, dbErr := os.Stat(databasePath)
	databaseExists := dbErr == nil
	if dbErr != nil && !errors.Is(dbErr, os.ErrNotExist) {
		return dbErr
	}
	if databaseExists {
		// A supported database can rebuild a missing, corrupt or unwritable
		// projection. Explicitly incompatible format versions still fail closed.
		if data, err := readSnapshot(runtimePath); err == nil {
			var header struct {
				Version int `json:"schema_version"`
			}
			if json.Unmarshal(data, &header) == nil && header.Version != 5 {
				return fmt.Errorf("unsupported runtime schema %d", header.Version)
			}
		}
		if err := validateExistingDatabase(databasePath); err != nil {
			return err
		}
		reader, err := OpenSQLiteReader(databasePath)
		if err != nil {
			return err
		}
		defer reader.Close()
		var tableCount int
		if err = reader.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='project_task_authorities'`).Scan(&tableCount); err != nil {
			return err
		}
		if tableCount == 1 {
			var authorities int
			if err = reader.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM project_task_authorities`).Scan(&authorities); err != nil {
				return err
			}
			if authorities > 0 {
				return nativeErr("authority_mismatch", "native task database cannot be opened through the Beads authority configuration")
			}
		}
		return nil
	}
	runtime, err := LoadRuntime(runtimePath)
	if err != nil {
		return err
	}
	if runtime != nil {
		return errors.New("runtime v5 projection exists without its authoritative SQLite database; restore the database before starting")
	}
	entries, err := os.ReadDir(filepath.Join(runDir, "runs"))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if len(entries) != 0 {
		return errors.New("run records exist without the authoritative SQLite database; preserve the state directory for manual recovery")
	}
	return nil
}

func validateExistingDatabase(databasePath string) error {
	reader, err := OpenSQLiteReader(databasePath)
	if err != nil {
		return fmt.Errorf("cannot inspect existing SQLite state without mutation: %w", err)
	}
	defer reader.Close()
	var version int
	if err = reader.db.QueryRowContext(context.Background(), `SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&version); err != nil {
		return fmt.Errorf("existing SQLite database has an unknown schema; preserve it for manual recovery: %w", err)
	}
	if version < 1 || version > domainSchemaVersion {
		return fmt.Errorf("unsupported SQLite schema %d; supported schema versions are 1 through %d", version, domainSchemaVersion)
	}
	var tables int
	if err = reader.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name IN ('tasks','workflow_runs','executions','step_attempts','decisions','decision_answers','events','operations','commands','projection','execution_bindings','execution_operations')`).Scan(&tables); err != nil {
		return fmt.Errorf("cannot validate SQLite schema: %w", err)
	}
	if tables != 12 {
		return fmt.Errorf("existing SQLite database schema %d is incomplete; preserve it for manual recovery", version)
	}
	if version >= 2 {
		var nativeTables int
		if err = reader.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name IN ('project_task_authorities','database_state','task_contracts','task_batches','task_dependencies','task_deliveries')`).Scan(&nativeTables); err != nil {
			return err
		}
		if nativeTables != 6 {
			return fmt.Errorf("existing SQLite database schema %d is incomplete; preserve it for manual recovery", version)
		}
		var nativeIndexes int
		if err = reader.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name IN ('task_dependencies_successor_idx','workflow_run_task_identity')`).Scan(&nativeIndexes); err != nil {
			return err
		}
		if nativeIndexes != 2 {
			return errors.New("existing SQLite database lacks native task relational constraints; preserve it for manual recovery")
		}
	}
	var indexes int
	if err = reader.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name IN ('task_external_identity','one_active_workflow_per_task','one_active_execution_per_workflow','one_active_attempt_per_execution')`).Scan(&indexes); err != nil {
		return err
	}
	if indexes != 4 {
		return errors.New("existing SQLite database lacks current identity and execution constraints; preserve it for manual recovery")
	}
	return nil
}
