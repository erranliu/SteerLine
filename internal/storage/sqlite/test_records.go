package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	m "github.com/erranliu/SteerLine/internal/testmanagement/model"
	"time"
)

func readTestRecord(ctx context.Context, db schemaQuery, project, id string, summary bool) (m.TestRecord, error) {
	var r m.TestRecord
	var raw, content []byte
	column := "NULL"
	if summary {
		column = "summary"
	}
	err := db.QueryRowContext(ctx, `SELECT seq,payload,`+column+`,evidence_status,diagnostic,source,observed_at FROM test_records WHERE project_identity=? AND record_id=?`, project, id).Scan(&r.Seq, &raw, &content, &r.EvidenceStatus, &r.Diagnostic, newJSONScanner(&r.Source), newTimeScanner(&r.ObservedAt))
	if errors.Is(err, sql.ErrNoRows) {
		return r, m.ErrNotFound
	}
	if err != nil {
		return r, err
	}
	r.Project = project
	r.Summary = content
	err = json.Unmarshal(raw, &r.Report)
	return r, err
}

// Tiny scanners keep storage values typed without importing the business owner.
type jsonScanner struct{ value any }

func newJSONScanner(v any) *jsonScanner { return &jsonScanner{v} }
func (s *jsonScanner) Scan(v any) error {
	switch x := v.(type) {
	case []byte:
		return json.Unmarshal(x, s.value)
	case string:
		return json.Unmarshal([]byte(x), s.value)
	}
	return errors.New("invalid stored JSON")
}

type timeScanner struct{ value *time.Time }

func newTimeScanner(v *time.Time) *timeScanner { return &timeScanner{v} }
func (s *timeScanner) Scan(v any) error {
	str, ok := v.(string)
	if !ok {
		return errors.New("invalid stored time")
	}
	t, e := time.Parse(time.RFC3339Nano, str)
	*s.value = t
	return e
}

func (s *SQLiteStore) TestRecord(ctx context.Context, project, id string) (m.TestRecord, error) {
	return readTestRecord(ctx, s.db, project, id, true)
}
func (s *SQLiteStore) SaveTestRecord(ctx context.Context, r m.TestRecord) (m.TestRecord, error) {
	if s.readOnly || r.Project == "" || r.Report.ID == "" || len(r.Report.Selection) != 1 {
		return r, fmt.Errorf("%w: invalid record write", m.ErrInvalidRecord)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return r, err
	}
	defer tx.Rollback()
	body, err := json.Marshal(r.Report)
	if err != nil {
		return r, err
	}
	old, e := readTestRecord(ctx, tx, r.Project, r.Report.ID, true)
	if e == nil {
		oldBody, _ := json.Marshal(old.Report)
		if string(oldBody) != string(body) {
			return r, fmt.Errorf("%w: immutable test report differs", m.ErrRecordConflict)
		}
		if r.Source.Kind != "runner" {
			r.Source = old.Source
		}
		if old.Source.Kind == "runner" && r.Source.Kind == "runner" && (old.Source.RunID != r.Source.RunID || old.Source.CandidateID != r.Source.CandidateID) {
			return r, fmt.Errorf("%w: original Runner association differs", m.ErrRecordConflict)
		}
		if len(r.Summary) == 0 {
			r.Summary = old.Summary
			r.EvidenceStatus = old.EvidenceStatus
			r.Diagnostic = old.Diagnostic
		}
		r.Seq = old.Seq
	} else if !errors.Is(e, m.ErrNotFound) {
		return r, e
	} else if r.Report.RerunOf != "" {
		parent, err := readTestRecord(ctx, tx, r.Project, r.Report.RerunOf, false)
		if errors.Is(err, m.ErrNotFound) {
			return r, fmt.Errorf("%w: rerun parent must already be registered", m.ErrInvalidRecord)
		}
		if err != nil {
			return r, err
		}
		if parent.Report.Definition.ID != r.Report.Definition.ID {
			return r, fmt.Errorf("%w: rerun parent belongs to another test set", m.ErrInvalidRecord)
		}
	}
	source, _ := json.Marshal(r.Source)
	if r.Seq == 0 {
		res, err := tx.ExecContext(ctx, `INSERT INTO test_records(project_identity,record_id,test_id,started_at,payload,summary,evidence_status,diagnostic,source,observed_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, r.Project, r.Report.ID, r.Report.Definition.ID, r.Report.StartedAt.UTC().Format("2006-01-02T15:04:05.000000000Z"), body, r.Summary, r.EvidenceStatus, r.Diagnostic, source, r.ObservedAt.UTC().Format(time.RFC3339Nano))
		if err != nil {
			return r, err
		}
		r.Seq, err = res.LastInsertId()
		if err != nil {
			return r, err
		}
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE test_records SET summary=?,evidence_status=?,diagnostic=?,source=?,observed_at=? WHERE project_identity=? AND record_id=?`, r.Summary, r.EvidenceStatus, r.Diagnostic, source, r.ObservedAt.UTC().Format(time.RFC3339Nano), r.Project, r.Report.ID)
		if err != nil {
			return r, err
		}
	}
	return r, tx.Commit()
}
func (s *SQLiteStore) TestRecordHistory(ctx context.Context, project, testID string, upper int64) ([]m.TestRecord, int64, error) {
	if upper == 0 {
		if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(seq),0) FROM test_records WHERE project_identity=?`, project).Scan(&upper); err != nil {
			return nil, 0, err
		}
	}
	query := `SELECT seq,payload,evidence_status,diagnostic,source,observed_at FROM test_records WHERE project_identity=? AND seq<=?`
	args := []any{project, upper}
	if testID != "" {
		query += ` AND test_id=?`
		args = append(args, testID)
	}
	query += ` ORDER BY started_at DESC,record_id DESC`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []m.TestRecord{}
	for rows.Next() {
		var r m.TestRecord
		var raw []byte
		if err := rows.Scan(&r.Seq, &raw, &r.EvidenceStatus, &r.Diagnostic, newJSONScanner(&r.Source), newTimeScanner(&r.ObservedAt)); err != nil {
			return nil, 0, err
		}
		if err = json.Unmarshal(raw, &r.Report); err != nil {
			return nil, 0, err
		}
		r.Project = project
		out = append(out, r)
	}
	return out, upper, rows.Err()
}
func (s *SQLiteStore) TestRecordDefinitions(ctx context.Context, project string) ([]m.Definition, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT json_extract(payload,'$.definition') FROM test_records WHERE seq IN (SELECT MAX(seq) FROM test_records WHERE project_identity=? GROUP BY test_id) ORDER BY test_id`, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []m.Definition{}
	for rows.Next() {
		var d m.Definition
		if err = rows.Scan(newJSONScanner(&d)); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
func (s *SQLiteStore) PendingTestEvidence(ctx context.Context, project, after string, limit int) ([]m.Evidence, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT e.payload,c.task_id FROM test_evidence e JOIN runner_candidates c ON c.project_identity=e.project_identity AND c.candidate_id=e.candidate_id WHERE e.project_identity=? AND json_array_length(e.payload,'$.artifacts')>e.projected_artifacts ORDER BY CASE WHEN e.candidate_id>? THEN 0 ELSE 1 END,e.candidate_id LIMIT ?`, project, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []m.Evidence{}
	for rows.Next() {
		var r m.Evidence
		if err = rows.Scan(newJSONScanner(&r), &r.TaskID); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func (s *SQLiteStore) MarkTestEvidenceProjected(ctx context.Context, project, candidate string, count int) error {
	if s.readOnly {
		return errors.New("read-only store")
	}
	_, err := s.db.ExecContext(ctx, `UPDATE test_evidence SET projected_artifacts=? WHERE project_identity=? AND candidate_id=? AND projected_artifacts<=? AND json_array_length(payload,'$.artifacts')>=?`, count, project, candidate, count, count)
	return err
}
