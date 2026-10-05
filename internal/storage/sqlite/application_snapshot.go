package sqlite

import (
	"context"
	"database/sql"
	"errors"

	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

// ReadApplicationProjectSnapshot samples the product view and service/runtime
// facts in one read transaction. Runtime reconstruction remains with the app.
func (s Reader) ReadApplicationProjectSnapshot(ctx context.Context, project, selectedTaskID string, limit int) (storageports.ApplicationProjectSnapshot, error) {
	if s.db == nil {
		return storageports.ApplicationProjectSnapshot{}, errors.New("application project reader is required")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return storageports.ApplicationProjectSnapshot{}, err
	}
	defer tx.Rollback()
	snapshot, err := readApplicationProjectSnapshotTx(ctx, tx, s, project, selectedTaskID, limit, nil)
	if err != nil {
		return storageports.ApplicationProjectSnapshot{}, err
	}
	if err := tx.Commit(); err != nil {
		return storageports.ApplicationProjectSnapshot{}, err
	}
	return snapshot, nil
}

// afterProject is a deterministic WAL interleaving seam used by this package's
// transaction test; production passes nil.
func readApplicationProjectSnapshotTx(ctx context.Context, tx *sql.Tx, reader Reader, project, selectedTaskID string, limit int, afterProject func() error) (storageports.ApplicationProjectSnapshot, error) {
	snapshot := storageports.ApplicationProjectSnapshot{ProjectView: reader.ReadProjectViewTx(ctx, tx, project, selectedTaskID, limit)}
	if afterProject != nil {
		if err := afterProject(); err != nil {
			return storageports.ApplicationProjectSnapshot{}, err
		}
	}
	snapshot.ExecutionState, snapshot.ExecutionError = readExecutionStateSnapshotTx(ctx, tx, project, reader.nativeBinding, nil)
	return snapshot, nil
}
