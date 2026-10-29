package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	env "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
)

func readEnvironment(ctx context.Context, db schemaQuery, project, id string) (env.Record, error) {
	var b []byte
	var rev uint64
	e := db.QueryRowContext(ctx, `SELECT revision,payload FROM project_environments WHERE project_identity=? AND activity_id=?`, project, id).Scan(&rev, &b)
	if errors.Is(e, sql.ErrNoRows) {
		return env.Record{}, env.ErrNotFound
	}
	if e != nil {
		return env.Record{}, e
	}
	var r env.Record
	if e = json.Unmarshal(b, &r); e != nil {
		return r, e
	}
	if r.ID != id || r.Project != project || r.Revision != rev || rev == 0 || len(r.Requests) == 0 {
		return r, errors.New("environment activity binding differs")
	}
	return r, nil
}
func (s *SQLiteStore) Environment(ctx context.Context, project, id string) (env.Record, error) {
	return readEnvironment(ctx, s.db, project, id)
}
func (s *SQLiteStore) Environments(ctx context.Context, project string) ([]env.Record, error) {
	rows, e := s.db.QueryContext(ctx, `SELECT activity_id,revision,payload FROM project_environments WHERE project_identity=? ORDER BY rowid DESC`, project)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []env.Record{}
	for rows.Next() {
		var id string
		var rev uint64
		var b []byte
		if e = rows.Scan(&id, &rev, &b); e != nil {
			return nil, e
		}
		var r env.Record
		if e = json.Unmarshal(b, &r); e != nil {
			return nil, e
		}
		if r.ID != id || r.Project != project || r.Revision != rev || rev == 0 {
			return nil, errors.New("environment row identity differs")
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func (s *SQLiteStore) SaveEnvironment(ctx context.Context, r env.Record, expected uint64) error {
	if s.readOnly {
		return errors.New("SQLite reader is read-only")
	}
	if !env.IDPattern.MatchString(r.ID) || r.Project == "" || r.Revision != expected+1 || len(r.Requests) == 0 || (r.Kind != "adaptation" && r.Kind != "generation") || len(r.Rounds) > env.MaxRounds {
		return errors.New("invalid environment activity")
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	old, e := readEnvironment(ctx, tx, r.Project, r.ID)
	if expected == 0 {
		if !errors.Is(e, env.ErrNotFound) {
			return errors.New("environment activity exists or unavailable")
		}
	} else {
		if e != nil {
			return e
		}
		if old.Revision != expected {
			return errors.New("stale_revision: environment activity changed")
		}
		if old.Kind != r.Kind || old.Profile != r.Profile || old.Build != r.Build || !old.CreatedAt.Equal(r.CreatedAt) || old.TimeoutSeconds != r.TimeoutSeconds || env.DigestJSON(old.Execution) != env.DigestJSON(r.Execution) || env.DigestJSON(old.Prepared) != env.DigestJSON(r.Prepared) || len(r.Rounds) < len(old.Rounds) || len(r.Rounds) > len(old.Rounds)+1 || len(r.Requests) < len(old.Requests) || old.Cancelled && !r.Cancelled {
			return errors.New("immutable environment activity changed")
		}
		for i, a := range old.Rounds {
			b := r.Rounds[i]
			stopChanged := a.Stop != nil && env.DigestJSON(a.Stop) != env.DigestJSON(b.Stop)
			if stopChanged && (a.StopState != "rejected" || len(b.RejectedStops) != len(a.RejectedStops)+1 || env.DigestJSON(b.RejectedStops[len(b.RejectedStops)-1]) != env.DigestJSON(*a.Stop)) {
				return errors.New("unverified original stop cannot be replaced")
			}
			if len(b.RejectedStops) < len(a.RejectedStops) {
				return errors.New("original stop history removed")
			}
			for j, q := range a.RejectedStops {
				if env.DigestJSON(q) != env.DigestJSON(b.RejectedStops[j]) {
					return errors.New("original stop history changed")
				}
			}
			if a.Number != b.Number || env.DigestJSON(a.Request) != env.DigestJSON(b.Request) || !a.Deadline.Equal(b.Deadline) || a.RunID != "" && a.RunID != b.RunID || a.InputDigest != "" && a.InputDigest != b.InputDigest || a.Settled && !b.Settled || a.TimedOut && !b.TimedOut {
				return errors.New("immutable environment round changed")
			}
		}
		for i, q := range old.Requests {
			if env.DigestJSON(q) != env.DigestJSON(r.Requests[i]) {
				return errors.New("immutable environment request changed")
			}
		}
	}
	b, e := json.Marshal(r)
	if e != nil {
		return e
	}
	if expected == 0 {
		_, e = tx.ExecContext(ctx, `INSERT INTO project_environments VALUES(?,?,?,?)`, r.Project, r.ID, r.Revision, b)
	} else {
		var result sql.Result
		result, e = tx.ExecContext(ctx, `UPDATE project_environments SET revision=?,payload=? WHERE project_identity=? AND activity_id=? AND revision=?`, r.Revision, b, r.Project, r.ID, expected)
		if e == nil {
			var n int64
			n, e = result.RowsAffected()
			if e == nil && n != 1 {
				e = errors.New("stale_revision: environment activity changed")
			}
		}
	}
	if e != nil {
		return e
	}
	for _, q := range r.Requests {
		if !env.IDPattern.MatchString(q.ID) || !json.Valid(q.Original) {
			return errors.New("invalid original environment request")
		}
		_, e = tx.ExecContext(ctx, `INSERT INTO environment_requests VALUES(?,?,?) ON CONFLICT DO NOTHING`, r.Project, q.ID, r.ID)
		if e != nil {
			return e
		}
		var id string
		if e = tx.QueryRowContext(ctx, `SELECT activity_id FROM environment_requests WHERE project_identity=? AND request_id=?`, r.Project, q.ID).Scan(&id); e != nil {
			return e
		}
		if id != r.ID {
			return errors.New("request_conflict: environment request already bound")
		}
	}
	return tx.Commit()
}
