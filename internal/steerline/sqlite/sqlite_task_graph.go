package sqlite

import (
	"context"
)

func (t *sqliteTx) TaskGraphSnapshot(ctx context.Context, project string) (taskGraphSnapshot, error) {
	return TaskGraphSnapshotTx(ctx, t.tx, project, nil)
}
