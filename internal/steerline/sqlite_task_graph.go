package steerline

import (
	"context"

	"github.com/erranliu/SteerLine/internal/steerline/sqlite"
)

func (s *SQLiteStore) TaskGraphSnapshot(ctx context.Context, project string) (taskGraphSnapshot, error) {
	return s.reader().TaskGraphSnapshot(ctx, project)
}
func (t *sqliteTx) TaskGraphSnapshot(ctx context.Context, project string) (taskGraphSnapshot, error) {
	return sqlite.TaskGraphSnapshotTx(ctx, t.tx, project, nil)
}
