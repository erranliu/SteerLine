package steerline

import (
	"context"
	"errors"

	"github.com/erranliu/SteerLine/internal/steerline/query"
	"github.com/erranliu/SteerLine/internal/steerline/sqlite"
	"github.com/erranliu/SteerLine/internal/steerline/task"
)

type taskGraphSnapshot = task.Snapshot

func (s *SQLiteStore) taskGraphSnapshot(ctx context.Context, project string) (taskGraphSnapshot, error) {
	return s.reader().TaskGraphSnapshot(ctx, project)
}
func (s *SQLiteStore) TaskGraphSnapshot(ctx context.Context, project string) (taskGraphSnapshot, error) {
	return s.reader().TaskGraphSnapshot(ctx, project)
}
func taskGraphFromStore(ctx context.Context, store query.TaskReader, project string) (taskGraphSnapshot, error) {
	if store == nil {
		return taskGraphSnapshot{}, errors.New("task graph reader is required")
	}
	return store.TaskGraphSnapshot(ctx, project)
}
func taskGraphSnapshotTx(ctx context.Context, t *sqliteTx, project string) (taskGraphSnapshot, error) {
	return sqlite.TaskGraphSnapshotTx(ctx, t.tx, project, nil)
}
