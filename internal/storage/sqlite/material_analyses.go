package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"

	"github.com/erranliu/SteerLine/internal/mainagent/model"
)

func readAnalysis(ctx context.Context, db schemaQuery, project, id string) (model.Record, error) {
	var raw []byte
	var rev uint64
	var r model.Record
	err := db.QueryRowContext(ctx, `SELECT revision,payload FROM material_analyses WHERE project_identity=? AND analysis_id=?`, project, id).Scan(&rev, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return r, model.ErrNotFound
	}
	if err != nil {
		return r, err
	}
	if err = json.Unmarshal(raw, &r); err != nil {
		return r, err
	}
	if r.ID != id || r.Project != project || r.Revision != rev {
		return r, errors.New("analysis identity/revision mismatch")
	}
	return r, nil
}
func (s *SQLiteStore) Analysis(ctx context.Context, project, id string) (model.Record, error) {
	return readAnalysis(ctx, s.db, project, id)
}
func (s Reader) Analysis(ctx context.Context, project, id string) (model.Record, error) {
	return readAnalysis(ctx, s.db, project, id)
}
func readAnalyses(ctx context.Context, db schemaQuery, project string) ([]model.Record, error) {
	rows, err := db.QueryContext(ctx, `SELECT analysis_id FROM material_analyses WHERE project_identity=? ORDER BY analysis_id`, project)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	err = errors.Join(rows.Err(), rows.Close())
	if err != nil {
		return nil, err
	}
	out := []model.Record{}
	for _, id := range ids {
		r, e := readAnalysis(ctx, db, project, id)
		if e != nil {
			return nil, e
		}
		out = append(out, r)
	}
	return out, nil
}
func (s *SQLiteStore) Analyses(ctx context.Context, project string) ([]model.Record, error) {
	if s.legacyDiscard40 {
		return []model.Record{}, nil
	} // This record kind did not exist in the existing discard-only source.
	return readAnalyses(ctx, s.db, project)
}
func (s Reader) Analyses(ctx context.Context, project string) ([]model.Record, error) {
	return readAnalyses(ctx, s.db, project)
}

// Only the newest candidate-specific summaries are read by UI polling. Bodies,
// nested follow-up context and results are fetched by exact analysis ID.
func (s *SQLiteStore) AnalysisSummaries(ctx context.Context, project, candidate string) ([]model.Summary, bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT analysis_id,revision,json_extract(CAST(payload AS TEXT),'$.created_at'),json_extract(CAST(payload AS TEXT),'$.state'),json_extract(CAST(payload AS TEXT),'$.input.kind'),json_extract(CAST(payload AS TEXT),'$.input.note'),COALESCE(json_extract(CAST(payload AS TEXT),'$.input.parent_id'),''),COALESCE(json_extract(CAST(payload AS TEXT),'$.diagnostic'),'') FROM material_analyses WHERE project_identity=? AND json_extract(CAST(payload AS TEXT),'$.context.candidate_id')=? ORDER BY json_extract(CAST(payload AS TEXT),'$.created_at') DESC,analysis_id DESC LIMIT 51`, project, candidate)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	out := []model.Summary{}
	for rows.Next() {
		var r model.Summary
		if err = rows.Scan(&r.ID, &r.Revision, &r.CreatedAt, &r.State, &r.Kind, &r.Note, &r.ParentID, &r.Diagnostic); err != nil {
			return nil, false, err
		}
		out = append(out, r)
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
func (s *SQLiteStore) SaveAnalysis(ctx context.Context, r model.Record, expected uint64) error {
	if s.readOnly || r.Project == "" || !model.ID.MatchString(r.ID) || r.Revision != expected+1 {
		return errors.New("invalid analysis write")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	old, readErr := readAnalysis(ctx, tx, r.Project, r.ID)
	if expected == 0 {
		if !errors.Is(readErr, model.ErrNotFound) {
			return errors.New("analysis already exists or is unreadable")
		}
	} else {
		if readErr != nil {
			return readErr
		}
		if old.Revision != expected {
			return errors.New("stale_revision: analysis changed")
		}
		definiteRejection := old.Submission == "started" && r.Submission == "rejected" && r.State == "rejected" && r.RunID == ""
		if !reflect.DeepEqual(old.Input, r.Input) || !reflect.DeepEqual(old.Context, r.Context) || old.Runner != r.Runner || !reflect.DeepEqual(old.Request, r.Request) || old.CreatedAt != r.CreatedAt || old.Submission != "pending" && old.Submission != r.Submission && !definiteRejection || old.RunID != "" && old.RunID != r.RunID || old.InputDigest != "" && old.InputDigest != r.InputDigest || old.Result != nil && !reflect.DeepEqual(old.Result, r.Result) || old.Cancelled && !r.Cancelled {
			return errors.New("immutable analysis context/input/result changed")
		}
		if old.AnswerAction != nil && !reflect.DeepEqual(old.AnswerAction, r.AnswerAction) {
			return errors.New("immutable analysis answer association changed")
		}
		if old.Publication != nil && (r.Publication == nil || old.Publication.RequestID != r.Publication.RequestID || old.Publication.Review != r.Publication.Review || old.Publication.Branch != r.Publication.Branch || old.Publication.Base != r.Publication.Base || old.Publication.Commit != "" && old.Publication.Commit != r.Publication.Commit || old.Publication.URL != "" && old.Publication.URL != r.Publication.URL) {
			return errors.New("immutable material publication changed")
		}
	}
	raw, err := json.Marshal(r)
	if err != nil {
		return err
	}
	if expected == 0 {
		_, err = tx.ExecContext(ctx, `INSERT INTO material_analyses VALUES(?,?,?,?)`, r.Project, r.ID, r.Revision, raw)
	} else {
		var result sql.Result
		result, err = tx.ExecContext(ctx, `UPDATE material_analyses SET revision=?,payload=? WHERE project_identity=? AND analysis_id=? AND revision=?`, r.Revision, raw, r.Project, r.ID, expected)
		if err == nil {
			err = requireIdentityWrite(result, nil, "stale_revision: analysis changed")
		}
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}
