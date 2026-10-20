package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/erranliu/SteerLine/internal/testmanagement/model"
	"reflect"
)

func readEvidence(ctx context.Context, db schemaQuery, project, id string) (model.Evidence, error) {
	var r model.Evidence
	var raw []byte
	err := db.QueryRowContext(ctx, `SELECT payload FROM test_evidence WHERE project_identity=? AND candidate_id=?`, project, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return r, model.ErrNotFound
	}
	if err != nil {
		return r, err
	}
	err = json.Unmarshal(raw, &r)
	if err == nil && (r.Project != project || r.CandidateID != id || r.Revision == 0) {
		err = errors.New("test evidence association differs")
	}
	return r, err
}
func (s *SQLiteStore) Evidence(ctx context.Context, project, id string) (model.Evidence, error) {
	return readEvidence(ctx, s.db, project, id)
}
func (s *SQLiteStore) SaveEvidence(ctx context.Context, r model.Evidence, expected uint64) error {
	if s.readOnly || r.Project == "" || r.CandidateID == "" || r.RunID == "" || r.InputDigest == "" || r.Revision != expected+1 {
		return errors.New("invalid evidence write")
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
		return errors.New("evidence does not belong to original candidate Run")
	}
	old, e := readEvidence(ctx, tx, r.Project, r.CandidateID)
	if expected == 0 {
		if !errors.Is(e, model.ErrNotFound) {
			return errors.New("evidence already exists")
		}
	} else {
		if e != nil {
			return e
		}
		if old.Revision != expected || old.RunID != r.RunID || old.InputDigest != r.InputDigest || len(r.Collections) < len(old.Collections) || len(r.Artifacts) < len(old.Artifacts) {
			return errors.New("stale or changed evidence association")
		}
		for i, v := range old.Collections {
			if v.Path != r.Collections[i].Path || !reflect.DeepEqual(v.Request, r.Collections[i].Request) {
				return errors.New("collection original request changed")
			}
		}
		for i, v := range old.Artifacts {
			if !reflect.DeepEqual(v, r.Artifacts[i]) {
				return errors.New("immutable evidence changed")
			}
		}
	}
	raw, err := json.Marshal(r)
	if err != nil {
		return err
	}
	if expected == 0 {
		_, err = tx.ExecContext(ctx, `INSERT INTO test_evidence(project_identity,candidate_id,revision,payload) VALUES(?,?,?,?)`, r.Project, r.CandidateID, r.Revision, raw)
	} else {
		var result sql.Result
		result, err = tx.ExecContext(ctx, `UPDATE test_evidence SET revision=?,payload=? WHERE project_identity=? AND candidate_id=? AND revision=?`, r.Revision, raw, r.Project, r.CandidateID, expected)
		if err == nil {
			var n int64
			n, err = result.RowsAffected()
			if err == nil && n != 1 {
				err = errors.New("stale evidence revision")
			}
		}
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}
