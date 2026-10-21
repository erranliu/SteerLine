package sqlite

import (
	"context"
	"encoding/json"
)

func (s *SQLiteStore) MaintenanceReceipt(ctx context.Context, project, id string) (json.RawMessage, error) {
	var raw []byte
	e := s.db.QueryRowContext(ctx, `SELECT result FROM maintenance_requests WHERE project_identity=? AND request_id=?`, project, id).Scan(&raw)
	return raw, persistenceError(e)
}
