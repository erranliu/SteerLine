package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/storage/ports"
)

func (s *SQLiteStore) ReadAbandonedActivities(ctx context.Context, project, source string, ids []string) (map[string]string, error) {
	result := map[string]string{}
	if len(ids) == 0 {
		return result, nil
	}
	db, err := s.asyncActivityPool()
	if err != nil {
		return nil, err
	}
	args := []any{project, source}
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := db.QueryContext(ctx, `SELECT source_id, abandoned_at FROM abandoned_activities WHERE project_identity=? AND source=? AND source_id IN (?`+strings.Repeat(",?", len(ids)-1)+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, at string
		if err = rows.Scan(&id, &at); err != nil {
			return nil, err
		}
		result[id] = at
	}
	return result, rows.Err()
}

type activityWriteKey struct{}

type activityWrite struct {
	source, id string
	receipt    ports.CommandRecord
	committed  bool
}

func (s *SQLiteStore) CommitActivityAbandonment(ctx context.Context, source, id string, receipt ports.CommandRecord, stop func(context.Context) error) error {
	w := &activityWrite{source: source, id: id, receipt: receipt}
	if stop != nil {
		if err := stop(context.WithValue(ctx, activityWriteKey{}, w)); err != nil {
			return err
		}
		if w.committed {
			return nil
		}
	}
	return s.transactSQLite(ctx, w.save)
}

func (w *activityWrite) save(tx *sqliteTx) error {
	if _, _, err := tx.EnqueueCommand(w.receipt); err != nil {
		return err
	}
	_, err := tx.tx.ExecContext(tx.ctx, `INSERT INTO abandoned_activities VALUES(?,?,?,?) ON CONFLICT DO NOTHING`, w.receipt.ProjectIdentity, w.source, w.id, w.receipt.CreatedAt.UTC().Format(time.RFC3339Nano))
	return err
}

// Only the owner record's save joins the abandonment transaction. Existing
// owner saves outside abandonment continue to commit on their own.
func (s *SQLiteStore) activityOwnerWrite(ctx context.Context, source, project, id string, write func(*sql.Tx) error) error {
	if s.readOnly {
		return errors.New("SQLite reader is read-only")
	}
	if w, ok := ctx.Value(activityWriteKey{}).(*activityWrite); ok {
		if w.source != source || w.receipt.ProjectIdentity != project || w.id != id || w.committed {
			return errors.New("activity abandonment owner binding differs")
		}
		err := s.transactSQLite(ctx, func(tx *sqliteTx) error {
			if err := write(tx.tx); err != nil {
				return err
			}
			return w.save(tx)
		})
		w.committed = err == nil
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = write(tx); err != nil {
		return err
	}
	return tx.Commit()
}
