package steerline

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const maxConversationBodyBytes = 16 << 20

func (s *SQLiteStore) CommitConversation(ctx context.Context, writes []ConversationWrite, checkpoint ConversationCheckpoint) ([]ConversationEvent, error) {
	if s.readOnly {
		return nil, errors.New("SQLite reader is read-only")
	}
	if len(writes) == 0 && checkpoint.ProjectID == "" {
		return []ConversationEvent{}, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	out := make([]ConversationEvent, 0, len(writes))
	for _, write := range writes {
		ev := write.Event
		if err := ev.validate(); err != nil {
			return nil, err
		}
		if checkpoint.ProjectID != "" && (checkpoint.ProjectID != ev.ProjectID || checkpoint.ExecutionID != ev.ExecutionID || checkpoint.AttemptID != ev.AttemptID || checkpoint.Source != ev.Source || checkpoint.SourceSegment != ev.SourceSegment) {
			return nil, errors.New("conversation event and source checkpoint identities differ")
		}
		key := ev.sourceKey()
		var existingID int64
		err := tx.QueryRowContext(ctx, `SELECT event_id FROM conversation_events WHERE project_identity=? AND source_event_key=?`, ev.ProjectID, key).Scan(&existingID)
		if err == nil {
			var payload []byte
			if err = tx.QueryRowContext(ctx, `SELECT payload FROM conversation_events WHERE event_id=?`, existingID).Scan(&payload); err != nil {
				return nil, err
			}
			var existing ConversationEvent
			if err = json.Unmarshal(payload, &existing); err != nil {
				return nil, fmt.Errorf("decode persisted conversation event %d: %w", existingID, err)
			}
			if existing.Type != ev.Type || existing.Phase != ev.Phase || existing.Status != ev.Status || existing.SessionID != ev.SessionID || existing.TurnID != ev.TurnID || existing.ItemKey != ev.ItemKey || existing.FinalAvailability != ev.FinalAvailability {
				return nil, errors.New("conversation source event identity was reused with different semantics")
			}
			if len(write.Raw) > 0 {
				actual := sha256.Sum256(write.Raw)
				if existing.Raw == nil || existing.Raw.SHA256 != hex.EncodeToString(actual[:]) || existing.Raw.Bytes != int64(len(write.Raw)) {
					return nil, errors.New("conversation source event identity was reused with different raw bytes")
				}
			}
			existing.EventID = existingID
			out = append(out, existing)
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		if len(write.Raw) > 0 {
			if len(write.Raw) > maxConversationBodyBytes {
				return nil, errors.New("conversation raw record exceeds 16 MiB")
			}
			sum := sha256.Sum256(write.Raw)
			rawID := key
			ev.Raw = &ConversationRawRef{ExecutionID: ev.ExecutionID, AttemptID: ev.AttemptID, ID: rawID, SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len(write.Raw))}
			if _, err = tx.ExecContext(ctx, `INSERT INTO conversation_raw(project_identity,execution_id,attempt_id,raw_id,source_event_key,payload,sha256,byte_count,created_at) VALUES(?,?,?,?,?,?,?,?,?)`, ev.ProjectID, ev.ExecutionID, ev.AttemptID, rawID, key, write.Raw, ev.Raw.SHA256, ev.Raw.Bytes, ev.ReceivedAt.UTC().Format(time.RFC3339Nano)); err != nil {
				return nil, err
			}
		}
		if write.BodyMode != "" {
			if len(write.Body) > maxConversationBodyBytes || !utf8.Valid(write.Body) {
				return nil, errors.New("conversation message body is invalid UTF-8 or exceeds 16 MiB")
			}
			if ev.ItemKey == "" {
				return nil, errors.New("conversation body requires a scoped item key")
			}
			bodyRef, diagnostic, bodyErr := commitConversationBody(ctx, tx, write)
			if bodyErr != nil {
				return nil, bodyErr
			}
			ev.Body = bodyRef
			if diagnostic != "" {
				ev.Diagnostic = strings.TrimSpace(ev.Diagnostic + " " + diagnostic)
			}
		}
		payload, err := json.Marshal(ev)
		if err != nil {
			return nil, err
		}
		if len(payload) > MaxEventPageBytes {
			return nil, errors.New("conversation event envelope exceeds the 1 MiB event-page budget")
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO conversation_events(project_identity,task_id,workflow_run_id,execution_id,attempt_id,source_event_key,source,source_segment,session_id,turn_id,item_key,event_type,phase,final_availability,received_at,payload_bytes,payload) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, ev.ProjectID, ev.TaskID, ev.WorkflowID, ev.ExecutionID, ev.AttemptID, key, ev.Source, ev.SourceSegment, ev.SessionID, ev.TurnID, ev.ItemKey, ev.Type, ev.Phase, ev.FinalAvailability, ev.ReceivedAt.UTC().Format(time.RFC3339Nano), len(payload), payload)
		if err != nil {
			return nil, err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return nil, err
		}
		ev.EventID, ev.LocalSeq = id, uint64(id)
		payload, err = json.Marshal(ev)
		if err != nil {
			return nil, err
		}
		if len(payload) > MaxEventPageBytes {
			return nil, errors.New("conversation event with local sequence exceeds the 1 MiB event-page budget")
		}
		if _, err = tx.ExecContext(ctx, `UPDATE conversation_events SET payload_bytes=?,payload=? WHERE event_id=?`, len(payload), payload, id); err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	if checkpoint.ProjectID != "" {
		if checkpoint.Source == "" || checkpoint.SourceSegment == "" || checkpoint.ExecutionID == "" || checkpoint.AttemptID == "" {
			return nil, errors.New("conversation checkpoint is missing bound source identity")
		}
		if checkpoint.UpdatedAt.IsZero() {
			checkpoint.UpdatedAt = now()
		}
		var oldSeq uint64
		err = tx.QueryRowContext(ctx, `SELECT local_seq FROM conversation_source_cursors WHERE project_identity=? AND execution_id=? AND attempt_id=? AND source=? AND source_segment=?`, checkpoint.ProjectID, checkpoint.ExecutionID, checkpoint.AttemptID, checkpoint.Source, checkpoint.SourceSegment).Scan(&oldSeq)
		advanceCheckpoint := true
		if err == nil && checkpoint.LocalSeq < oldSeq {
			if !checkpoint.AllowReplay {
				return nil, errors.New("conversation source checkpoint cannot move backwards")
			}
			advanceCheckpoint = false
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		if advanceCheckpoint {
			_, err = tx.ExecContext(ctx, `INSERT INTO conversation_source_cursors(project_identity,execution_id,attempt_id,source,source_segment,source_identity,generation,cursor,local_seq,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT(project_identity,execution_id,attempt_id,source,source_segment) DO UPDATE SET source_identity=excluded.source_identity,generation=excluded.generation,cursor=excluded.cursor,local_seq=excluded.local_seq,updated_at=excluded.updated_at`, checkpoint.ProjectID, checkpoint.ExecutionID, checkpoint.AttemptID, checkpoint.Source, checkpoint.SourceSegment, checkpoint.SourceIdentity, checkpoint.Generation, checkpoint.Cursor, checkpoint.LocalSeq, checkpoint.UpdatedAt.UTC().Format(time.RFC3339Nano))
			if err != nil {
				return nil, err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}

func commitConversationBody(ctx context.Context, tx *sql.Tx, write ConversationWrite) (*ConversationBodyRef, string, error) {
	ev := write.Event
	var currentRevision int64
	var currentSeq uint64
	var currentBody []byte
	var completeness string
	err := tx.QueryRowContext(ctx, `SELECT revision,source_seq,body,completeness FROM conversation_current_messages WHERE project_identity=? AND execution_id=? AND attempt_id=? AND item_key=?`, ev.ProjectID, ev.ExecutionID, ev.AttemptID, ev.ItemKey).Scan(&currentRevision, &currentSeq, &currentBody, &completeness)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, "", err
	}
	if errors.Is(err, sql.ErrNoRows) {
		currentRevision, currentSeq = 0, 0
		currentBody = nil
		completeness = string(BodyPartial)
	}
	var order uint64
	if ev.SourceRevision > 0 {
		order = uint64(ev.SourceRevision)
	} else {
		order = ev.SourceSeq
	}
	if order == 0 {
		order = ev.LocalSeq
	}
	if currentRevision > 0 && (order == 0 || currentSeq > order) {
		ref, err := conversationBodyRef(ctx, tx, ev.ProjectID, ev.ExecutionID, ev.AttemptID, ev.ItemKey, currentRevision)
		return ref, "source revision arrived out of order; prior message revision retained", err
	}
	if currentRevision > 0 && currentSeq == order && write.BodyMode == "delta" {
		ref, err := conversationBodyRef(ctx, tx, ev.ProjectID, ev.ExecutionID, ev.AttemptID, ev.ItemKey, currentRevision)
		return ref, "delta source order conflicts with an existing message revision; prior revision retained", err
	}
	if currentRevision > 0 && (order == 0 || currentSeq == order) && (write.BodyMode == "snapshot" || write.BodyMode == "complete") && string(currentBody) != string(write.Body) {
		ref, err := conversationBodyRef(ctx, tx, ev.ProjectID, ev.ExecutionID, ev.AttemptID, ev.ItemKey, currentRevision)
		return ref, "conflicting message revisions share source order; prior revision retained", err
	}
	next := append([]byte(nil), currentBody...)
	nextCompleteness := BodyPartial
	switch write.BodyMode {
	case "delta":
		next = append(next, write.Body...)
	case "snapshot":
		next = append(next[:0], write.Body...)
	case "complete":
		next = append(next[:0], write.Body...)
		nextCompleteness = BodyComplete
	default:
		return nil, "", fmt.Errorf("unsupported conversation body update %q", write.BodyMode)
	}
	revision := currentRevision + 1
	sum := sha256.Sum256(next)
	digest := hex.EncodeToString(sum[:])
	created := ev.ReceivedAt.UTC().Format(time.RFC3339Nano)
	if _, err = tx.ExecContext(ctx, `INSERT INTO conversation_bodies(project_identity,execution_id,attempt_id,item_key,revision,body,sha256,byte_count,source,completeness,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, ev.ProjectID, ev.ExecutionID, ev.AttemptID, ev.ItemKey, revision, next, digest, len(next), nonEmpty(write.BodySource, ev.Source), string(nextCompleteness), created); err != nil {
		return nil, "", err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO conversation_current_messages(project_identity,execution_id,attempt_id,item_key,revision,source_seq,body,completeness) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(project_identity,execution_id,attempt_id,item_key) DO UPDATE SET revision=excluded.revision,source_seq=excluded.source_seq,body=excluded.body,completeness=excluded.completeness`, ev.ProjectID, ev.ExecutionID, ev.AttemptID, ev.ItemKey, revision, order, next, string(nextCompleteness))
	if err != nil {
		return nil, "", err
	}
	return &ConversationBodyRef{ExecutionID: ev.ExecutionID, AttemptID: ev.AttemptID, ItemKey: ev.ItemKey, Revision: revision, SHA256: digest, Bytes: int64(len(next)), Source: nonEmpty(write.BodySource, ev.Source), Completeness: nextCompleteness}, "", nil
}

func conversationBodyRef(ctx context.Context, tx *sql.Tx, project, execution, attempt, item string, revision int64) (*ConversationBodyRef, error) {
	var ref ConversationBodyRef
	var complete string
	err := tx.QueryRowContext(ctx, `SELECT sha256,byte_count,source,completeness FROM conversation_bodies WHERE project_identity=? AND execution_id=? AND attempt_id=? AND item_key=? AND revision=?`, project, execution, attempt, item, revision).Scan(&ref.SHA256, &ref.Bytes, &ref.Source, &complete)
	if err != nil {
		return nil, err
	}
	ref.ExecutionID, ref.AttemptID = execution, attempt
	ref.ItemKey, ref.Revision, ref.Completeness = item, revision, BodyCompleteness(complete)
	return &ref, nil
}

type ConversationFilter struct {
	WorkflowID  string `json:"workflow_id,omitempty"`
	ExecutionID string `json:"execution_id,omitempty"`
	AttemptID   string `json:"attempt_id,omitempty"`
	SessionID   string `json:"session_id,omitempty"`
	TurnID      string `json:"turn_id,omitempty"`
	Source      string `json:"source,omitempty"`
}

// A verified native session supersedes the lossy raw projection for an attempt.
// Its source_changed event tells incremental viewers to replace that projection.
// Explicit Source queries retain access to every raw observation and raw ref.
func conversationSourcePredicate(filter ConversationFilter, args []any) (string, []any) {
	if filter.Source != "" {
		return ` AND source=?`, append(args, filter.Source)
	}
	return ` AND (event_type IN ('gap','input.wait') OR source NOT IN ('codex-jsonl','orca-terminal-v2','orca-reply-receipt') OR NOT EXISTS (SELECT 1 FROM conversation_events native WHERE native.project_identity=conversation_events.project_identity AND native.execution_id=conversation_events.execution_id AND native.attempt_id=conversation_events.attempt_id AND native.source='codex-app-server' AND native.event_type='session'))`, args
}

type ConversationPage struct {
	Events       []ConversationEvent `json:"events"`
	NextCursor   string              `json:"next_cursor,omitempty"`
	HasMore      bool                `json:"has_more"`
	PayloadBytes int                 `json:"payload_bytes"`
}

type ConversationBodyPage struct {
	ItemKey      string           `json:"item_key"`
	Revision     int64            `json:"revision"`
	Offset       int64            `json:"offset"`
	NextOffset   int64            `json:"next_offset"`
	TotalBytes   int64            `json:"total_bytes"`
	SHA256       string           `json:"sha256"`
	Completeness BodyCompleteness `json:"completeness"`
	Complete     bool             `json:"complete"`
	Data         []byte           `json:"data"`
}

type ConversationRawPage struct {
	RawID      string `json:"raw_id"`
	Offset     int64  `json:"offset"`
	NextOffset int64  `json:"next_offset"`
	TotalBytes int64  `json:"total_bytes"`
	SHA256     string `json:"sha256"`
	Complete   bool   `json:"complete"`
	Data       []byte `json:"data"`
}

type conversationQueryCursor struct {
	Version int                `json:"v"`
	Project string             `json:"p"`
	Filter  ConversationFilter `json:"f"`
	After   int64              `json:"n"`
}

type ConversationQueryService struct {
	Store   *SQLiteStore
	Project string
}

func (q ConversationQueryService) ReadConversation(ctx context.Context, filter ConversationFilter, cursor string, limit int) (ConversationPage, error) {
	if q.Store == nil || q.Project == "" {
		return ConversationPage{}, errors.New("conversation query requires a store and project identity")
	}
	if limit < 1 {
		limit = DefaultEventPageSize
	}
	if limit > MaxEventPageSize {
		limit = MaxEventPageSize
	}
	after := int64(0)
	if cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil {
			return ConversationPage{}, &CursorError{Code: "cursor_invalidated"}
		}
		var saved conversationQueryCursor
		if json.Unmarshal(raw, &saved) != nil || saved.Version != 1 || saved.Project != q.Project || saved.Filter != filter || saved.After < 0 {
			return ConversationPage{}, &CursorError{Code: "cursor_invalidated"}
		}
		after = saved.After
	}
	page := ConversationPage{Events: []ConversationEvent{}}
	for len(page.Events) < limit {
		query := `SELECT event_id,payload_bytes,payload FROM conversation_events WHERE project_identity=? AND event_id>?`
		args := []any{q.Project, after}
		sourcePredicate, args := conversationSourcePredicate(filter, args)
		query += sourcePredicate
		if filter.WorkflowID != "" {
			query += ` AND workflow_run_id=?`
			args = append(args, filter.WorkflowID)
		}
		if filter.ExecutionID != "" {
			query += ` AND execution_id=?`
			args = append(args, filter.ExecutionID)
		}
		if filter.AttemptID != "" {
			query += ` AND attempt_id=?`
			args = append(args, filter.AttemptID)
		}
		if filter.SessionID != "" {
			query += ` AND session_id=?`
			args = append(args, filter.SessionID)
		}
		if filter.TurnID != "" {
			query += ` AND turn_id=?`
			args = append(args, filter.TurnID)
		}
		query += ` ORDER BY event_id LIMIT 1`
		var id int64
		var payloadBytes int
		var payload []byte
		err := q.Store.db.QueryRowContext(ctx, query, args...).Scan(&id, &payloadBytes, &payload)
		if errors.Is(err, sql.ErrNoRows) {
			break
		}
		if err != nil {
			return ConversationPage{}, err
		}
		if len(page.Events) > 0 && page.PayloadBytes+payloadBytes > MaxEventPageBytes {
			page.HasMore = true
			break
		}
		var event ConversationEvent
		if err = json.Unmarshal(payload, &event); err != nil {
			return ConversationPage{}, fmt.Errorf("decode conversation event %d: %w", id, err)
		}
		event.EventID = id
		page.Events = append(page.Events, event)
		page.PayloadBytes += payloadBytes
		after = id
	}
	if len(page.Events) > 0 {
		if !page.HasMore && len(page.Events) == limit {
			probe, err := q.readOne(ctx, filter, after)
			if err != nil {
				return ConversationPage{}, err
			}
			page.HasMore = probe != nil
		}
		if page.HasMore {
			raw, _ := json.Marshal(conversationQueryCursor{Version: 1, Project: q.Project, Filter: filter, After: page.Events[len(page.Events)-1].EventID})
			page.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
		}
	}
	return page, nil
}

func (q ConversationQueryService) readOne(ctx context.Context, filter ConversationFilter, after int64) (*ConversationEvent, error) {
	query := `SELECT event_id,payload FROM conversation_events WHERE project_identity=? AND event_id>?`
	args := []any{q.Project, after}
	sourcePredicate, args := conversationSourcePredicate(filter, args)
	query += sourcePredicate
	if filter.WorkflowID != "" {
		query += ` AND workflow_run_id=?`
		args = append(args, filter.WorkflowID)
	}
	if filter.ExecutionID != "" {
		query += ` AND execution_id=?`
		args = append(args, filter.ExecutionID)
	}
	if filter.AttemptID != "" {
		query += ` AND attempt_id=?`
		args = append(args, filter.AttemptID)
	}
	if filter.SessionID != "" {
		query += ` AND session_id=?`
		args = append(args, filter.SessionID)
	}
	if filter.TurnID != "" {
		query += ` AND turn_id=?`
		args = append(args, filter.TurnID)
	}
	query += ` ORDER BY event_id LIMIT 1`
	var id int64
	var payload []byte
	if err := q.Store.db.QueryRowContext(ctx, query, args...).Scan(&id, &payload); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	var event ConversationEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return nil, err
	}
	event.EventID = id
	return &event, nil
}

func (q ConversationQueryService) ReadMessageBody(ctx context.Context, ref ConversationBodyRef, offset int64, limit int) (ConversationBodyPage, error) {
	if q.Store == nil || q.Project == "" || ref.ItemKey == "" || ref.Revision < 1 || offset < 0 {
		return ConversationBodyPage{}, errors.New("message body reference or offset is invalid")
	}
	if limit < 1 {
		limit = 256 << 10
	}
	if limit > 256<<10 {
		limit = 256 << 10
	}
	if limit < utf8.UTFMax {
		limit = utf8.UTFMax
	}
	var body []byte
	var total int64
	var digest, complete string
	err := q.Store.db.QueryRowContext(ctx, `SELECT body,byte_count,sha256,completeness FROM conversation_bodies WHERE project_identity=? AND execution_id=? AND attempt_id=? AND item_key=? AND revision=?`, q.Project, ref.ExecutionID, ref.AttemptID, ref.ItemKey, ref.Revision).Scan(&body, &total, &digest, &complete)
	if err != nil {
		return ConversationBodyPage{}, err
	}
	actual := sha256.Sum256(body)
	if total != int64(len(body)) || digest != hex.EncodeToString(actual[:]) || !utf8.Valid(body) {
		return ConversationBodyPage{}, errors.New("message body revision is corrupt")
	}
	if offset > total {
		return ConversationBodyPage{}, &CursorError{Code: "cursor_invalidated"}
	}
	if digest != ref.SHA256 || total != ref.Bytes || BodyCompleteness(complete) != ref.Completeness {
		return ConversationBodyPage{}, errors.New("message body revision does not match the supplied reference")
	}
	end := offset + int64(limit)
	if end > total {
		end = total
	}
	data := body[offset:end]
	if len(data) > 0 && data[0]&0xC0 == 0x80 {
		return ConversationBodyPage{}, &CursorError{Code: "cursor_invalidated"}
	}
	data = utf8SafePrefix(data)
	return ConversationBodyPage{ItemKey: ref.ItemKey, Revision: ref.Revision, Offset: offset, NextOffset: offset + int64(len(data)), TotalBytes: total, SHA256: digest, Completeness: BodyCompleteness(complete), Complete: offset+int64(len(data)) >= total, Data: data}, nil
}

func (q ConversationQueryService) ReadRaw(ctx context.Context, ref ConversationRawRef, offset int64, limit int) (ConversationRawPage, error) {
	if q.Store == nil || q.Project == "" || ref.ID == "" || ref.ExecutionID == "" || ref.AttemptID == "" || offset < 0 {
		return ConversationRawPage{}, errors.New("raw reference or offset is invalid")
	}
	if limit < 1 {
		limit = MaxEventPageBytes
	}
	if limit > MaxEventPageBytes {
		limit = MaxEventPageBytes
	}
	var body []byte
	var total int64
	var digest string
	err := q.Store.db.QueryRowContext(ctx, `SELECT payload,byte_count,sha256 FROM conversation_raw WHERE project_identity=? AND execution_id=? AND attempt_id=? AND raw_id=?`, q.Project, ref.ExecutionID, ref.AttemptID, ref.ID).Scan(&body, &total, &digest)
	if err != nil {
		return ConversationRawPage{}, err
	}
	actual := sha256.Sum256(body)
	if total != int64(len(body)) || digest != hex.EncodeToString(actual[:]) {
		return ConversationRawPage{}, errors.New("raw conversation record is corrupt")
	}
	if offset > total {
		return ConversationRawPage{}, &CursorError{Code: "cursor_invalidated"}
	}
	if ref.SHA256 != "" && (digest != ref.SHA256 || total != ref.Bytes) {
		return ConversationRawPage{}, errors.New("raw body does not match the supplied reference")
	}
	end := offset + int64(limit)
	if end > total {
		end = total
	}
	data := body[offset:end]
	return ConversationRawPage{RawID: ref.ID, Offset: offset, NextOffset: end, TotalBytes: total, SHA256: digest, Complete: end >= total, Data: data}, nil
}

func (s *SQLiteStore) ConversationCheckpoint(ctx context.Context, project, execution, attempt, source, segment string) (ConversationCheckpoint, error) {
	var checkpoint ConversationCheckpoint
	var updated string
	err := s.db.QueryRowContext(ctx, `SELECT project_identity,execution_id,attempt_id,source,source_segment,source_identity,generation,cursor,local_seq,updated_at FROM conversation_source_cursors WHERE project_identity=? AND execution_id=? AND attempt_id=? AND source=? AND source_segment=?`, project, execution, attempt, source, segment).Scan(&checkpoint.ProjectID, &checkpoint.ExecutionID, &checkpoint.AttemptID, &checkpoint.Source, &checkpoint.SourceSegment, &checkpoint.SourceIdentity, &checkpoint.Generation, &checkpoint.Cursor, &checkpoint.LocalSeq, &updated)
	if err != nil {
		return ConversationCheckpoint{}, err
	}
	checkpoint.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return checkpoint, nil
}

func (s *SQLiteStore) ConversationSourceCheckpoints(ctx context.Context, project, execution, attempt, source string) ([]ConversationCheckpoint, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT source_segment,source_identity,generation,cursor,local_seq,updated_at FROM conversation_source_cursors WHERE project_identity=? AND execution_id=? AND attempt_id=? AND source=? ORDER BY updated_at`, project, execution, attempt, source)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ConversationCheckpoint{}
	for rows.Next() {
		var checkpoint ConversationCheckpoint
		var updated string
		if err = rows.Scan(&checkpoint.SourceSegment, &checkpoint.SourceIdentity, &checkpoint.Generation, &checkpoint.Cursor, &checkpoint.LocalSeq, &updated); err != nil {
			return nil, err
		}
		checkpoint.ProjectID, checkpoint.ExecutionID, checkpoint.AttemptID, checkpoint.Source = project, execution, attempt, source
		checkpoint.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		out = append(out, checkpoint)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) RestoreConversationMapper(ctx context.Context, project, execution, attempt, source, segment string) (*ConversationMapper, error) {
	mapper := &ConversationMapper{turnFinalSeen: map[string]bool{}}
	var payload []byte
	err := s.db.QueryRowContext(ctx, `SELECT payload FROM conversation_events WHERE project_identity=? AND execution_id=? AND attempt_id=? AND source=? AND source_segment=? AND event_type=? ORDER BY event_id DESC LIMIT 1`, project, execution, attempt, source, segment, ConversationSession).Scan(&payload)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err == nil {
		var event ConversationEvent
		if err = json.Unmarshal(payload, &event); err != nil {
			return nil, err
		}
		mapper.sessionID, mapper.sessionQuality = event.SessionID, event.SessionQuality
	}
	payload = nil
	err = s.db.QueryRowContext(ctx, `SELECT payload FROM conversation_events WHERE project_identity=? AND execution_id=? AND attempt_id=? AND source=? AND source_segment=? AND event_type=? ORDER BY event_id DESC LIMIT 1`, project, execution, attempt, source, segment, ConversationTurnStarted).Scan(&payload)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err == nil {
		var event ConversationEvent
		if err = json.Unmarshal(payload, &event); err != nil {
			return nil, err
		}
		mapper.currentTurn, mapper.turnQuality = event.TurnID, event.TurnQuality
		var count int
		err = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM conversation_events WHERE project_identity=? AND execution_id=? AND attempt_id=? AND source=? AND source_segment=? AND turn_id=? AND event_type=? AND phase=?`, project, execution, attempt, source, segment, mapper.currentTurn, ConversationMessageComplete, MessagePhaseFinalAnswer).Scan(&count)
		if err != nil {
			return nil, err
		}
		mapper.turnFinalSeen[mapper.currentTurn] = count > 0
	}
	return mapper, nil
}

func (s *SQLiteStore) SaveConversationCapabilityReport(ctx context.Context, report ConversationCapabilityReport) error {
	if s.readOnly {
		return errors.New("SQLite reader is read-only")
	}
	if report.Version != 1 || report.Backend == "" || report.Protocol == "" || report.Agent == "" || report.Target.ID == "" || report.CheckedAt.IsZero() || len(report.Capabilities) == 0 {
		return errors.New("conversation capability report is incomplete")
	}
	if report.Source != "" && (report.ExecutionID == "" || report.AttemptID == "" || report.SourceIdentity == "") {
		return errors.New("source-scoped conversation capability report is incomplete")
	}
	seen := map[string]bool{}
	for _, capability := range report.Capabilities {
		if capability.Name == "" || seen[capability.Name] {
			return errors.New("conversation capability report has an empty or duplicate capability")
		}
		seen[capability.Name] = true
		switch capability.Status {
		case CapabilityVerified, CapabilityUnsupported, CapabilityNotTested:
		default:
			return fmt.Errorf("invalid conversation capability status %q", capability.Status)
		}
		if capability.Status == CapabilityVerified && capability.Evidence == "" {
			return fmt.Errorf("verified conversation capability %q requires evidence", capability.Name)
		}
	}
	payload, err := json.Marshal(report)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO conversation_capability_reports(backend,protocol,agent,agent_version,platform_version,target_id,checked_at,payload) VALUES(?,?,?,?,?,?,?,?)`, report.Backend, report.Protocol, report.Agent, report.AgentVersion, report.PlatformVersion, report.Target.ID, report.CheckedAt.UTC().Format(time.RFC3339Nano), payload)
	return err
}

func (s *SQLiteStore) ConversationCapabilityReports(ctx context.Context, backend BackendKind, protocol ExecutionProtocol, targetID string, limit int) ([]ConversationCapabilityReport, error) {
	if limit < 1 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	query := `SELECT payload FROM conversation_capability_reports WHERE 1=1`
	args := []any{}
	if backend != "" {
		query += ` AND backend=?`
		args = append(args, backend)
	}
	if protocol != "" {
		query += ` AND protocol=?`
		args = append(args, protocol)
	}
	if targetID != "" {
		query += ` AND target_id=?`
		args = append(args, targetID)
	}
	query += ` ORDER BY report_id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	reports := []ConversationCapabilityReport{}
	for rows.Next() {
		var payload []byte
		if err = rows.Scan(&payload); err != nil {
			return nil, err
		}
		var report ConversationCapabilityReport
		if err = json.Unmarshal(payload, &report); err != nil {
			return nil, err
		}
		reports = append(reports, report)
	}
	return reports, rows.Err()
}

func utf8SafePrefix(data []byte) []byte {
	for len(data) > 0 && !utf8.Valid(data) {
		data = data[:len(data)-1]
	}
	return data
}
