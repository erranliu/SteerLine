package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

// These helpers seed typed binding/operation rows explicitly in test fixtures.
func (r executionRecords) saveBinding(ref executionapi.ExecutionRef) error {
	if err := validateExecutionRef(ref); err != nil {
		return err
	}
	ref.WorkspacePath = canonical(ref.WorkspacePath)
	if old, err := r.loadBinding(ref.RunID, ref.AttemptID); err == nil {
		old.WorkspacePath = canonical(old.WorkspacePath)
		if digestValue(old) != digestValue(ref) {
			return errors.New("execution binding is immutable and conflicts with existing record")
		}
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	b, err := json.MarshalIndent(ref, "", "  ")
	if err != nil {
		return err
	}
	return r.writeRecord("execution_bindings", ref.RunID, ref.AttemptID, "binding", b)
}
func (r executionRecords) saveOperation(op executionapi.Operation) error {
	if err := validateExecutionOperation(op); err != nil {
		return err
	}
	if old, err := r.loadOperation(op.RunID, op.AttemptID, op.ID); err == nil {
		if old.PayloadDigest != op.PayloadDigest || old.Action != op.Action || old.Backend != op.Backend || old.Protocol != op.Protocol || old.Target != op.Target {
			return errors.New("operation id reused with different fixed payload")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	b, err := json.MarshalIndent(op, "", "  ")
	if err != nil {
		return err
	}
	return r.writeRecord("execution_operations", op.RunID, op.AttemptID, op.ID, b)
}
func (r executionRecords) writeRecord(table, run, attempt, id string, payload []byte) error {
	s, err := OpenSQLiteStore(filepath.Join(r.root, "steerline.db"))
	if err != nil {
		return err
	}
	defer s.Close()
	return s.transactSQLite(context.Background(), func(tx *sqliteTx) error {
		_, err := tx.tx.ExecContext(tx.ctx, "INSERT INTO "+table+"(run_id,attempt_id,record_id,payload) VALUES(?,?,?,?) ON CONFLICT(run_id,attempt_id,record_id) DO UPDATE SET payload=excluded.payload", run, attempt, id, payload)
		return err
	})
}
