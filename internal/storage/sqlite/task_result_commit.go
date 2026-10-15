package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func (s *SQLiteStore) TaskResult(ctx context.Context, project, id string) (taskmodel.ResultReceipt, bool, error) {
	return readTaskResult(ctx, s.db, project, id)
}

func (tx *sqliteTx) taskResult(ctx context.Context, project, id string) (taskmodel.ResultReceipt, bool, error) {
	return readTaskResult(ctx, tx.tx, project, id)
}

func readTaskResult(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, project, id string) (taskmodel.ResultReceipt, bool, error) {
	var raw []byte
	var receipt taskmodel.ResultReceipt
	err := q.QueryRowContext(ctx, `SELECT receipt FROM task_results WHERE project_identity=? AND result_id=?`, project, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return receipt, false, nil
	}
	if err != nil {
		return receipt, false, err
	}
	err = json.Unmarshal(raw, &receipt)
	return receipt, err == nil, err
}

func mustResultJSON(v any) []byte { b, _ := json.Marshal(v); return b }
