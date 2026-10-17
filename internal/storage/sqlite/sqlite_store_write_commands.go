package sqlite

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/erranliu/SteerLine/internal/hostcontract"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

func (t *sqliteTx) AppendEvent(v storageports.DomainEvent) (int64, error) {
	if v.ProjectIdentity == "" || v.Type == "" {
		return 0, errors.New("event project and type are required")
	}
	if v.ReceivedAt.IsZero() {
		v.ReceivedAt = now()
	}
	if v.PayloadBytes == 0 && len(v.Payload) > 0 {
		v.PayloadBytes = len(v.Payload)
	}
	if v.PayloadDigest == "" && len(v.Payload) > 0 {
		v.PayloadDigest = operationDigest(v.Payload)
	}
	res, err := t.tx.ExecContext(t.ctx, `INSERT INTO events(project_identity,task_id,workflow_run_id,execution_id,attempt_id,external_source,external_id,type,received_at,payload_bytes,payload_digest,payload) VALUES(?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT DO NOTHING`, v.ProjectIdentity, v.TaskID, v.WorkflowRunID, v.ExecutionID, v.AttemptID, v.ExternalSource, v.ExternalID, v.Type, v.ReceivedAt.UTC().Format(time.RFC3339Nano), v.PayloadBytes, v.PayloadDigest, v.Payload)
	if err != nil {
		return 0, err
	}
	if n, _ := res.RowsAffected(); n == 0 && v.ExternalSource != "" && v.ExternalID != "" {
		err = t.tx.QueryRowContext(t.ctx, `SELECT event_id FROM events WHERE project_identity=? AND external_source=? AND external_id=?`, v.ProjectIdentity, v.ExternalSource, v.ExternalID).Scan(&v.EventID)
		return v.EventID, err
	}
	id, err := res.LastInsertId()
	return id, err
}

func (t *sqliteTx) EnqueueCommand(v storageports.CommandRecord) (storageports.CommandRecord, bool, error) {
	if v.ProjectIdentity == "" || v.CommandID == "" || v.Kind == "" || v.TargetID == "" || v.IdempotencyKey == "" || v.RequestedBy == "" {
		return v, false, errors.New("command identity, kind, target, idempotency key and requester are required")
	}
	var old storageports.CommandRecord
	var payload []byte
	var created, updated string
	err := t.tx.QueryRowContext(t.ctx, `SELECT command_id,kind,target_id,expected_version,idempotency_key,requested_by,payload,status,result,created_at,updated_at FROM commands WHERE project_identity=? AND idempotency_key=?`, v.ProjectIdentity, v.IdempotencyKey).Scan(&old.CommandID, &old.Kind, &old.TargetID, &old.ExpectedVersion, &old.IdempotencyKey, &old.RequestedBy, &payload, &old.Status, &old.Result, &created, &updated)
	if err == nil {
		if old.Kind != v.Kind || old.TargetID != v.TargetID {
			return v, false, errors.New("idempotency key reused with different command content")
		}
		if old.ExpectedVersion != v.ExpectedVersion || old.RequestedBy != v.RequestedBy || string(payload) != string(v.Payload) {
			return v, false, errors.New("idempotency key reused with different command content")
		}
		old.ProjectIdentity = v.ProjectIdentity
		old.Payload = payload
		old.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		old.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		return old, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return v, false, err
	}
	if v.Kind == "answer_decision" {
		return v, false, errors.New("native answers must use the Runner control outbox")
	}
	if v.CreatedAt.IsZero() {
		v.CreatedAt = now()
	}
	v.UpdatedAt = v.CreatedAt
	_, err = t.tx.ExecContext(t.ctx, `INSERT INTO commands(project_identity,command_id,kind,target_id,expected_version,idempotency_key,requested_by,payload,status,result,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, v.ProjectIdentity, v.CommandID, v.Kind, v.TargetID, v.ExpectedVersion, v.IdempotencyKey, v.RequestedBy, v.Payload, v.Status, v.Result, v.CreatedAt.UTC().Format(time.RFC3339Nano), v.UpdatedAt.UTC().Format(time.RFC3339Nano))
	return v, err == nil, err
}

func (t *sqliteTx) UpdateCommand(v storageports.CommandRecord) error {
	if v.Status == "" {
		return errors.New("command status is required")
	}
	v.UpdatedAt = now()
	res, err := t.tx.ExecContext(t.ctx, `UPDATE commands SET status=?,result=?,updated_at=? WHERE project_identity=? AND command_id=?`, v.Status, v.Result, v.UpdatedAt.Format(time.RFC3339Nano), v.ProjectIdentity, v.CommandID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err == nil && n != 1 {
		return errors.New("command not found")
	}
	return err
}

func (t *sqliteTx) PutProjection(project string, revision uint64, payload []byte) error {
	if project == "" || len(payload) == 0 {
		return errors.New("projection identity and payload are required")
	}
	_, err := t.tx.ExecContext(t.ctx, `INSERT INTO projection(project_identity,revision,payload,updated_at) VALUES(?,?,?,?) ON CONFLICT(project_identity) DO UPDATE SET revision=excluded.revision,payload=excluded.payload,updated_at=excluded.updated_at`, project, revision, payload, now().Format(time.RFC3339Nano))
	return err
}

func (t *sqliteTx) heartbeatProjection(project string) error {
	var payload []byte
	var revision uint64
	err := t.tx.QueryRowContext(t.ctx, `SELECT payload,revision FROM projection WHERE project_identity=?`, project).Scan(&payload, &revision)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var checkpoint hostcontract.ServiceCheckpoint
	if err = decode(payload, &checkpoint); err != nil {
		return err
	}
	checkpoint.UpdatedAt = now()
	checkpoint.HeartbeatAt = checkpoint.UpdatedAt
	payload, err = json.Marshal(checkpoint)
	if err != nil {
		return err
	}
	return t.PutProjection(project, revision, payload)
}
