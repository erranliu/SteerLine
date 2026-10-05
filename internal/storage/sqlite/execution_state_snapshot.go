package sqlite

import (
	"context"
	"database/sql"

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
	graph, err := s.TaskGraphSnapshotTx(ctx, tx, project)
	if err != nil {
		return storageports.ExecutionStateSnapshot{}, err
	}
	records, err := QuerySnapshot(ctx, tx, project)
	if err != nil {
		return storageports.ExecutionStateSnapshot{}, err
	}
	if err := tx.Commit(); err != nil {
		return storageports.ExecutionStateSnapshot{}, err
	}
	return storageports.ExecutionStateSnapshot{Graph: graph, Records: records}, nil
}
