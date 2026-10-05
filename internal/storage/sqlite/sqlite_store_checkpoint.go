package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

// SaveCheckpoint publishes service metadata only. It has no business write set.
func (s *SQLiteStore) SaveCheckpoint(ctx context.Context, checkpoint storageports.ServiceCheckpoint) (storageports.ServiceCheckpoint, error) {
	var committed storageports.ServiceCheckpoint
	err := s.transactSQLite(ctx, func(tx *sqliteTx) error {
		var err error
		committed, err = tx.publishCheckpoint(checkpoint)
		return err
	})
	return committed, err
}

func (tx *sqliteTx) publishCheckpoint(checkpoint storageports.ServiceCheckpoint) (storageports.ServiceCheckpoint, error) {
	project := checkpoint.Repository.Identity
	if project == "" {
		return checkpoint, errors.New("checkpoint repository identity is required")
	}
	if checkpoint.SchemaVersion != storageports.WorkerFlowRuntimeSchema {
		return checkpoint, fmt.Errorf("unsupported checkpoint schema %d", checkpoint.SchemaVersion)
	}
	var authority string
	if err := tx.tx.QueryRowContext(tx.ctx, `SELECT authority FROM project_task_authorities WHERE project_identity=?`, project).Scan(&authority); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return checkpoint, nativeErr("authority_mismatch", "checkpoint requires native task authority")
		}
		return checkpoint, err
	}
	if authority != "native" {
		return checkpoint, nativeErr("authority_mismatch", "checkpoint requires native task authority")
	}
	var revision uint64
	if err := tx.tx.QueryRowContext(tx.ctx, `SELECT COALESCE(MAX(revision),0) FROM projection WHERE project_identity=?`, project).Scan(&revision); err != nil {
		return checkpoint, err
	}
	checkpoint.Revision = revision + 1
	payload, err := json.Marshal(checkpoint)
	if err != nil {
		return checkpoint, err
	}
	return checkpoint, tx.PutProjection(project, checkpoint.Revision, payload)
}

func operationDigest(payload []byte) string {
	h := sha256.Sum256(payload)
	return hex.EncodeToString(h[:])
}
