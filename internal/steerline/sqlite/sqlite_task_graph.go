package sqlite

import (
	"context"

	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func (t *sqliteTx) TaskGraphSnapshot(ctx context.Context, project string) (taskGraphSnapshot, error) {
	return TaskGraphSnapshotTx(ctx, t.tx, project, nil)
}

// TaskSnapshot converts one SQLite read snapshot to task-only delivery facts.
func (s *SQLiteStore) TaskSnapshot(ctx context.Context, project string) (taskmodel.Snapshot, error) {
	graph, err := s.TaskGraphSnapshot(ctx, project)
	if err != nil {
		return taskmodel.Snapshot{}, err
	}
	return graph.TaskGraph(), nil
}
