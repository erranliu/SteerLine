package sqlite

import (
	"bytes"
	"context"
	"encoding/json"

	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func (s *SQLiteStore) TaskContract(ctx context.Context, project string, task taskmodel.TaskRecord) (taskmodel.TaskContract, error) {
	var body []byte
	var digest string
	err := s.db.QueryRowContext(ctx, `SELECT digest,payload FROM task_contracts WHERE project_identity=? AND contract_id=? AND task_id=?`, project, task.ContractID, task.TaskID).Scan(&digest, &body)
	if err != nil {
		return taskmodel.TaskContract{}, requiredRecordError(err, "contract_mismatch", "native task has no matching fixed contract")
	}
	var contract taskmodel.TaskContract
	if err = json.Unmarshal(body, &contract); err != nil {
		return contract, err
	}
	if contract.Digest != digest {
		return contract, nativeErr("contract_mismatch", "fixed task contract digest differs from its stored row")
	}
	if err = taskmodel.ValidateContract(contract); err != nil {
		return contract, err
	}
	return contract, nil
}

func (t *sqliteTx) BumpOpenTaskVersionCAS(next taskmodel.TaskRecord, expectedVersion uint64) error {
	if next.Status != "open" || next.Claimant != "" || next.Version != expectedVersion+1 {
		return nativeErr("contract_mismatch", "predecessor addition requires an unclaimed open task")
	}
	var oldPayload []byte
	if err := t.tx.QueryRowContext(t.ctx, `SELECT payload FROM tasks WHERE project_identity=? AND task_id=? AND status='open' AND version=?`, next.ProjectIdentity, next.TaskID, expectedVersion).Scan(&oldPayload); err != nil {
		return requiredRecordError(err, "stale_version", "task changed before predecessor addition")
	}
	var old taskmodel.TaskRecord
	if err := json.Unmarshal(oldPayload, &old); err != nil {
		return err
	}
	old.Version, old.UpdatedAt = next.Version, next.UpdatedAt
	if !sameTaskRecord(old, next) {
		return nativeErr("contract_mismatch", "predecessor addition cannot alter the task contract or identity")
	}
	var active, delivered int
	if err := t.tx.QueryRowContext(t.ctx, `SELECT COUNT(*) FROM runner_candidates WHERE project_identity=? AND task_id=? AND json_extract(payload,'$.participation')='active'`, next.ProjectIdentity, next.TaskID).Scan(&active); err != nil {
		return err
	}
	if err := t.tx.QueryRowContext(t.ctx, `SELECT COUNT(*) FROM task_deliveries WHERE project_identity=? AND task_id=?`, next.ProjectIdentity, next.TaskID).Scan(&delivered); err != nil {
		return err
	}
	if active != 0 || delivered != 0 {
		return nativeErr("inconsistent_delivery", "open task still has an active workflow or delivery")
	}
	body, err := json.Marshal(next)
	if err != nil {
		return err
	}
	result, err := t.tx.ExecContext(t.ctx, `UPDATE tasks SET version=?,payload=? WHERE project_identity=? AND task_id=? AND status='open' AND version=?`, next.Version, body, next.ProjectIdentity, next.TaskID, expectedVersion)
	return requireIdentityWrite(result, err, "stale_version: task changed before predecessor addition")
}

func sameTaskRecord(a, b taskmodel.TaskRecord) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return bytes.Equal(x, y)
}

func (tx *sqliteTx) TaskContract(ctx context.Context, project string, task taskmodel.TaskRecord) (taskmodel.TaskContract, error) {
	var body []byte
	var digest string
	err := tx.tx.QueryRowContext(ctx, `SELECT digest,payload FROM task_contracts WHERE project_identity=? AND contract_id=? AND task_id=?`, project, task.ContractID, task.TaskID).Scan(&digest, &body)
	if err != nil {
		return taskmodel.TaskContract{}, requiredRecordError(err, "contract_mismatch", "native task has no matching fixed contract")
	}
	var contract taskmodel.TaskContract
	if err = json.Unmarshal(body, &contract); err != nil {
		return contract, err
	}
	if contract.Digest != digest {
		return contract, nativeErr("contract_mismatch", "fixed task contract digest differs from its stored row")
	}
	if err = taskmodel.ValidateContract(contract); err != nil {
		return contract, err
	}
	return contract, nil
}
