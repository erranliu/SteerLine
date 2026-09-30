package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
)

func ReadRuntimeCheckpointTx(ctx context.Context, tx *sql.Tx, project string, requireIdentity bool) (*contracts.Runtime, error) {
	var payload []byte
	if err := tx.QueryRowContext(ctx, `SELECT payload FROM projection WHERE project_identity=?`, project).Scan(&payload); err != nil {
		return nil, err
	}
	var checkpoint contracts.ServiceCheckpoint
	if err := contracts.DecodeJSON(payload, &checkpoint); err != nil {
		return nil, err
	}
	state := contracts.RuntimeFromCheckpoint(checkpoint)
	if state.SchemaVersion != contracts.WorkerFlowRuntimeSchema {
		return nil, fmt.Errorf("unsupported runtime schema %d", state.SchemaVersion)
	}
	if requireIdentity && state.Repository.Identity != project {
		return nil, errors.New("database runtime repository identity mismatch")
	}
	var paused int
	if err := tx.QueryRowContext(ctx, `SELECT dispatch_mode,dispatch_paused,authority_version FROM project_task_authorities WHERE project_identity=?`, project).Scan(&state.Control.DispatchMode, &paused, &state.Control.Revision); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("runtime checkpoint has no task authority")
		}
		return nil, err
	}
	state.Control.DispatchPaused = paused != 0
	return &state, nil
}
