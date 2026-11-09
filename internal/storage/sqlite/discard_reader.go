package sqlite

// Explicit discard still reads only the current schema. Retired development
// formats require their original build for resource settlement; no migration.
func OpenSQLiteDiscardReader(path string) (*SQLiteStore, error) { return OpenSQLiteReader(path) }
