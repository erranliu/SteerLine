package sqlite

import (
	"context"
	"strings"
)

// OpenSQLiteDiscardReader is exclusively for the explicit discard-runs goal.
// Schema 40 has the same task contracts and Runner ownership records as 41.
// It is read only for archival/recreation, never opened for resumed execution
// or migrated in place. Normal readers and writers still require schema 41.
func OpenSQLiteDiscardReader(path string) (*SQLiteStore, error) {
	reader, err := openSQLiteInspector(path)
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	var version int
	err = reader.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&version)
	if err == nil && version == 40 && SchemaVersion == 41 {
		err = validateDatabaseDefinitions(ctx, reader.db, 40, discardSchema40())
	} else if err == nil {
		err = validateDatabaseSchema(ctx, reader.db)
	}
	if err != nil {
		_ = reader.Close()
		return nil, schemaFailure(err, "cannot inspect database for explicit discard; preserve original state")
	}
	return reader, nil
}

// The only differences introduced by schema 41 are observation/preparation
// storage and activity indexes; none are imported into the replacement DB.
func discardSchema40() []string {
	added := map[string]bool{
		"source_observations": true, "preparation_batches": true, "preparation_associations": true,
		"preparation_created": true, "candidate_activity_created": true,
		"environment_activity_created": true, "explicit_check_created": true,
	}
	definitions := append(append([]string{}, taskServiceSchema...), nativeTaskSchema...)
	var original []string
	for _, stmt := range definitions {
		fields := strings.Fields(stmt)
		if len(fields) >= 3 && fields[0] == "CREATE" {
			if added[fields[2]] {
				continue
			}
			if fields[2] == "maintenance_settings" {
				stmt = `CREATE TABLE maintenance_settings (project_identity TEXT PRIMARY KEY, profile TEXT NOT NULL, observation BLOB, generation_batch BLOB)`
			}
		}
		original = append(original, stmt)
	}
	return original
}
