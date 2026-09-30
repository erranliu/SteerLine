package steerline

import (
	"database/sql"
	"encoding/json"
	"errors"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

func (t *sqliteTx) putExecutionBinding(ref executionapi.ExecutionRef) error {
	if err := validateExecutionRef(ref); err != nil {
		return err
	}
	var body []byte
	err := t.tx.QueryRowContext(t.ctx, `SELECT payload FROM execution_bindings WHERE run_id=? AND attempt_id=? AND record_id='binding'`, ref.RunID, ref.AttemptID).Scan(&body)
	if err == nil {
		var old executionapi.ExecutionRef
		if err := json.Unmarshal(body, &old); err != nil {
			return err
		}
		old.WorkspacePath, ref.WorkspacePath = canonical(old.WorkspacePath), canonical(ref.WorkspacePath)
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

func (t *sqliteTx) putExecutionOperation(op executionapi.Operation, expected string) error {
	if err := validateExecutionOperation(op); err != nil {
		return err
	}
	var body []byte
	err := t.tx.QueryRowContext(t.ctx, `SELECT payload FROM execution_operations WHERE run_id=? AND attempt_id=? AND record_id=?`, op.RunID, op.AttemptID, op.ID).Scan(&body)
	if err == nil {
		var old executionapi.Operation
		if err := json.Unmarshal(body, &old); err != nil {
			return err
		}
		if expected != recordFingerprint(old) {
			return nativeErr("stale_revision", "execution operation changed before commit")
		}
		if old.PayloadDigest != op.PayloadDigest || old.Action != op.Action || old.Backend != op.Backend || old.Protocol != op.Protocol || old.Target != op.Target || digestValue(old.Ref) != digestValue(op.Ref) {
			return errors.New("execution operation fixed identity changed")
		}
		if old.Status != op.Status && !allowedOperationTransitions[string(old.Status)][string(op.Status)] {
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

func (s *Service) commitWorkerBinding(run *Run, ref executionapi.ExecutionRef) error {
	return s.commitWorkerAttached(run, workerCommitAttachment{Binding: &ref})
}
func (s *Service) commitExecutionOperation(run *Run, op executionapi.Operation, expected string) error {
	var before *executionapi.Operation
	if expected != "" {
		saved, err := (executionRecords{root: s.Config.RunDir}).loadOperation(op.RunID, op.AttemptID, op.ID)
		if err != nil {
			return err
		}
		if recordFingerprint(saved) != expected {
			return nativeErr("stale_revision", "execution operation changed before commitment")
		}
		before = &saved
	}
	return s.commitWorkerAttached(run, workerCommitAttachment{ExecutionOperation: &recordWrite[executionapi.Operation]{Before: before, After: op}})
}
