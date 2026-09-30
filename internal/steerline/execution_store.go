package steerline

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
)

// Adapter bindings and side-effect receipts share the authoritative database.
// Files under execution/v1 contain capability evidence only, never recovery state.
func (r executionRecords) readRecord(table, run, attempt, id string) ([]byte, error) {
	s, err := OpenSQLiteReader(filepath.Join(r.root, "steerline.db"))
	if err != nil {
		if _, statErr := os.Stat(filepath.Join(r.root, "steerline.db")); os.IsNotExist(statErr) {
			return nil, os.ErrNotExist
		}
		return nil, err
	}
	defer s.Close()
	var payload []byte
	err = s.db.QueryRow("SELECT payload FROM "+table+" WHERE run_id=? AND attempt_id=? AND record_id=?", run, attempt, id).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, os.ErrNotExist
	}
	return payload, err
}
