package sqlite

import (
	"database/sql"
	"encoding/json"
	"errors"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

func (t *sqliteTx) putExecutionBinding(ref executionbackend.ExecutionRef) error {
	if err := executionbackend.ValidateExecutionRef(ref); err != nil {
		return err
	}
	var body []byte
	err := t.tx.QueryRowContext(t.ctx, `SELECT payload FROM execution_bindings WHERE run_id=? AND attempt_id=? AND record_id='binding'`, ref.RunID, ref.AttemptID).Scan(&body)
	if err == nil {
		var old executionbackend.ExecutionRef
		if err := json.Unmarshal(body, &old); err != nil {
			return err
		}
		if digestValue(old) != digestValue(ref) {
			return errors.New("execution binding is immutable and conflicts with existing record")
		}
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	body, err = json.Marshal(ref)
	if err != nil {
		return err
	}
	_, err = t.tx.ExecContext(t.ctx, `INSERT INTO execution_bindings(run_id,attempt_id,record_id,payload) VALUES(?,?,'binding',?)`, ref.RunID, ref.AttemptID, body)
	return err
}

func (t *sqliteTx) putExecutionOperation(op executionbackend.Operation, expected string) error {
	if err := executionbackend.ValidateExecutionOperation(op); err != nil {
		return err
	}
	var body []byte
	err := t.tx.QueryRowContext(t.ctx, `SELECT payload FROM execution_operations WHERE run_id=? AND attempt_id=? AND record_id=?`, op.RunID, op.AttemptID, op.ID).Scan(&body)
	if err == nil {
		var old executionbackend.Operation
		if err := json.Unmarshal(body, &old); err != nil {
			return err
		}
		if expected != recordFingerprint(old) {
			return nativeErr("stale_revision", "execution operation changed before commit")
		}
		if old.PayloadDigest != op.PayloadDigest || old.Action != op.Action || old.Backend != op.Backend || old.Protocol != op.Protocol || old.Target != op.Target || digestValue(old.Ref) != digestValue(op.Ref) {
			return errors.New("execution operation fixed identity changed")
		}
		if old.Status != op.Status && !executionmodel.OperationTransitionAllowed(string(old.Status), string(op.Status)) {
			return errors.New("execution operation status cannot regress")
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	} else if expected != "" {
		return nativeErr("stale_revision", "execution operation disappeared before commit")
	}
	body, err = json.Marshal(op)
	if err != nil {
		return err
	}
	_, err = t.tx.ExecContext(t.ctx, `INSERT INTO execution_operations(run_id,attempt_id,record_id,payload) VALUES(?,?,?,?) ON CONFLICT(run_id,attempt_id,record_id) DO UPDATE SET payload=excluded.payload`, op.RunID, op.AttemptID, op.ID, body)
	return err
}
