package steerline

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

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
	reader, err := openSQLiteInspector(databasePath)
	if err != nil {
		return nativeErr("unsupported_schema", fmt.Sprintf("cannot inspect existing SQLite state without mutation: %v", err))
	}
	defer reader.Close()
	return validateDatabaseSchema(context.Background(), reader.db)
}

type schemaQuery interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

// Check definitions as well as the version stamp before reading or writing state.
func validateDatabaseSchema(ctx context.Context, db schemaQuery) error {
	var version int
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&version); err != nil {
		return nativeErr("unsupported_schema", "existing SQLite database has an unknown schema; preserve it for manual recovery")
	}
	if version != domainSchemaVersion {
		return nativeErr("unsupported_schema", fmt.Sprintf("unsupported SQLite schema %d; only schema %d is accepted", version, domainSchemaVersion))
	}
	expected := map[string]string{}
	definitions := append(append(append([]string{}, domainExecutionSchema...), nativeTaskSchema...), conversationSchema...)
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

// preflightWorkerFlowState is read-only and runs before workspace preparation.
// A previous schema may represent a still-running external worker;
// it must not be converted or overwritten on startup.
func preflightWorkerFlowState(ctx context.Context, c Config) error {
	if c.WorkflowProtocol != WorkerFlowVersion {
		return nil
	}
	path := filepath.Join(c.RunDir, "steerline.db")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	reader, err := OpenNativeTaskReader(ctx, &c)
	if err != nil {
		return err
	}
	defer reader.Close()
	payload, _, err := reader.LoadProjection(ctx, c.Identity())
	if err != nil {
		return err
	}
	if len(payload) != 0 {
		var runtime ServiceCheckpoint
		if err := decode(payload, &runtime); err != nil || runtime.SchemaVersion != WorkerFlowRuntimeSchema {
			return nativeErr("unsupported_schema", "worker-flow-v1 refuses the old runtime projection; preserve the original state and external session")
		}
	}
	snapshot, err := reader.Query(ctx, c.Identity())
	if err != nil {
		return err
	}
	for _, workflow := range snapshot.WorkflowRuns {
		if workflow.WorkflowVersion != WorkerFlowVersion {
			return nativeErr("unsupported_schema", "worker-flow-v1 refuses an old WorkflowRun; preserve the original state and external session")
		}
	}
	for _, execution := range snapshot.Executions {
		if execution.Backend == string(BackendLocal) && execution.Protocol != string(ProtocolLocalCodexAppServer) {
			return nativeErr("unsupported_schema", "worker-flow-v1 refuses an old local execution protocol; preserve its external process and state")
		}
	}
	return nil
}
