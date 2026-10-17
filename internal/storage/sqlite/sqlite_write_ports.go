package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

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

func (s *SQLiteStore) UpdateCommand(ctx context.Context, v storageports.CommandRecord) error {
	return s.transactSQLite(ctx, func(tx *sqliteTx) error { return tx.UpdateCommand(v) })
}

func (s *SQLiteStore) Heartbeat(ctx context.Context, project string) error {
	return s.transactSQLite(ctx, func(tx *sqliteTx) error { return tx.heartbeatProjection(project) })
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

func (s *SQLiteStore) TransactTaskBatch(ctx context.Context, fn func(storageports.BatchTx) error) error {
	if fn == nil {
		return errors.New("transaction callback is required")
	}
	return s.transactSQLite(ctx, func(tx *sqliteTx) error { return fn(tx) })
}

func (s *SQLiteStore) TransactPredecessors(ctx context.Context, fn func(storageports.PredecessorTx) error) error {
	if fn == nil {
		return errors.New("transaction callback is required")
	}
	return s.transactSQLite(ctx, func(tx *sqliteTx) error { return fn(tx) })
}

func (s *SQLiteStore) InitializeProject(ctx context.Context, authority taskmodel.ProjectTaskAuthority) error {
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
