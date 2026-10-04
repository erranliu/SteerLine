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
	"time"
	"unicode/utf8"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

const maxConversationBodyBytes = contracts.MaxConversationBodyBytes

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
		if err := ev.Validate(); err != nil {
			return nil, err
		}
		if checkpoint.ProjectID != "" && (checkpoint.ProjectID != ev.ProjectID || checkpoint.ExecutionID != ev.ExecutionID || checkpoint.AttemptID != ev.AttemptID || checkpoint.Source != ev.Source || checkpoint.SourceSegment != ev.SourceSegment) {
			return nil, errors.New("conversation event and source checkpoint identities differ")
		}
		key := ev.SourceKey()
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
	if err := storageFailure(tx.Commit(), storageports.StorageCommitFailed); err != nil {
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
	if next == nil {
		next = []byte{}
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

func (s *SQLiteStore) ConversationCheckpoint(ctx context.Context, project, execution, attempt, source, segment string) (ConversationCheckpoint, error) {
	var checkpoint ConversationCheckpoint
	var updated string
	err := s.db.QueryRowContext(ctx, `SELECT project_identity,execution_id,attempt_id,source,source_segment,source_identity,generation,cursor,local_seq,updated_at FROM conversation_source_cursors WHERE project_identity=? AND execution_id=? AND attempt_id=? AND source=? AND source_segment=?`, project, execution, attempt, source, segment).Scan(&checkpoint.ProjectID, &checkpoint.ExecutionID, &checkpoint.AttemptID, &checkpoint.Source, &checkpoint.SourceSegment, &checkpoint.SourceIdentity, &checkpoint.Generation, &checkpoint.Cursor, &checkpoint.LocalSeq, &updated)
	if err != nil {
		return ConversationCheckpoint{}, persistenceError(err)
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

func (s *SQLiteStore) ConversationMapperSnapshot(ctx context.Context, project, execution, attempt, source, segment string) (contracts.ConversationMapperSnapshot, error) {
	var snapshot contracts.ConversationMapperSnapshot
	var payload []byte
	err := s.db.QueryRowContext(ctx, `SELECT payload FROM conversation_events WHERE project_identity=? AND execution_id=? AND attempt_id=? AND source=? AND source_segment=? AND event_type=? ORDER BY event_id DESC LIMIT 1`, project, execution, attempt, source, segment, ConversationSession).Scan(&payload)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return snapshot, err
	}
	if err == nil {
		var event ConversationEvent
		if err = json.Unmarshal(payload, &event); err != nil {
			return snapshot, err
		}
		if err = event.Validate(); err != nil {
			return snapshot, err
		}
		snapshot.Session = &event
	}
	payload = nil
	err = s.db.QueryRowContext(ctx, `SELECT payload FROM conversation_events WHERE project_identity=? AND execution_id=? AND attempt_id=? AND source=? AND source_segment=? AND event_type=? ORDER BY event_id DESC LIMIT 1`, project, execution, attempt, source, segment, ConversationTurnStarted).Scan(&payload)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return snapshot, err
	}
	if err == nil {
		var event ConversationEvent
		if err = json.Unmarshal(payload, &event); err != nil {
			return snapshot, err
		}
		if err = event.Validate(); err != nil {
			return snapshot, err
		}

		var count int
		err = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM conversation_events WHERE project_identity=? AND execution_id=? AND attempt_id=? AND source=? AND source_segment=? AND turn_id=? AND event_type=? AND phase=?`, project, execution, attempt, source, segment, event.TurnID, ConversationMessageComplete, MessagePhaseFinalAnswer).Scan(&count)
		if err != nil {
			return snapshot, err
		}
		snapshot.Turn = &event
		snapshot.FinalSeen = count > 0
	}
	return snapshot, nil
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
		case executionbackend.CapabilityVerified, executionbackend.CapabilityUnsupported, executionbackend.CapabilityNotTested:
		default:
			return fmt.Errorf("invalid conversation capability status %q", capability.Status)
		}
		if capability.Status == executionbackend.CapabilityVerified && capability.Evidence == "" {
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

func (s *SQLiteStore) ValidateConversationCheckpointFormat(ctx context.Context, project string, sources []string) error {
	if len(sources) == 0 {
		return nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(sources)), ",")
	args := make([]any, 0, len(sources)+1)
	args = append(args, project)
	for _, source := range sources {
		if source == "" {
			return errors.New("conversation checkpoint source is empty")
		}
		args = append(args, source)
	}
	query := `SELECT execution_id,attempt_id,source_segment,source_identity,cursor,local_seq FROM conversation_source_cursors WHERE project_identity=? AND source IN (` + placeholders + `)`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cp ConversationCheckpoint
		if err := rows.Scan(&cp.ExecutionID, &cp.AttemptID, &cp.SourceSegment, &cp.SourceIdentity, &cp.Cursor, &cp.LocalSeq); err != nil {
			return err
		}
		if _, err := contracts.DecodeConversationJournalPosition(cp); err != nil {
			return fmt.Errorf("unsupported conversation checkpoint %s/%s: %w; preserve state and inspect external execution before recovery", cp.ExecutionID, cp.AttemptID, err)
		}
	}
	return rows.Err()
}
