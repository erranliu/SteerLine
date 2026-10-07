package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	executionvalues "github.com/erranliu/SteerLine/internal/execution/model"
	"time"
	"unicode/utf8"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

const maxConversationBodyBytes = executionvalues.MaxConversationBodyBytes

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
			if write.BodyMode != "" {
				sum := sha256.Sum256(write.Body)
				if existing.Body == nil || existing.Body.SHA256 != hex.EncodeToString(sum[:]) || existing.Body.Bytes != int64(len(write.Body)) {
					return nil, errors.New("final event identity was reused with different content")
				}
			}
			existing.EventID = existingID
			out = append(out, existing)
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		if write.BodyMode != "" {
			if len(write.Body) > maxConversationBodyBytes || !utf8.Valid(write.Body) {
				return nil, errors.New("conversation message body is invalid UTF-8 or exceeds 16 MiB")
			}
			if ev.ItemKey == "" {
				return nil, errors.New("conversation body requires a scoped item key")
			}
			bodyRef, bodyErr := commitConversationBody(ctx, tx, write)
			if bodyErr != nil {
				return nil, bodyErr
			}
			ev.Body = bodyRef
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

func commitConversationBody(ctx context.Context, tx *sql.Tx, write ConversationWrite) (*ConversationBodyRef, error) {
	ev := write.Event
	if write.BodyMode != "complete" || ev.Type != ConversationMessageComplete || ev.Phase != MessagePhaseFinalAnswer || ev.FinalAvailability != executionvalues.FinalAvailable {
		return nil, errors.New("only a verified complete final reply can have a stored body")
	}
	sum := sha256.Sum256(write.Body)
	digest := hex.EncodeToString(sum[:])
	ref, err := conversationBodyRef(ctx, tx, ev.ProjectID, ev.ExecutionID, ev.AttemptID, ev.ItemKey, 1)
	if err == nil {
		if ref.SHA256 != digest || ref.Bytes != int64(len(write.Body)) {
			return nil, errors.New("native final item changed after completion; original final retained")
		}
		return ref, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	body := write.Body
	if body == nil {
		body = []byte{}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO conversation_bodies(project_identity,execution_id,attempt_id,item_key,revision,body,sha256,byte_count,source,completeness,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, ev.ProjectID, ev.ExecutionID, ev.AttemptID, ev.ItemKey, 1, body, digest, len(body), nonEmpty(write.BodySource, ev.Source), string(BodyComplete), ev.ReceivedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	return &ConversationBodyRef{ExecutionID: ev.ExecutionID, AttemptID: ev.AttemptID, ItemKey: ev.ItemKey, Revision: 1, SHA256: digest, Bytes: int64(len(body)), Source: nonEmpty(write.BodySource, ev.Source), Completeness: BodyComplete}, nil
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
