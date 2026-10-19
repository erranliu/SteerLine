package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

func (s Reader) LoadProjection(ctx context.Context, project string) ([]byte, uint64, error) {
	var payload []byte
	var revision uint64
	err := s.db.QueryRowContext(ctx, `SELECT payload,revision FROM projection WHERE project_identity=?`, project).Scan(&payload, &revision)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, 0, nil
	}
	return payload, revision, err
}

// DatabaseVersion is a read-only cache invalidation hint. Callers must still
// use the bound native reader before trusting task data or issuing commands.
func (s Reader) DatabaseVersion(ctx context.Context) (string, uint64, error) {
	var id string
	var revision uint64
	err := s.db.QueryRowContext(ctx, `SELECT database_id,revision FROM database_state WHERE singleton=1`).Scan(&id, &revision)
	return id, revision, err
}

type QueryContext interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

type CommandRowQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func ReadCommandByKey(ctx context.Context, query CommandRowQuerier, project, key string) (storageports.CommandRecord, bool, error) {
	var command storageports.CommandRecord
	var created, updated string
	err := query.QueryRowContext(ctx, `SELECT project_identity,command_id,kind,target_id,expected_version,idempotency_key,requested_by,payload,status,result,created_at,updated_at FROM commands WHERE project_identity=? AND idempotency_key=?`, project, key).Scan(&command.ProjectIdentity, &command.CommandID, &command.Kind, &command.TargetID, &command.ExpectedVersion, &command.IdempotencyKey, &command.RequestedBy, &command.Payload, &command.Status, &command.Result, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return command, false, nil
	}
	if err != nil {
		return command, false, err
	}
	command.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	command.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return command, true, nil
}

// Events reads bounded metadata and materializes only the inline bodies that
// fit payloadBudget. A zero budget reads metadata only, including lookahead.
func (s Reader) Events(ctx context.Context, project, execution, attempt string, after int64, limit, payloadBudget int) ([]storageports.DomainEvent, error) {
	return readEvents(ctx, s.db, project, execution, attempt, after, limit, payloadBudget)
}

func readEvents(ctx context.Context, db QueryContext, project, execution, attempt string, after int64, limit, payloadBudget int) ([]storageports.DomainEvent, error) {
	return readBoundEvents(ctx, db, project, "", execution, attempt, after, limit, payloadBudget)
}

func readBoundEvents(ctx context.Context, db QueryContext, project, workflow, execution, attempt string, after int64, limit, payloadBudget int) ([]storageports.DomainEvent, error) {
	if limit < 1 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	if payloadBudget < 0 {
		return nil, errors.New("event payload budget cannot be negative")
	}
	payloadBudget = min(payloadBudget, storageports.MaxEventPageBytes)
	// The window sums metadata before looking up payloads, so a batch of large
	// events does not load bodies that the page will immediately discard.
	query := `WITH batch AS (SELECT event_id,payload_bytes FROM events WHERE project_identity=? AND event_id>?`
	args := []any{project, after}
	if workflow != "" {
		query += ` AND workflow_run_id=?`
		args = append(args, workflow)
	}
	if execution != "" {
		query += ` AND execution_id=?`
		args = append(args, execution)
	}
	if attempt != "" {
		query += ` AND attempt_id=?`
		args = append(args, attempt)
	}
	query += ` ORDER BY event_id LIMIT ?), bounded AS (
		SELECT event_id,SUM(CASE WHEN payload_bytes<=? THEN payload_bytes ELSE 0 END) OVER (ORDER BY event_id) AS inline_bytes FROM batch
	) SELECT e.event_id,e.project_identity,e.task_id,e.workflow_run_id,e.execution_id,e.attempt_id,e.external_source,e.external_id,e.type,e.received_at,e.payload_bytes,e.payload_digest,
		CASE WHEN e.payload_bytes<=? AND b.inline_bytes<=? THEN e.payload ELSE X'' END
		FROM bounded b JOIN events e ON e.event_id=b.event_id ORDER BY e.event_id`
	args = append(args, limit, storageports.MaxEventPageBytes, storageports.MaxEventPageBytes, payloadBudget)
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return scanEventRows(rows)
}

func scanEventRows(rows *sql.Rows) ([]storageports.DomainEvent, error) {
	defer rows.Close()
	out := []storageports.DomainEvent{}
	for rows.Next() {
		var v storageports.DomainEvent
		var received string
		if err := rows.Scan(&v.EventID, &v.ProjectIdentity, &v.TaskID, &v.WorkflowRunID, &v.ExecutionID, &v.AttemptID, &v.ExternalSource, &v.ExternalID, &v.Type, &received, &v.PayloadBytes, &v.PayloadDigest, &v.Payload); err != nil {
			return nil, err
		}
		v.ReceivedAt, _ = time.Parse(time.RFC3339Nano, received)
		v.PayloadOmitted = v.PayloadBytes > len(v.Payload)
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s Reader) EventPayload(ctx context.Context, project string, eventID int64, offset int64, limit int) ([]byte, int64, string, error) {
	if eventID <= 0 || offset < 0 {
		return nil, 0, "", errors.New("event body reference or offset is invalid")
	}
	if limit < 1 {
		limit = 256 << 10
	}
	if limit > 256<<10 {
		limit = 256 << 10
	}
	var total int64
	var digest string
	var body []byte
	err := s.db.QueryRowContext(ctx, `SELECT payload_bytes,payload_digest,substr(payload,?,?) FROM events WHERE project_identity=? AND event_id=?`, offset+1, limit, project, eventID).Scan(&total, &digest, &body)
	return body, total, digest, err
}

func (s Reader) Commands(ctx context.Context, project, status string, limit int) ([]storageports.CommandRecord, error) {
	return s.CommandsFiltered(ctx, project, status, limit, nil)
}

func (s Reader) CommandsFiltered(ctx context.Context, project, status string, limit int, kinds []string) ([]storageports.CommandRecord, error) {
	if limit < 1 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	q := `SELECT command_id,kind,target_id,expected_version,idempotency_key,requested_by,payload,status,result,created_at,updated_at FROM commands WHERE project_identity=?`
	args := []any{project}
	if status != "" {
		q += ` AND status=?`
		args = append(args, status)
	}
	if len(kinds) > 0 {
		q += ` AND kind IN (` + strings.TrimSuffix(strings.Repeat("?,", len(kinds)), ",") + `)`
		for _, kind := range kinds {
			args = append(args, kind)
		}
	}
	q += ` ORDER BY created_at LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []storageports.CommandRecord{}
	for rows.Next() {
		var v storageports.CommandRecord
		var created, updated string
		if err = rows.Scan(&v.CommandID, &v.Kind, &v.TargetID, &v.ExpectedVersion, &v.IdempotencyKey, &v.RequestedBy, &v.Payload, &v.Status, &v.Result, &created, &updated); err != nil {
			return nil, err
		}
		v.ProjectIdentity = project
		v.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		v.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s Reader) Command(ctx context.Context, project, commandID string) (storageports.CommandRecord, error) {
	var command storageports.CommandRecord
	var created, updated string
	err := s.db.QueryRowContext(ctx, `SELECT project_identity,command_id,kind,target_id,expected_version,idempotency_key,requested_by,payload,status,result,created_at,updated_at FROM commands WHERE project_identity=? AND command_id=?`, project, commandID).Scan(&command.ProjectIdentity, &command.CommandID, &command.Kind, &command.TargetID, &command.ExpectedVersion, &command.IdempotencyKey, &command.RequestedBy, &command.Payload, &command.Status, &command.Result, &created, &updated)
	if err != nil {
		return command, err
	}
	command.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return command, err
	}
	command.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated)
	return command, err
}

func (s Reader) CommandByIdempotency(ctx context.Context, project, idempotencyKey string) (storageports.CommandRecord, error) {
	var command storageports.CommandRecord
	var created, updated string
	err := s.db.QueryRowContext(ctx, `SELECT project_identity,command_id,kind,target_id,expected_version,idempotency_key,requested_by,payload,status,result,created_at,updated_at FROM commands WHERE project_identity=? AND idempotency_key=?`, project, idempotencyKey).Scan(&command.ProjectIdentity, &command.CommandID, &command.Kind, &command.TargetID, &command.ExpectedVersion, &command.IdempotencyKey, &command.RequestedBy, &command.Payload, &command.Status, &command.Result, &created, &updated)
	if err != nil {
		return command, err
	}
	command.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return command, err
	}
	command.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated)
	return command, err
}
