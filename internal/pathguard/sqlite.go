package pathguard

import "fmt"

// RejectSharedSQLiteFiles checks existing database and SQLite sidecar files
// before opening even a reader: SQLite can write WAL shared memory on reads.
// Missing files are allowed and created normally by SQLite after admission.
func RejectSharedSQLiteFiles(database string) error {
	for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
		path := database + suffix
		links, err := fileLinkCount(path)
		if err != nil {
			return fmt.Errorf("inspect SQLite state file %s: %w", path, err)
		}
		if links > 1 {
			return fmt.Errorf("SQLite state file must not be a hard link (%d links): %s", links, path)
		}
	}
	return nil
}
