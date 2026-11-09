package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/erranliu/SteerLine/internal/mainagent/contract"
)

func (s *SQLiteStore) LoadAdvisory(ctx context.Context, project, id string) ([]byte, uint64, error) {
	var raw []byte
	var rev uint64
	err := s.db.QueryRowContext(ctx, `SELECT payload,revision FROM advisory_jobs WHERE project_identity=? AND request_id=?`, project, id).Scan(&raw, &rev)
	if errors.Is(err, sql.ErrNoRows) {
		err = contract.ErrNotFound
	}
	return raw, rev, err
}
func (s *SQLiteStore) SaveAdvisory(ctx context.Context, project, id, candidate string, raw []byte, expected uint64) error {
	if s.readOnly || project == "" || !contract.ID.MatchString(id) || !contract.ID.MatchString(candidate) || !json.Valid(raw) {
		return errors.New("invalid advisory storage identity or payload")
	}
	var binding struct {
		Project  string
		ID       string
		Revision uint64
		Source   struct {
			CandidateID string `json:"candidate_id"`
		}
	}
	if err := json.Unmarshal(raw, &binding); err != nil {
		return err
	}
	if binding.Project != project || binding.ID != id || binding.Revision != expected+1 || binding.Source.CandidateID != candidate {
		return errors.New("advisory storage identity mismatch")
	}
	if expected == 0 {
		result, err := s.db.ExecContext(ctx, `INSERT INTO advisory_jobs(project_identity,request_id,candidate_id,revision,payload) VALUES(?,?,?,?,?) ON CONFLICT(project_identity,request_id) DO NOTHING`, project, id, candidate, binding.Revision, raw)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err == nil && n != 1 {
			return contract.ErrConflict
		}
		return err
	}
	result, err := s.db.ExecContext(ctx, `UPDATE advisory_jobs SET revision=?,payload=? WHERE project_identity=? AND request_id=? AND candidate_id=? AND revision=?`, binding.Revision, raw, project, id, candidate, expected)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n != 1 {
		return contract.ErrConflict
	}
	return err
}
func (s *SQLiteStore) AdvisoryIDs(ctx context.Context, project, scope string) ([]string, error) {
	where := ""
	switch scope {
	case "active":
		where = `json_extract(payload,'$.source_active')=1`
	case "pending":
		where = `json_extract(payload,'$.state') IN ('queued','preparing')`
	default:
		return nil, fmt.Errorf("unsupported advisory read scope %q", scope)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT request_id FROM advisory_jobs WHERE project_identity=? AND `+where+` ORDER BY json_extract(payload,'$.created_at'),request_id`, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
func (s *SQLiteStore) AdvisorySummaries(ctx context.Context, project, candidate string) ([]json.RawMessage, bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT json_object('id',request_id,'revision',revision,'source',json_remove(json_extract(payload,'$.source'),'$.facts','$.goal'),'state',json_extract(payload,'$.state'),'created_at',json_extract(payload,'$.created_at'),'diagnostic',json_extract(payload,'$.diagnostic')) FROM advisory_jobs WHERE project_identity=? AND candidate_id=? ORDER BY json_extract(payload,'$.created_at') DESC,request_id DESC LIMIT 51`, project, candidate)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	out := []json.RawMessage{}
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			return nil, false, err
		}
		out = append(out, json.RawMessage(raw))
	}
	if err = rows.Err(); err != nil {
		return nil, false, err
	}
	more := len(out) > 50
	if more {
		out = out[:50]
	}
	return out, more, nil
}
