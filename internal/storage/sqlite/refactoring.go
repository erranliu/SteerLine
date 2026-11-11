package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	refactoring "github.com/erranliu/SteerLine/internal/refactoring/model"
)

func (s *SQLiteStore) Refactoring(ctx context.Context, project, id string) (refactoring.Record, error) {
	var r refactoring.Record
	var raw []byte
	var rev uint64
	e := s.db.QueryRowContext(ctx, `SELECT revision,payload FROM refactoring_actions WHERE project_identity=? AND action_id=?`, project, id).Scan(&rev, &raw)
	if errors.Is(e, sql.ErrNoRows) {
		return r, refactoring.ErrNotFound
	}
	if e != nil {
		return r, e
	}
	if e = json.Unmarshal(raw, &r); e != nil {
		return r, e
	}
	if r.ID != id || r.Project != project || r.Revision != rev {
		return r, errors.New("维护记录身份不符")
	}
	return r, nil
}
func (s *SQLiteStore) Refactorings(ctx context.Context, project string) ([]refactoring.Record, error) {
	rows, e := s.db.QueryContext(ctx, `SELECT payload FROM refactoring_actions WHERE project_identity=? ORDER BY rowid DESC`, project)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []refactoring.Record{}
	for rows.Next() {
		var b []byte
		var r refactoring.Record
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		if e = json.Unmarshal(b, &r); e != nil {
			return nil, e
		}
		if r.Project != project {
			return nil, errors.New("维护项目身份不符")
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func (s *SQLiteStore) RefactoringCommand(ctx context.Context, project, id string) ([]byte, []byte, error) {
	var raw, result []byte
	var kind string
	e := s.db.QueryRowContext(ctx, `SELECT kind,payload,result FROM commands WHERE project_identity=? AND command_id=?`, project, id).Scan(&kind, &raw, &result)
	if errors.Is(e, sql.ErrNoRows) {
		return nil, nil, refactoring.ErrNotFound
	}
	if e != nil {
		return nil, nil, e
	}
	if kind != "refactoring" {
		return nil, nil, refactoring.ErrConflict
	}
	return raw, result, nil
}
func (s *SQLiteStore) SaveRefactoring(ctx context.Context, r refactoring.Record, expected uint64, cmd *refactoring.Command) error {
	if s.readOnly || r.ID == "" || r.Project == "" || r.Revision != expected+1 {
		return errors.New("无效维护写入")
	}
	raw, e := json.Marshal(r)
	if e != nil {
		return e
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	active := 0
	if r.Active() {
		active = 1
	}
	if active == 1 {
		var n int
		if e = tx.QueryRowContext(ctx, `SELECT count(*) FROM refactoring_actions WHERE project_identity=? AND action_id<>? AND active=1`, r.Project, r.ID).Scan(&n); e != nil {
			return e
		}
		if n > 0 {
			return refactoring.ErrConflict
		}
	}
	if expected == 0 {
		_, e = tx.ExecContext(ctx, `INSERT INTO refactoring_actions VALUES(?,?,?,?,?)`, r.Project, r.ID, r.Revision, active, raw)
	} else {
		var result sql.Result
		result, e = tx.ExecContext(ctx, `UPDATE refactoring_actions SET revision=?,active=?,payload=? WHERE project_identity=? AND action_id=? AND revision=?`, r.Revision, active, raw, r.Project, r.ID, expected)
		if e == nil {
			n, err := result.RowsAffected()
			if err != nil {
				return err
			}
			if n != 1 {
				return refactoring.ErrConflict
			}
		}
	}
	if e != nil {
		return e
	}
	if cmd != nil {
		ack, _ := json.Marshal(struct {
			ID       string `json:"id"`
			Revision uint64 `json:"revision"`
		}{r.ID, r.Revision})
		_, e = tx.ExecContext(ctx, `INSERT INTO commands(project_identity,command_id,kind,target_id,expected_version,idempotency_key,requested_by,payload,status,result,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, r.Project, cmd.ID, "refactoring", cmd.Target, cmd.Expected, cmd.ID, "local-user", cmd.Raw, "applied", ack, r.UpdatedAt.Format(time.RFC3339Nano), r.UpdatedAt.Format(time.RFC3339Nano))
		if e != nil {
			return e
		}
	}
	return tx.Commit()
}
