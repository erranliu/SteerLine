package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/erranliu/SteerLine/internal/hostcontract"
)

type checkpointQuery interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func ReadServiceCheckpointTx(ctx context.Context, tx *sql.Tx, project string, requireIdentity bool) (*hostcontract.ServiceCheckpoint, error) {
	return readServiceCheckpoint(ctx, tx, project, requireIdentity)
}

func readServiceCheckpoint(ctx context.Context, tx checkpointQuery, project string, requireIdentity bool) (*hostcontract.ServiceCheckpoint, error) {
	var payload []byte
	if err := tx.QueryRowContext(ctx, `SELECT payload FROM projection WHERE project_identity=?`, project).Scan(&payload); err != nil {
		return nil, err
	}
	var checkpoint hostcontract.ServiceCheckpoint
	if err := decode(payload, &checkpoint); err != nil {
		return nil, err
	}
	if checkpoint.SchemaVersion != hostcontract.ServiceCheckpointSchema {
		return nil, fmt.Errorf("unsupported runtime schema %d", checkpoint.SchemaVersion)
	}
	if requireIdentity && checkpoint.Repository.Identity != project {
		return nil, errors.New("database checkpoint repository identity mismatch")
	}
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM project_task_authorities WHERE project_identity=?`, project).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("service checkpoint has no task authority")
		}
		return nil, err
	}
	return &checkpoint, nil
}
