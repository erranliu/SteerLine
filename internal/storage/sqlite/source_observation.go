package sqlite

import (
	"context"
	"time"
)

// Observation metadata is not an execution state, revision or admission gate.
// Only the existing fact collector calls this; the activity reader cannot.
func (s *SQLiteStore) RecordSourceObservation(ctx context.Context, project, source, id string, revision uint64, at time.Time, diagnostic string) error {
	stamp := ""
	if !at.IsZero() {
		stamp = at.UTC().Format(time.RFC3339Nano)
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO source_observations VALUES(?,?,?,?,?,?) ON CONFLICT(project_identity,source,source_id) DO UPDATE SET source_revision=excluded.source_revision,last_success=CASE WHEN excluded.last_success='' THEN source_observations.last_success ELSE excluded.last_success END,diagnostic=excluded.diagnostic WHERE excluded.source_revision>=source_observations.source_revision`, project, source, id, revision, stamp, diagnostic)
	return err
}
