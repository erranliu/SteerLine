package sqlite

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	b "github.com/erranliu/SteerLine/internal/execution/backend"
	m "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	p "github.com/erranliu/SteerLine/internal/storage/ports"
)

// Reads return immutable public CLI facts. Product conversation mapping lives
// in query; no native log, private route or second conversation writer is read.
func (s Reader) NextConversationEvent(ctx context.Context, project string, f p.ConversationFilter, after int64) (*p.ConversationEventRecord, error) {
	q := `SELECT e.seq,c.candidate_id,c.task_id,c.run_id,e.payload FROM runner_events e JOIN runner_candidates c ON c.project_identity=e.project_identity AND c.candidate_id=e.candidate_id WHERE e.project_identity=? AND e.seq>? AND json_extract(e.payload,'$.final.verified')=1 AND coalesce(json_extract(e.payload,'$.final.text'),'')<>''`
	args := []any{project, after}
	for _, v := range []struct {
		sql   string
		value string
	}{{"c.candidate_id", f.WorkflowID}, {"c.run_id", f.ExecutionID}, {"json_extract(e.payload,'$.final.attempt_id')", f.AttemptID}, {"json_extract(e.payload,'$.final.session_id')", f.SessionID}, {"json_extract(e.payload,'$.final.turn_id')", f.TurnID}, {"json_extract(e.payload,'$.final.source')", f.Source}} {
		if v.value != "" {
			q += " AND " + v.sql + "=?"
			args = append(args, v.value)
		}
	}
	q += ` ORDER BY e.seq LIMIT 1`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, rows.Err()
	}
	var seq int64
	var candidate, task, run string
	var raw []byte
	if err = rows.Scan(&seq, &candidate, &task, &run, &raw); err != nil {
		return nil, err
	}
	var event runnercontract.Event
	if err = json.Unmarshal(raw, &event); err != nil {
		return nil, err
	}
	return &p.ConversationEventRecord{EventID: seq, ProjectID: project, TaskID: task, CandidateID: candidate, RunID: run, Event: event}, nil
}
func (s Reader) ConversationBody(ctx context.Context, project string, ref m.ConversationBodyRef, limit int) (p.ConversationBodyRecord, error) {
	var raw []byte
	err := s.db.QueryRowContext(ctx, `SELECT e.payload FROM runner_events e JOIN runner_candidates c ON c.project_identity=e.project_identity AND c.candidate_id=e.candidate_id WHERE e.project_identity=? AND c.run_id=? AND e.event_id=?`, project, ref.ExecutionID, ref.ItemKey).Scan(&raw)
	if err != nil {
		return p.ConversationBodyRecord{}, err
	}
	var e runnercontract.Event
	if err = json.Unmarshal(raw, &e); err != nil {
		return p.ConversationBodyRecord{}, err
	}
	f := e.Final
	if f == nil || !f.Verified || f.AttemptID != ref.AttemptID || f.Source != ref.Source || ref.Revision != 1 {
		return p.ConversationBodyRecord{}, errors.New("original Runner final reference changed")
	}
	body := []byte(f.Text)
	sum := sha256.Sum256(body)
	hash := hex.EncodeToString(sum[:])
	if hash != ref.SHA256 || int64(len(body)) != ref.Bytes {
		return p.ConversationBodyRecord{}, errors.New("original Runner final digest differs")
	}
	result := p.ConversationBodyRecord{TotalBytes: int64(len(body)), SHA256: hash, Completeness: string(m.BodyComplete)}
	if limit < 0 {
		limit = 0
	}
	result.Body = body[:min(len(body), limit)]
	return result, nil
}
func (s Reader) ConversationCapabilityReports(context.Context, b.BackendKind, b.ExecutionProtocol, string, int) ([]m.ConversationCapabilityReport, error) {
	return []m.ConversationCapabilityReport{}, nil
}
