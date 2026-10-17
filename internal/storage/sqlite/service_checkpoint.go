package sqlite

import (
	"context"
	"database/sql"

	"github.com/erranliu/SteerLine/internal/hostcontract"
)

func (s *SQLiteStore) LoadCheckpoint(ctx context.Context, project string, requireIdentity bool) (*hostcontract.ServiceCheckpoint, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	checkpoint, err := ReadServiceCheckpointTx(ctx, tx, project, requireIdentity)
	if err != nil {
		return nil, persistenceError(err)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return checkpoint, nil
}
