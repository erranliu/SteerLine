package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/erranliu/SteerLine/internal/testmanagement/model"
)

func readProgress(ctx context.Context, db schemaQuery, project, id string) (model.Progress, error) {
	var r model.Progress
	var raw []byte
	err := db.QueryRowContext(ctx, `SELECT payload FROM candidate_progress WHERE project_identity=? AND candidate_id=?`, project, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return r, model.ErrNotFound
	}
	if err != nil {
		return r, err
	}
	err = json.Unmarshal(raw, &r)
	if err == nil && (r.Project != project || r.CandidateID != id || r.Revision == 0) {
		err = errors.New("test progress association differs")
	}
	return r, err
}
func (s *SQLiteStore) Progress(ctx context.Context, project, id string) (model.Progress, error) {
	return readProgress(ctx, s.db, project, id)
}
func (s *SQLiteStore) SaveProgress(ctx context.Context, r model.Progress, expected uint64) error {
	if s.readOnly || r.Project == "" || r.CandidateID == "" || r.RunID == "" || r.InputDigest == "" || r.Revision != expected+1 {
		return errors.New("invalid progress write")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var candidateRaw []byte
	if err = tx.QueryRowContext(ctx, `SELECT payload FROM runner_candidates WHERE project_identity=? AND candidate_id=?`, r.Project, r.CandidateID).Scan(&candidateRaw); err != nil {
		return err
	}
	var c struct {
		RunID       string `json:"run_id"`
		InputDigest string `json:"input_digest"`
	}
	if json.Unmarshal(candidateRaw, &c) != nil || c.RunID != r.RunID || c.InputDigest != r.InputDigest {
		return errors.New("progress does not belong to original candidate Run")
	}
	old, e := readProgress(ctx, tx, r.Project, r.CandidateID)
	if expected == 0 {
		if !errors.Is(e, model.ErrNotFound) {
			return errors.New("progress already exists")
		}
	} else {
		if e != nil {
			return e
		}
		if old.Revision != expected || old.RunID != r.RunID || old.InputDigest != r.InputDigest || r.Snapshot.Version < old.Snapshot.Version {
			return errors.New("stale or changed progress association")
		}
	}
	raw, err := json.Marshal(r)
	if err != nil {
		return err
	}
	if expected == 0 {
		_, err = tx.ExecContext(ctx, `INSERT INTO candidate_progress(project_identity,candidate_id,revision,payload) VALUES(?,?,?,?)`, r.Project, r.CandidateID, r.Revision, raw)
	} else {
		var result sql.Result
		result, err = tx.ExecContext(ctx, `UPDATE candidate_progress SET revision=?,payload=? WHERE project_identity=? AND candidate_id=? AND revision=?`, r.Revision, raw, r.Project, r.CandidateID, expected)
		if err == nil {
			var n int64
			n, err = result.RowsAffected()
			if err == nil && n != 1 {
				err = errors.New("stale progress revision")
			}
		}
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}
