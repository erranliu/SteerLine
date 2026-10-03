package sqlite

import (
	"context"
	"database/sql"

	"github.com/erranliu/SteerLine/internal/steerline/projection"
)

func (db *SQLiteStore) LoadRuntime(project string, requireIdentity bool) (*Runtime, error) {
	ctx := context.Background()
	tx, err := db.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	state, err := ReadRuntimeCheckpointTx(ctx, tx, project, requireIdentity)
	if err != nil {
		return nil, persistenceError(err)
	}
	snapshot, err := QuerySnapshot(ctx, tx, project)
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
