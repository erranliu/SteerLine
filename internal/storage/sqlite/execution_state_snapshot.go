package sqlite

import (
	"context"
	"database/sql"
	"errors"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

// ReadExecutionStateSnapshot pins the actor's task authority and execution
// records to one database revision for restart reconstruction.
func (s Reader) ReadExecutionStateSnapshot(ctx context.Context, project string) (storageports.ExecutionStateSnapshot, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return storageports.ExecutionStateSnapshot{}, err
	}
	defer tx.Rollback()
	snapshot, err := readExecutionStateSnapshotTx(ctx, tx, project, s.nativeBinding, nil)
	if err != nil {
		return storageports.ExecutionStateSnapshot{}, err
	}
	if err := tx.Commit(); err != nil {
		return storageports.ExecutionStateSnapshot{}, err
	}
	return snapshot, nil
}

func readExecutionStateSnapshotTx(ctx context.Context, tx graphSnapshotQuery, project string, binding *executionmodel.ProjectTaskAuthority, afterCheckpoint func() error) (storageports.ExecutionStateSnapshot, error) {
	var checkpoint *storageports.ServiceCheckpoint
	checkpoint, err := readServiceCheckpoint(ctx, tx, project, true)
	if errors.Is(err, sql.ErrNoRows) {
		checkpoint, err = nil, nil
	}
	if err != nil {
		return storageports.ExecutionStateSnapshot{}, persistenceError(err)
	}
	if afterCheckpoint != nil {
		if err := afterCheckpoint(); err != nil {
			return storageports.ExecutionStateSnapshot{}, err
		}
	}
	graph, err := TaskGraphSnapshotTx(ctx, tx, project, binding)
	if err != nil {
		return storageports.ExecutionStateSnapshot{}, err
	}
	records, err := QuerySnapshot(ctx, tx, project)
	if err != nil {
		return storageports.ExecutionStateSnapshot{}, err
	}
	return storageports.ExecutionStateSnapshot{Graph: graph, Records: records, Checkpoint: checkpoint}, nil
}
