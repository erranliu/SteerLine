package steerline

import (
	"context"
	"database/sql"

	"github.com/erranliu/SteerLine/internal/steerline/projection"
	"github.com/erranliu/SteerLine/internal/steerline/sqlite"
)

func loadRuntimeSnapshot(db *SQLiteStore, project string, requireIdentity bool) (*Runtime, error) {
	ctx := context.Background()
	tx, err := db.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	state, err := readRuntimeCheckpointTx(ctx, tx, project, requireIdentity)
	if err != nil {
		return nil, persistenceError(err)
	}
	snapshot, err := querySnapshot(ctx, tx, project)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	view, err := projection.Build(snapshot, projection.MetadataOf(*state))
	if err != nil {
		return nil, err
	}
	return &view, nil
}

// readRuntimeCheckpointTx reads only service metadata; callers hydrate it with
// domain records from the same transaction before exposing a Runtime.
func readRuntimeCheckpointTx(ctx context.Context, tx *sql.Tx, project string, requireIdentity bool) (*Runtime, error) {
	return sqlite.ReadRuntimeCheckpointTx(ctx, tx, project, requireIdentity)
}
