package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

func validateRetiredWholeRunResultCommands(ctx context.Context, db *sql.DB) error {
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM commands WHERE kind='accept_result'`).Scan(&count); err != nil {
		return schemaFailure(err, "cannot inspect retired whole-run result commands without mutation")
	}
	if count > 0 {
		return nativeErr("unsupported_schema", "database contains retired whole-run result commands; preserve it for manual recovery")
	}
	return nil
}

func validateExistingDatabase(databasePath string) error {
	reader, err := openSQLiteInspector(databasePath)
	if err != nil {
		return schemaFailure(err, fmt.Sprintf("cannot inspect existing SQLite state without mutation: %v", err))
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
		return schemaFailure(err, "existing SQLite database has an unknown schema; preserve it for manual recovery")
	}
	if version != domainSchemaVersion {
		return nativeErr("unsupported_schema", fmt.Sprintf("unsupported SQLite schema %d; only schema %d is accepted", version, domainSchemaVersion))
	}
	expected := map[string]string{}
	definitions := append(append([]string{}, taskServiceSchema...), nativeTaskSchema...)
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
		if name == "one_active_workflow_per_task" {
			return nativeErr("unsupported_schema", "SQLite retains the retired single-candidate constraint; preserve it")
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
	if err = db.QueryRowContext(ctx, `SELECT database_id,revision FROM database_state WHERE singleton=1`).Scan(&databaseID, &revision); err != nil {
		return schemaFailure(err, "SQLite database sampling identity is missing or invalid")
	}
	if len(databaseID) != 32 {
		return nativeErr("unsupported_schema", "SQLite database sampling identity is missing or invalid")
	}
	return nil
}
