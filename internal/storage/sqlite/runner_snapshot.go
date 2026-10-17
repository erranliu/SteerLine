package sqlite

import (
	"context"
	"database/sql"
	"errors"

	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func (s Reader) ReadRunnerSnapshot(ctx context.Context, project string) (storageports.RunnerSnapshot, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return storageports.RunnerSnapshot{}, err
	}
	defer tx.Rollback()
	result, err := readRunnerSnapshotTx(ctx, tx, project, s.nativeBinding)
	if err != nil {
		return result, err
	}
	return result, tx.Commit()
}

func readRunnerSnapshotTx(ctx context.Context, tx *sql.Tx, project string, binding *taskmodel.ProjectTaskAuthority) (storageports.RunnerSnapshot, error) {
	var result storageports.RunnerSnapshot
	var err error
	result.Graph, err = TaskGraphSnapshotTx(ctx, tx, project, binding)
	if err != nil {
		return result, err
	}
	result.Candidates, err = readRunnerCandidates(ctx, tx, project, "")
	if err != nil {
		return result, err
	}
	result.Checkpoint, err = readServiceCheckpoint(ctx, tx, project, true)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		return result, err
	}
	return result, nil
}
