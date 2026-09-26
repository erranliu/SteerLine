package steerline

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
		if err = validateRetiredWholeRunResultCommands(context.Background(), reader.db); err != nil {
			return err
		}
		if err = validateNativeSQLiteRuntimeProjection(context.Background(), reader.db); err != nil {
			return err
		}
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

func validateRetiredWholeRunResultCommands(ctx context.Context, db *sql.DB) error {
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM commands WHERE kind='accept_result'`).Scan(&count); err != nil {
		return nativeErr("unsupported_schema", "cannot inspect retired whole-run result commands without mutation")
	}
	if count > 0 {
		return nativeErr("unsupported_schema", "database contains retired whole-run result commands; preserve it for manual recovery")
	}
	return nil
}

func validateExistingDatabase(databasePath string) error {
	reader, err := OpenSQLiteReader(databasePath)
	if err != nil {
		return nativeErr("unsupported_schema", fmt.Sprintf("cannot inspect existing SQLite state without mutation: %v", err))
	}
	defer reader.Close()
	return validateDatabaseSchemaVersion(context.Background(), reader.db, true)
}

type schemaQuery interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

// Check definitions as well as the version stamp before writing existing state.
func validateDatabaseSchema(ctx context.Context, db schemaQuery) error {
	return validateDatabaseSchemaVersion(ctx, db, false)
}

func validateDatabaseSchemaVersion(ctx context.Context, db schemaQuery, allowCurrentUpgrade bool) error {
	var version int
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&version); err != nil {
		return nativeErr("unsupported_schema", "existing SQLite database has an unknown schema; preserve it for manual recovery")
	}
	if version != domainSchemaVersion && !(allowCurrentUpgrade && version == 3) {
		return nativeErr("unsupported_schema", fmt.Sprintf("unsupported SQLite schema %d; only schema %d is accepted", version, domainSchemaVersion))
	}
	expected := map[string]string{}
	definitions := append(append([]string{}, domainExecutionSchema...), nativeTaskSchema...)
	if version == domainSchemaVersion {
		definitions = append(definitions, conversationSchemaV4...)
	}
	for _, stmt := range definitions {
		fields := strings.Fields(stmt)
		if len(fields) < 3 || fields[0] != "CREATE" {
			continue
		}
		index := 2
		if fields[1] == "UNIQUE" {
			index = 3
		}
		expected[fields[index]] = strings.Join(fields, " ")
	}
	rows, err := db.QueryContext(ctx, `SELECT name,sql FROM sqlite_master WHERE type IN ('table','index','trigger') AND sql IS NOT NULL`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var name, definition string
		if err = rows.Scan(&name, &definition); err != nil {
			return err
		}
		if want, ok := expected[name]; ok {
			if strings.Join(strings.Fields(definition), " ") != want {
				return nativeErr("unsupported_schema", "SQLite definition differs from the current schema: "+name)
			}
			delete(expected, name)
		}
	}
	if err = rows.Err(); err != nil {
		return err
	}
	rows.Close()
	if len(expected) != 0 {
		return nativeErr("unsupported_schema", "SQLite database lacks current tables, indexes or constraints")
	}
	var databaseID string
	var revision uint64
	if err = db.QueryRowContext(ctx, `SELECT database_id,revision FROM database_state WHERE singleton=1`).Scan(&databaseID, &revision); err != nil || len(databaseID) != 32 {
		return nativeErr("unsupported_schema", "SQLite database sampling identity is missing or invalid")
	}
	return nil
}
