package sqlite

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	b "github.com/erranliu/SteerLine/internal/execution/backend"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	p "github.com/erranliu/SteerLine/internal/storage/ports"
)

// Finals are projections of immutable public CLI events. No native log, private
// route, parser, source cursor or second conversation writer exists here.
func (s Reader) NextConversationEvent(ctx context.Context, project string, f api.ConversationFilter, after int64) (*p.ConversationEventRecord, error) {
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
	v := event.Final
	if v == nil || event.RunID != run || v.AttemptID == "" || v.SessionID == "" || v.TurnID == "" || v.Source == "" || v.ItemID == "" {
		return nil, errors.New("public Runner final lacks its original provenance")
	}
	sum := sha256.Sum256([]byte(v.Text))
	body := &api.ConversationBodyRef{ExecutionID: run, AttemptID: v.AttemptID, ItemKey: event.EventID, Revision: 1, SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len([]byte(v.Text))), Source: v.Source, Completeness: api.BodyComplete}
	e := api.ConversationEvent{Version: api.ConversationVersion, EventID: seq, ProjectID: project, TaskID: task, WorkflowID: candidate, ExecutionID: run, AttemptID: v.AttemptID, Source: v.Source, SourceRole: api.ConversationSourceCanonical, SourceSegment: run, SourceIdentity: run, SessionID: v.SessionID, SessionQuality: api.IdentityNative, TurnID: v.TurnID, TurnQuality: api.IdentityNative, ItemID: v.ItemID, ItemKey: event.EventID, ItemQuality: api.IdentityNative, SourceEventID: event.EventID, SourceEventQuality: api.IdentityDerived, Type: api.ConversationMessageComplete, Phase: api.MessagePhaseFinalAnswer, FinalAvailability: api.FinalAvailable, Body: body, ReceivedAt: event.CreatedAt, Diagnostic: v.Diagnostic}
	payload, err := json.Marshal(e)
	if err != nil {
		return nil, err
	}
	return &p.ConversationEventRecord{EventID: seq, PayloadBytes: len(payload), Payload: payload}, nil
}
func (s Reader) ConversationBody(ctx context.Context, project string, ref api.ConversationBodyRef, limit int) (p.ConversationBodyRecord, error) {
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
	result := p.ConversationBodyRecord{TotalBytes: int64(len(body)), SHA256: hash, Completeness: string(api.BodyComplete)}
	if limit < 0 {
		limit = 0
	}
	result.Body = body[:min(len(body), limit)]
	return result, nil
}
func (s Reader) ConversationCapabilityReports(context.Context, b.BackendKind, b.ExecutionProtocol, string, int) ([]api.ConversationCapabilityReport, error) {
	return []api.ConversationCapabilityReport{}, nil
}
