package sqlite

import (
	"context"
	"fmt"
	"strings"

	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

// OpenSQLiteDiscardReader allows compatible older layouts only for explicit
// discard. The returned store can inspect the source but cannot write it.
func OpenSQLiteDiscardReader(path string) (_ *SQLiteStore, retErr error) {
	defer func() { retErr = storageFailure(retErr, storageports.StorageIO) }()
	reader, err := openSQLiteInspector(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if retErr != nil {
			_ = reader.Close()
		}
	}()

	ctx := context.Background()
	version, err := readDatabaseSchemaVersion(ctx, reader.db)
	if err != nil {
		return nil, err
	}
	if version <= 0 || version > domainSchemaVersion {
		return nil, nativeErr("unsupported_schema", fmt.Sprintf("unsupported SQLite schema %d for discard-runs", version))
	}
	// Only the tables used to reconstruct tasks and prove shutdown ownership
	// must match. Other tables and indexes remain in the archived database.
	var definitions []string
	for _, schema := range [][]string{taskServiceSchema, nativeTaskSchema} {
		for _, stmt := range schema {
			if version == domainSchemaVersion {
				definitions = append(definitions, stmt)
				continue
			}
			rest, table := strings.CutPrefix(stmt, "CREATE TABLE ")
			if !table {
				continue
			}
			name, _, _ := strings.Cut(rest, " ")
			switch name {
			case "project_task_authorities", "database_state", "tasks", "task_contracts", "task_dependencies",
				"task_deliveries", "runner_candidates", "commands", "projection", "roadmaps", "advisory_jobs", "project_environments":
				definitions = append(definitions, stmt)
			}
		}
	}
	if err := validateDatabaseLayout(ctx, reader.db, definitions); err != nil {
		return nil, err
	}
	return reader, nil
}
