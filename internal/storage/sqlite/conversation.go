package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	api "github.com/erranliu/SteerLine/internal/product/api"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

// A committed canonical source hides fallback projections only within the same
// project/execution/attempt. Explicit source queries retain the raw observations.
func conversationSourcePredicate(filter api.ConversationFilter, args []any) (string, []any) {
	if filter.Source != "" {
		return ` AND source=?`, append(args, filter.Source)
	}
	return ` AND (event_type IN ('gap','input.wait') OR COALESCE(json_extract(payload,'$.source_role'),'')<>'fallback' OR NOT EXISTS (SELECT 1 FROM conversation_events canonical WHERE canonical.project_identity=conversation_events.project_identity AND canonical.execution_id=conversation_events.execution_id AND canonical.attempt_id=conversation_events.attempt_id AND canonical.event_type='session' AND json_extract(canonical.payload,'$.source_role')='canonical'))`, args
}

// NextConversationEvent returns stored bytes; decoding and page budgets belong to query.
func (s Reader) NextConversationEvent(ctx context.Context, project string, filter api.ConversationFilter, after int64) (*storageports.ConversationEventRecord, error) {
	statement := `SELECT event_id,payload_bytes,payload FROM conversation_events WHERE project_identity=? AND event_id>?`
	args := []any{project, after}
	sourcePredicate, args := conversationSourcePredicate(filter, args)
	statement += sourcePredicate
	if filter.WorkflowID != "" {
		statement += ` AND workflow_run_id=?`
		args = append(args, filter.WorkflowID)
	}
	if filter.ExecutionID != "" {
		statement += ` AND execution_id=?`
		args = append(args, filter.ExecutionID)
	}
	if filter.AttemptID != "" {
		statement += ` AND attempt_id=?`
		args = append(args, filter.AttemptID)
	}
	if filter.SessionID != "" {
		statement += ` AND session_id=?`
		args = append(args, filter.SessionID)
	}
	if filter.TurnID != "" {
		statement += ` AND turn_id=?`
		args = append(args, filter.TurnID)
	}
	statement += ` ORDER BY event_id LIMIT 1`
	var record storageports.ConversationEventRecord
	err := s.db.QueryRowContext(ctx, statement, args...).Scan(&record.EventID, &record.PayloadBytes, &record.Payload)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &record, nil
}

func (s Reader) ConversationBody(ctx context.Context, project string, ref api.ConversationBodyRef, limit int) (storageports.ConversationBodyRecord, error) {
	var record storageports.ConversationBodyRecord
	err := s.db.QueryRowContext(ctx, `SELECT substr(body,1,?),byte_count,sha256,completeness FROM conversation_bodies WHERE project_identity=? AND execution_id=? AND attempt_id=? AND item_key=? AND revision=?`, limit, project, ref.ExecutionID, ref.AttemptID, ref.ItemKey, ref.Revision).Scan(&record.Body, &record.TotalBytes, &record.SHA256, &record.Completeness)
	return record, err
}

func (s Reader) ConversationRaw(ctx context.Context, project string, ref api.ConversationRawRef, limit int) (storageports.ConversationRawRecord, error) {
	var record storageports.ConversationRawRecord
	err := s.db.QueryRowContext(ctx, `SELECT substr(payload,1,?),byte_count,sha256 FROM conversation_raw WHERE project_identity=? AND execution_id=? AND attempt_id=? AND raw_id=?`, limit, project, ref.ExecutionID, ref.AttemptID, ref.ID).Scan(&record.Data, &record.TotalBytes, &record.SHA256)
	return record, err
}

func (s Reader) ConversationCapabilityReports(ctx context.Context, backend executionbackend.BackendKind, protocol executionbackend.ExecutionProtocol, targetID string, limit int) ([]api.ConversationCapabilityReport, error) {
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
	reports := []api.ConversationCapabilityReport{}
	for rows.Next() {
		var payload []byte
		if err = rows.Scan(&payload); err != nil {
			return nil, err
		}
		var report struct {
			Version         int                                `json:"version"`
			Backend         executionbackend.BackendKind       `json:"backend"`
			Protocol        executionbackend.ExecutionProtocol `json:"protocol"`
			Agent           string                             `json:"agent"`
			AgentVersion    string                             `json:"agent_version,omitempty"`
			PlatformVersion string                             `json:"platform_version,omitempty"`
			Target          executionbackend.ExecutionTarget   `json:"target"`
			ExecutionID     string                             `json:"execution_id,omitempty"`
			AttemptID       string                             `json:"attempt_id,omitempty"`
			Source          string                             `json:"source,omitempty"`
			SourceIdentity  string                             `json:"source_identity,omitempty"`
			CheckedAt       time.Time                          `json:"checked_at"`
			Capabilities    []struct {
				Name     string                            `json:"name"`
				Status   executionbackend.CapabilityStatus `json:"status"`
				Evidence string                            `json:"evidence,omitempty"`
			} `json:"capabilities"`
		}
		if err = json.Unmarshal(payload, &report); err != nil {
			return nil, err
		}
		projected := api.ConversationCapabilityReport{Version: report.Version, Backend: string(report.Backend), Protocol: string(report.Protocol), Agent: report.Agent, AgentVersion: report.AgentVersion, PlatformVersion: report.PlatformVersion, Target: api.TargetSummary{ID: report.Target.ID}, ExecutionID: report.ExecutionID, AttemptID: report.AttemptID, Source: report.Source, SourceIdentity: report.SourceIdentity, CheckedAt: report.CheckedAt}
		for _, capability := range report.Capabilities {
			projected.Capabilities = append(projected.Capabilities, api.ConversationCapability{Name: capability.Name, Status: string(capability.Status), Evidence: capability.Evidence})
		}
		reports = append(reports, projected)
	}
	return reports, rows.Err()
}
