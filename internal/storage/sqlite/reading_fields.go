package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstate"
	"github.com/erranliu/SteerLine/internal/storage/ports"
)

type readingWriter interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

type readingField struct {
	source, sourceID, id, origin, originID, field                          string
	role, kind, label, text, at, state, run, attempt, session, turn, group string
}

func readingPrefix(s string, n int) string {
	if len(s) <= n {
		return s
	}
	s = s[:n]
	for !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}

// Only metadata is indexed, in the same transaction as its original owner.
func putReadingField(ctx context.Context, tx readingWriter, project string, f readingField) error {
	if f.at == "0001-01-01T00:00:00Z" {
		f.at = ""
	}
	if f.text == "" || !utf8.ValidString(f.text) {
		return nil
	}
	sum := sha256.Sum256([]byte(f.text))
	result, err := tx.ExecContext(ctx, `INSERT INTO reading_fields(project_identity,source,source_id,item_id,origin,origin_id,field,role,kind,label,preview,body_bytes,body_sha256,at,state,run_id,attempt_id,session_id,turn_id,group_name)
	VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(project_identity,source,source_id,item_id) DO UPDATE SET
	preview=excluded.preview,body_bytes=excluded.body_bytes,body_sha256=excluded.body_sha256,state=excluded.state,at=excluded.at,
	run_id=excluded.run_id,attempt_id=excluded.attempt_id,session_id=excluded.session_id,turn_id=excluded.turn_id,group_name=excluded.group_name
	WHERE reading_fields.body_sha256<>excluded.body_sha256 OR reading_fields.state<>excluded.state OR reading_fields.at<>excluded.at
	OR reading_fields.run_id<>excluded.run_id OR reading_fields.attempt_id<>excluded.attempt_id
	OR reading_fields.session_id<>excluded.session_id OR reading_fields.turn_id<>excluded.turn_id OR reading_fields.group_name<>excluded.group_name`,
		project, f.source, f.sourceID, f.id, f.origin, f.originID, f.field, f.role, f.kind, f.label, readingPrefix(f.text, 512), len(f.text), hex.EncodeToString(sum[:]), f.at, f.state, f.run, f.attempt, f.session, f.turn, f.group)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil || changed == 0 {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO reading_versions VALUES(?,?,?,1) ON CONFLICT(project_identity,source,source_id) DO UPDATE SET revision=reading_versions.revision+1`, project, f.source, f.sourceID)
	return err
}

func indexCandidateReading(ctx context.Context, tx readingWriter, old, c runnerstate.Candidate, events []runnercontract.Event) error {
	for i, a := range c.Actions {
		if i < len(old.Actions) && old.Actions[i].State == a.State && old.Actions[i].ActionStatus == a.ActionStatus {
			continue
		}
		if a.Command != "reply" && a.Command != "message" {
			continue
		}
		var input struct{ Text, Kind string }
		if json.Unmarshal(a.Request.Input, &input) != nil || input.Text == "" {
			continue
		}
		kind := input.Kind
		if kind != "feedback" && kind != "answer" {
			kind = a.Command
		}
		if err := putReadingField(ctx, tx, c.Project, readingField{source: "task_candidate", sourceID: c.ID, id: "input:" + a.Request.RequestID, origin: "candidate", originID: c.ID, field: fmt.Sprintf("$.actions[%d].request.input.text", i), role: "user", kind: kind, text: input.Text, state: strings.Trim(a.State+" / "+a.ActionStatus, " /")}); err != nil {
			return err
		}
	}
	for _, e := range events {
		f := e.Final
		if f == nil || !f.Verified || f.Text == "" || f.AttemptID == "" || f.SessionID == "" || f.TurnID == "" || f.Source == "" || f.ItemID == "" {
			continue
		}
		if err := putReadingField(ctx, tx, c.Project, readingField{source: "task_candidate", sourceID: c.ID, id: "final:" + e.EventID, origin: "event", originID: e.EventID, field: "$.final.text", role: "assistant", kind: "verified_final", text: f.Text, at: e.CreatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"), run: c.RunID, attempt: f.AttemptID, session: f.SessionID, turn: f.TurnID}); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteStore) ReadReadingWindow(ctx context.Context, project, source, id string, after, end int64, limit int) (ports.ReadingWindow, error) {
	var out ports.ReadingWindow
	if limit < 1 || limit > 200 {
		return out, errors.New("invalid reading page size")
	}
	db, err := s.asyncActivityPool()
	if err != nil {
		return out, err
	}
	if end == 0 {
		if err = db.QueryRowContext(ctx, `SELECT COALESCE(MAX(seq),0) FROM reading_fields WHERE project_identity=? AND source=? AND source_id=?`, project, source, id).Scan(&end); err != nil {
			return out, err
		}
	}
	out.End = end
	rows, err := db.QueryContext(ctx, `SELECT seq,item_id,role,kind,label,preview,body_bytes,body_sha256,at,state,run_id,attempt_id,session_id,turn_id,group_name FROM reading_fields WHERE project_identity=? AND source=? AND source_id=? AND seq>? AND seq<=? ORDER BY seq LIMIT ?`, project, source, id, after, end, limit+1)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var r ports.ReadingRecord
		if err = rows.Scan(&r.Seq, &r.ID, &r.Role, &r.Kind, &r.Label, &r.Preview, &r.Bytes, &r.SHA256, &r.At, &r.State, &r.RunID, &r.AttemptID, &r.SessionID, &r.TurnID, &r.Group); err != nil {
			return out, err
		}
		if len(out.Records) == limit {
			out.More = true
			break
		}
		out.Records = append(out.Records, r)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) ReadingVersion(ctx context.Context, project, source, id string) (string, error) {
	db, err := s.asyncActivityPool()
	if err != nil {
		return "", err
	}
	var version string
	switch source {
	case "task_candidate":
		err = db.QueryRowContext(ctx, `SELECT CAST(c.revision AS TEXT)||':'||COALESCE((SELECT revision FROM reading_versions v WHERE v.project_identity=c.project_identity AND v.source='task_candidate' AND v.source_id=c.candidate_id),0) FROM runner_candidates c WHERE project_identity=? AND candidate_id=?`, project, id).Scan(&version)
	case "environment_detection":
		err = db.QueryRowContext(ctx, `SELECT CAST(revision AS TEXT) FROM project_environments WHERE project_identity=? AND activity_id=?`, project, id).Scan(&version)
	case "roadmap":
		err = db.QueryRowContext(ctx, `SELECT CAST(revision AS TEXT) FROM roadmaps WHERE project_identity=? AND roadmap_id=?`, project, id).Scan(&version)
	case "main_agent_advisory":
		err = db.QueryRowContext(ctx, `SELECT CAST(revision AS TEXT) FROM advisory_jobs WHERE project_identity=? AND request_id=?`, project, id).Scan(&version)
	case "refactoring":
		err = db.QueryRowContext(ctx, `SELECT CAST(revision AS TEXT) FROM refactoring_actions WHERE project_identity=? AND action_id=?`, project, id).Scan(&version)
	case "baseline_check":
		err = db.QueryRowContext(ctx, `SELECT status||':'||updated_at FROM commands WHERE project_identity=? AND command_id=? AND kind='project_maintenance' AND json_extract(payload,'$.action')='maintenance_check'`, project, id).Scan(&version)
	default:
		return "", errors.New("unknown reading source")
	}
	return version, err
}

// A source pointer is chosen from stored metadata, never from an HTTP field/path.
func readingOrigin(origin string) (table, key string) {
	switch origin {
	case "candidate":
		return "runner_candidates", "candidate_id"
	case "event":
		return "runner_events", "event_id"
	case "environment":
		return "project_environments", "activity_id"
	case "roadmap":
		return "roadmaps", "roadmap_id"
	case "advisory":
		return "advisory_jobs", "request_id"
	case "refactoring":
		return "refactoring_actions", "action_id"
	}
	return "", ""
}

func (s *SQLiteStore) ReadingBodyIdentity(ctx context.Context, project, source, id, item string) (ports.ReadingRecord, error) {
	db, err := s.asyncActivityPool()
	if err != nil {
		return ports.ReadingRecord{}, err
	}
	var r ports.ReadingRecord
	var origin, originID string
	err = db.QueryRowContext(ctx, `SELECT item_id,body_bytes,body_sha256,origin,origin_id FROM reading_fields WHERE project_identity=? AND source=? AND source_id=? AND item_id=?`, project, source, id, item).Scan(&r.ID, &r.Bytes, &r.SHA256, &origin, &originID)
	if err != nil {
		return r, err
	}
	table, key := readingOrigin(origin)
	if table == "" {
		return r, errors.New("unknown reading origin")
	}
	statement := `SELECT 1 FROM ` + table + ` WHERE project_identity=? AND ` + key + `=?`
	args := []any{project, originID}
	if origin == "event" {
		statement += ` AND candidate_id=?`
		args = append(args, id)
	}
	if origin == "advisory" && source == "task_candidate" {
		statement += ` AND candidate_id=?`
		args = append(args, id)
	}
	var exists int
	err = db.QueryRowContext(ctx, statement, args...).Scan(&exists)
	return r, err
}

func (s *SQLiteStore) ReadReadingBody(ctx context.Context, project, source, id, item string, limit int) (string, error) {
	db, err := s.asyncActivityPool()
	if err != nil {
		return "", err
	}
	var origin, originID, field string
	if err = db.QueryRowContext(ctx, `SELECT origin,origin_id,field FROM reading_fields WHERE project_identity=? AND source=? AND source_id=? AND item_id=?`, project, source, id, item).Scan(&origin, &originID, &field); err != nil {
		return "", err
	}
	table, key := readingOrigin(origin)
	if table == "" {
		return "", errors.New("unknown reading origin")
	}
	// Byte slicing in SQL bounds transfer even for an oversized original field.
	q := `SELECT CAST(substr(CAST(json_extract(payload,?) AS BLOB),1,?) AS TEXT) FROM ` + table + ` WHERE project_identity=? AND ` + key + `=?`
	args := []any{field, limit + 1, project, originID}
	if origin == "event" {
		q += ` AND candidate_id=?`
		args = append(args, id)
	}
	var body sql.NullString
	if err = db.QueryRowContext(ctx, q, args...).Scan(&body); err != nil {
		return "", err
	}
	if !body.Valid || len(body.String) > limit {
		return "", errors.New("reading body missing or exceeds limit")
	}
	return body.String, nil
}
