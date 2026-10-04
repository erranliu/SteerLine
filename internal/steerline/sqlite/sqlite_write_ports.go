package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/steerline/persistence"
	tasklegacy "github.com/erranliu/SteerLine/internal/steerline/task"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

var _ persistence.Projects = (*SQLiteStore)(nil)

var _ persistence.CommandStore = (*SQLiteStore)(nil)

var _ persistence.WorkerMetadata = (*SQLiteStore)(nil)

var _ persistence.Conversations = (*SQLiteStore)(nil)

func persistenceError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return errors.Join(storageports.ErrNotFound, err)
	}
	return storageFailure(err, storageports.StorageIO)
}

func (s *SQLiteStore) EnqueueCommand(ctx context.Context, v storageports.CommandRecord) (storageports.CommandRecord, bool, error) {
	var result storageports.CommandRecord
	var created bool
	err := s.transactSQLite(ctx, func(tx *sqliteTx) error { var e error; result, created, e = tx.EnqueueCommand(v); return e })
	return result, created, err
}

func (s *SQLiteStore) AcceptDecisionAnswer(ctx context.Context, command storageports.CommandRecord, answer executionmodel.DecisionAnswer) (executionmodel.DecisionRecord, bool, error) {
	var result executionmodel.DecisionRecord
	var accepted bool
	err := s.transactSQLite(ctx, func(tx *sqliteTx) error {
		var e error
		result, accepted, e = tx.AcceptDecisionAnswer(command, answer)
		return e
	})
	return result, accepted, err
}

func (s *SQLiteStore) UpdateCommand(ctx context.Context, v storageports.CommandRecord) error {
	return s.transactSQLite(ctx, func(tx *sqliteTx) error { return tx.UpdateCommand(v) })
}

func (s *SQLiteStore) UpdateDiscardReceipt(ctx context.Context, v executionmodel.OperationRecord) error {
	if v.Kind != "discard_task" {
		return errors.New("discard receipt requires its fixed discard operation")
	}
	return s.transactSQLite(ctx, func(tx *sqliteTx) error { return tx.UpdateOperation(v) })
}

func (s *SQLiteStore) Heartbeat(ctx context.Context, project string) error {
	return s.transactSQLite(ctx, func(tx *sqliteTx) error { return tx.heartbeatProjection(project) })
}

func (tx *sqliteTx) Query(ctx context.Context, project string) (DomainSnapshot, error) {
	return QuerySnapshot(ctx, tx.tx, project)
}

func (s *SQLiteStore) WorkflowOwned(ctx context.Context, project, workflow, task string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM workflow_runs WHERE project_identity=? AND workflow_run_id=? AND task_id=?`, project, workflow, task).Scan(&count)
	return count == 1, err
}

func (tx *sqliteTx) OccupiedExecutions(ctx context.Context, project string) (int, error) {
	var count int
	err := tx.tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM executions e JOIN workflow_runs w ON w.project_identity=e.project_identity AND w.workflow_run_id=e.workflow_run_id WHERE e.project_identity=? AND e.status IN ('pending','active','waiting_decision','blocked')`, project).Scan(&count)
	return count, err
}

func (tx *sqliteTx) DeliveryCount(ctx context.Context, project, task string) (int, error) {
	var count int
	err := tx.tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM task_deliveries WHERE project_identity=? AND task_id=?`, project, task).Scan(&count)
	return count, err
}

func (tx *sqliteTx) PRBindingsOnOtherTasks(ctx context.Context, project, task, url string) (int, error) {
	var count int
	err := tx.tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM workflow_runs w WHERE project_identity=? AND task_id<>? AND (json_extract(payload,'$.pr_url')=? OR EXISTS (SELECT 1 FROM json_each(w.payload,'$.pr_history') h WHERE h.value=?))`, project, task, url, url).Scan(&count)
	return count, err
}

func (tx *sqliteTx) Task(ctx context.Context, project, id string) (taskmodel.TaskRecord, error) {
	var payload []byte
	var version uint64
	var status string
	var v taskmodel.TaskRecord
	err := tx.tx.QueryRowContext(ctx, `SELECT payload,status,version FROM tasks WHERE project_identity=? AND task_id=?`, project, id).Scan(&payload, &status, &version)
	if err != nil {
		return v, persistenceError(err)
	}
	if err = json.Unmarshal(payload, &v); err != nil {
		return v, err
	}
	if v.ProjectIdentity != project || v.TaskID != id || v.Status != status || v.Version != version {
		return v, nativeErr("authority_mismatch", "task row and payload identity differ")
	}
	return v, nil
}

func (tx *sqliteTx) Workflow(ctx context.Context, project, id, task string) (executionmodel.WorkflowRunRecord, error) {
	var payload []byte
	var v executionmodel.WorkflowRunRecord
	err := tx.tx.QueryRowContext(ctx, `SELECT payload FROM workflow_runs WHERE project_identity=? AND workflow_run_id=? AND task_id=?`, project, id, task).Scan(&payload)
	if err != nil {
		return v, persistenceError(err)
	}
	if err = json.Unmarshal(payload, &v); err != nil {
		return v, err
	}
	if v.ProjectIdentity != project || v.WorkflowRunID != id || v.TaskID != task {
		return v, nativeErr("authority_mismatch", "workflow payload differs from its fixed row identity")
	}
	return v, nil
}

func (tx *sqliteTx) Execution(ctx context.Context, project, id, workflow string) (executionmodel.ExecutionRecord, error) {
	var payload []byte
	var v executionmodel.ExecutionRecord
	err := tx.tx.QueryRowContext(ctx, `SELECT payload FROM executions WHERE project_identity=? AND execution_id=? AND workflow_run_id=?`, project, id, workflow).Scan(&payload)
	if err != nil {
		return v, persistenceError(err)
	}
	if err = json.Unmarshal(payload, &v); err != nil {
		return v, err
	}
	if v.ProjectIdentity != project || v.ExecutionID != id || v.WorkflowRunID != workflow {
		return v, nativeErr("authority_mismatch", "execution payload differs from its fixed row identity")
	}
	return v, nil
}

func (tx *sqliteTx) Attempt(ctx context.Context, project, id, execution string) (executionmodel.StepAttemptRecord, error) {
	var payload []byte
	var v executionmodel.StepAttemptRecord
	err := tx.tx.QueryRowContext(ctx, `SELECT payload FROM step_attempts WHERE project_identity=? AND attempt_id=? AND execution_id=?`, project, id, execution).Scan(&payload)
	if err != nil {
		return v, persistenceError(err)
	}
	if err = json.Unmarshal(payload, &v); err != nil {
		return v, err
	}
	if v.ProjectIdentity != project || v.AttemptID != id || v.ExecutionID != execution {
		return v, nativeErr("authority_mismatch", "attempt payload differs from its fixed row identity")
	}
	return v, nil
}

func (tx *sqliteTx) UpdateDiscardedTaskCAS(v taskmodel.TaskRecord, expected uint64) error {
	if v.Status != "discarded" || v.Source != "native" || expected == 0 || v.Version != expected+1 {
		return nativeErr("contract_mismatch", "cleanup requires the next discarded task version")
	}
	old, err := tx.Task(tx.ctx, v.ProjectIdentity, v.TaskID)
	if err != nil {
		return err
	}
	if old.Status != "discarded" || old.Version != expected {
		return nativeErr("stale_version", "discarded task changed during cleanup receipt")
	}
	beforeCleanup := append([]string(nil), old.CleanupPending...)
	old.CleanupPending, old.Version, old.UpdatedAt = v.CleanupPending, v.Version, v.UpdatedAt
	if !sameTaskRecord(old, v) || len(beforeCleanup) != len(v.CleanupPending)+1 {
		return nativeErr("contract_mismatch", "cleanup may only settle one pending item")
	}
	removed := false
	for i := range beforeCleanup {
		candidate := append(append([]string(nil), beforeCleanup[:i]...), beforeCleanup[i+1:]...)
		if slices.Equal(candidate, v.CleanupPending) {
			removed = true
			break
		}
	}
	if !removed {
		return nativeErr("contract_mismatch", "cleanup cannot replace its pending resource identities")
	}

	payload, err := json.Marshal(v)
	if err != nil {
		return err
	}
	result, err := tx.tx.ExecContext(tx.ctx, `UPDATE tasks SET version=?,payload=? WHERE project_identity=? AND task_id=? AND status='discarded' AND version=?`, v.Version, payload, v.ProjectIdentity, v.TaskID, expected)
	return requireIdentityWrite(result, err, "stale_version: discarded task changed during cleanup receipt")
}

func (s *SQLiteStore) TransactTaskBatch(ctx context.Context, fn func(persistence.BatchTx) error) error {
	if fn == nil {
		return errors.New("transaction callback is required")
	}
	return s.transactSQLite(ctx, func(tx *sqliteTx) error { return fn(tx) })
}

func (s *SQLiteStore) TransactClaim(ctx context.Context, fn func(persistence.ClaimTx) error) error {
	if fn == nil {
		return errors.New("transaction callback is required")
	}
	return s.transactSQLite(ctx, func(tx *sqliteTx) error { return fn(tx) })
}

func (s *SQLiteStore) TransactCandidate(ctx context.Context, fn func(persistence.CandidateTx) error) error {
	if fn == nil {
		return errors.New("transaction callback is required")
	}
	return s.transactSQLite(ctx, func(tx *sqliteTx) error { return fn(tx) })
}

func (s *SQLiteStore) TransactDispatch(ctx context.Context, fn func(persistence.DispatchTx) error) error {
	if fn == nil {
		return errors.New("transaction callback is required")
	}
	return s.transactSQLite(ctx, func(tx *sqliteTx) error { return fn(tx) })
}

func (s *SQLiteStore) TransactPredecessors(ctx context.Context, fn func(persistence.PredecessorTx) error) error {
	if fn == nil {
		return errors.New("transaction callback is required")
	}
	return s.transactSQLite(ctx, func(tx *sqliteTx) error { return fn(tx) })
}

func (s *SQLiteStore) TransactDiscardIntent(ctx context.Context, fn func(persistence.DiscardIntentTx) error) error {
	if fn == nil {
		return errors.New("transaction callback is required")
	}
	return s.transactSQLite(ctx, func(tx *sqliteTx) error { return fn(tx) })
}

func (s *SQLiteStore) TransactDiscardPending(ctx context.Context, fn func(persistence.DiscardPendingTx) error) error {
	if fn == nil {
		return errors.New("transaction callback is required")
	}
	return s.transactSQLite(ctx, func(tx *sqliteTx) error { return fn(tx) })
}

func (s *SQLiteStore) TransactDiscard(ctx context.Context, fn func(persistence.DiscardTx) error) error {
	if fn == nil {
		return errors.New("transaction callback is required")
	}
	return s.transactSQLite(ctx, func(tx *sqliteTx) error { return fn(tx) })
}

func (s *SQLiteStore) TransactDelivery(ctx context.Context, fn func(persistence.DeliveryTx) error) error {
	if fn == nil {
		return errors.New("transaction callback is required")
	}
	return s.transactSQLite(ctx, func(tx *sqliteTx) error { return fn(tx) })
}

func (s *SQLiteStore) TransactCleanupProgress(ctx context.Context, fn func(persistence.CleanupProgressTx) error) error {
	if fn == nil {
		return errors.New("transaction callback is required")
	}
	return s.transactSQLite(ctx, func(tx *sqliteTx) error { return fn(tx) })
}

func (s *SQLiteStore) TransactCleanup(ctx context.Context, fn func(persistence.CleanupTx) error) error {
	if fn == nil {
		return errors.New("transaction callback is required")
	}
	return s.transactSQLite(ctx, func(tx *sqliteTx) error { return fn(tx) })
}

func (s *SQLiteStore) InitializeProject(ctx context.Context, authority tasklegacy.ProjectTaskAuthority) error {
	err := s.transactSQLite(ctx, func(tx *sqliteTx) error {
		var count int
		if err := tx.tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks`).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return nativeErr("authority_mismatch", "clean native initialization found existing task records")
		}
		return tx.PutProjectTaskAuthority(authority)
	})
	if err != nil {
		s.abortNewDatabase()
	} else {
		s.createdNew = false
	}
	return err
}
