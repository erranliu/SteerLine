package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"sort"

	"github.com/erranliu/SteerLine/internal/runnerstate"
)

func readCheck(ctx context.Context, db schemaQuery, project, id string) (runnerstate.EnvironmentCheckRecord, error) {
	var raw []byte
	var revision uint64
	err := db.QueryRowContext(ctx, `SELECT revision,payload FROM environment_checks WHERE project_identity=? AND check_key=?`, project, id).Scan(&revision, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return runnerstate.EnvironmentCheckRecord{}, runnerstate.ErrEnvironmentCheckNotFound
	}
	if err != nil {
		return runnerstate.EnvironmentCheckRecord{}, err
	}
	var r runnerstate.EnvironmentCheckRecord
	if err = json.Unmarshal(raw, &r); err != nil {
		return r, err
	}
	if r.Project != project || r.ID != id || r.Revision == 0 || r.Revision != revision || len(r.Operations) == 0 {
		return r, errors.New("environment check association identity mismatch")
	}
	return r, nil
}
func (s *SQLiteStore) Check(ctx context.Context, project, id string) (runnerstate.EnvironmentCheckRecord, error) {
	return readCheck(ctx, s.db, project, id)
}
func (s *SQLiteStore) CheckRequest(ctx context.Context, project, id string) (runnerstate.EnvironmentCheckRecord, error) {
	var key string
	err := s.db.QueryRowContext(ctx, `SELECT check_key FROM environment_check_requests WHERE project_identity=? AND request_id=?`, project, id).Scan(&key)
	if errors.Is(err, sql.ErrNoRows) {
		return runnerstate.EnvironmentCheckRecord{}, runnerstate.ErrEnvironmentCheckNotFound
	}
	if err != nil {
		return runnerstate.EnvironmentCheckRecord{}, err
	}
	return s.Check(ctx, project, key)
}
func (s *SQLiteStore) Checks(ctx context.Context, project string) ([]runnerstate.EnvironmentCheckRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT check_key,revision,payload FROM environment_checks WHERE project_identity=?`, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []runnerstate.EnvironmentCheckRecord{}
	for rows.Next() {
		var raw []byte
		var key string
		var revision uint64
		if err = rows.Scan(&key, &revision, &raw); err != nil {
			return nil, err
		}
		var r runnerstate.EnvironmentCheckRecord
		if err = json.Unmarshal(raw, &r); err != nil {
			return nil, err
		}
		if r.Project != project || r.ID != key || r.Revision == 0 || r.Revision != revision || len(r.Operations) == 0 {
			return nil, errors.New("environment check record is incomplete")
		}
		result = append(result, r)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	return result, rows.Err()
}
func (s *SQLiteStore) SaveCheck(ctx context.Context, r runnerstate.EnvironmentCheckRecord, expected uint64) error {
	if s.readOnly {
		return errors.New("SQLite reader is read-only")
	}
	if r.ID == "" || r.Project == "" || r.Revision != expected+1 || len(r.Operations) == 0 {
		return errors.New("invalid environment check association")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	old, readErr := readCheck(ctx, tx, r.Project, r.ID)
	if expected == 0 {
		if !errors.Is(readErr, runnerstate.ErrEnvironmentCheckNotFound) {
			return errors.New("environment check already exists or cannot be read")
		}
	} else {
		if readErr != nil {
			return readErr
		}
		if old.Revision != expected {
			return errors.New("stale_revision: environment check changed")
		}
		if old.StateDir != r.StateDir || old.Selection != r.Selection || old.ResourceDigest != r.ResourceDigest || !sameCheckJSON(old.Input, r.Input) || !sameCheckJSON(old.Capabilities, r.Capabilities) || !old.CreatedAt.Equal(r.CreatedAt) || old.CheckID != "" && old.CheckID != r.CheckID || len(r.Operations) < len(old.Operations) {
			return errors.New("immutable environment check association changed")
		}
		for i, op := range old.Operations {
			next := r.Operations[i]
			if op.RequestID != next.RequestID || op.Action != next.Action || !sameCheckJSON(op.Original, next.Original) || !sameCheckJSON(op.Request, next.Request) || op.CommandID != "" && op.CommandID != next.CommandID {
				return errors.New("immutable environment check request changed")
			}
		}
	}
	body, err := json.Marshal(r)
	if err != nil {
		return err
	}
	if expected == 0 {
		_, err = tx.ExecContext(ctx, `INSERT INTO environment_checks VALUES(?,?,?,?)`, r.Project, r.ID, r.Revision, body)
	} else {
		var result sql.Result
		result, err = tx.ExecContext(ctx, `UPDATE environment_checks SET revision=?,payload=? WHERE project_identity=? AND check_key=? AND revision=?`, r.Revision, body, r.Project, r.ID, expected)
		if err == nil {
			var n int64
			n, err = result.RowsAffected()
			if err == nil && n != 1 {
				err = errors.New("stale_revision: environment check changed")
			}
		}
	}
	if err != nil {
		return err
	}
	for _, op := range r.Operations {
		if _, err = tx.ExecContext(ctx, `INSERT INTO environment_check_requests VALUES(?,?,?) ON CONFLICT DO NOTHING`, r.Project, op.RequestID, r.ID); err != nil {
			return err
		}
		var key string
		if err = tx.QueryRowContext(ctx, `SELECT check_key FROM environment_check_requests WHERE project_identity=? AND request_id=?`, r.Project, op.RequestID).Scan(&key); err != nil {
			return err
		}
		if key != r.ID {
			return errors.New("request_conflict: check request already belongs to another check")
		}
	}
	return tx.Commit()
}

func sameCheckJSON(a, b any) bool {
	x, ex := json.Marshal(a)
	y, ey := json.Marshal(b)
	var left, right any
	l, r := json.NewDecoder(bytes.NewReader(x)), json.NewDecoder(bytes.NewReader(y))
	l.UseNumber()
	r.UseNumber()
	return ex == nil && ey == nil && l.Decode(&left) == nil && r.Decode(&right) == nil && reflect.DeepEqual(left, right)
}
