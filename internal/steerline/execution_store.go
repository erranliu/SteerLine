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

func (r executionRecords) writeRecord(table, run, attempt, id string, payload []byte) error {
	s, err := OpenSQLiteStore(filepath.Join(r.root, "steerline.db"))
	if err != nil {
		return err
	}
	defer s.Close()
	_, err = s.db.Exec("INSERT INTO "+table+"(run_id,attempt_id,record_id,payload) VALUES(?,?,?,?) ON CONFLICT(run_id,attempt_id,record_id) DO UPDATE SET payload=excluded.payload", run, attempt, id, payload)
	return err
}

func (r executionRecords) attemptOperations(run, attempt string) ([]Operation, error) {
	s, err := OpenSQLiteReader(filepath.Join(r.root, "steerline.db"))
	if err != nil {
		return nil, err
	}
	defer s.Close()
	rows, err := s.db.Query("SELECT record_id FROM execution_operations WHERE run_id=? AND attempt_id=? ORDER BY record_id", run, attempt)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	var result []Operation
	for _, id := range ids {
		op, err := r.loadOperation(run, attempt, id)
		if err != nil {
			return nil, err
		}
		result = append(result, op)
	}
	return result, nil
}
