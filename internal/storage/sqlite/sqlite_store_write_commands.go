package sqlite

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

func (t *sqliteTx) PutDecision(v executionmodel.DecisionRecord) error {
	if v.ProjectIdentity == "" || v.DecisionID == "" || v.QuestionID == "" || v.Status == "" || v.Scope == "" {
		return errors.New("decision identity, scope, question and status are required")
	}
	if !validDecisionStatus(v.Status) {
		return fmt.Errorf("invalid decision status %q", v.Status)
	}
	if err := t.advanceControlRevision("decisions", "decision_id", v.ProjectIdentity, v.DecisionID, v.WorkflowRunID, v); err != nil {
		return err
	}
	b, err := encodePayload(v)
	if err != nil {
		return err
	}
	_, err = t.tx.ExecContext(t.ctx, `INSERT INTO decisions(project_identity,decision_id,scope,workflow_run_id,execution_id,attempt_id,question_id,status,version,payload) VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT(project_identity,decision_id) DO UPDATE SET scope=excluded.scope,workflow_run_id=excluded.workflow_run_id,execution_id=excluded.execution_id,attempt_id=excluded.attempt_id,question_id=excluded.question_id,status=excluded.status,version=excluded.version,payload=excluded.payload`, v.ProjectIdentity, v.DecisionID, v.Scope, nullIfEmpty(v.WorkflowRunID), nullIfEmpty(v.ExecutionID), nullIfEmpty(v.AttemptID), v.QuestionID, v.Status, v.Version, b)
	return err
}

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

func (t *sqliteTx) PutOperation(v executionmodel.OperationRecord) (executionmodel.OperationRecord, bool, error) {
	if v.ProjectIdentity == "" || v.OperationID == "" || v.IdempotencyKey == "" || v.Kind == "" || v.PayloadDigest == "" {
		return v, false, errors.New("operation identity, idempotency key, kind and digest are required")
	}
	if !validOperationStatus(v.Status) {
		return v, false, fmt.Errorf("invalid operation status %q", v.Status)
	}
	var oldID, oldDigest, payload string
	err := t.tx.QueryRowContext(t.ctx, `SELECT operation_id,payload_digest,payload FROM operations WHERE project_identity=? AND idempotency_key=?`, v.ProjectIdentity, v.IdempotencyKey).Scan(&oldID, &oldDigest, &payload)
	if err == nil {
		if oldDigest != v.PayloadDigest {
			return v, false, errors.New("idempotency key reused with different operation content")
		}
		var existing executionmodel.OperationRecord
		if err = json.Unmarshal([]byte(payload), &existing); err != nil {
			return v, false, err
		}
		if existing.Kind != v.Kind || !sameOperationRequest(existing.Request, v.Request) || recordFingerprint(existing.LifecycleIdentity) != recordFingerprint(v.LifecycleIdentity) {
			return v, false, errors.New("operation request content cannot change")
		}
		if oldID == v.OperationID {
			b, marshalErr := encodePayload(v)
			if marshalErr != nil {
				return v, false, marshalErr
			}
			if _, updateErr := t.tx.ExecContext(t.ctx, `UPDATE operations SET status=?,platform_request_id=?,payload=? WHERE project_identity=? AND operation_id=? AND payload_digest=?`, v.Status, v.PlatformRequestID, b, v.ProjectIdentity, v.OperationID, v.PayloadDigest); updateErr != nil {
				return v, false, updateErr
			}
			return v, false, nil
		}
		return existing, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return v, false, err
	}
	b, err := encodePayload(v)
	if err != nil {
		return v, false, err
	}
	_, err = t.tx.ExecContext(t.ctx, `INSERT INTO operations(project_identity,operation_id,idempotency_key,kind,status,payload_digest,platform_request_id,payload) VALUES(?,?,?,?,?,?,?,?)`, v.ProjectIdentity, v.OperationID, v.IdempotencyKey, v.Kind, v.Status, v.PayloadDigest, v.PlatformRequestID, b)
	return v, err == nil, err
}

func (t *sqliteTx) UpdateOperation(v executionmodel.OperationRecord) error {
	if v.ProjectIdentity == "" || v.OperationID == "" || v.IdempotencyKey == "" || v.PayloadDigest == "" || v.Status == "" {
		return errors.New("operation identity, digest and status are required")
	}
	var digest, originalPayload string
	if err := t.tx.QueryRowContext(t.ctx, `SELECT payload_digest,payload FROM operations WHERE project_identity=? AND operation_id=?`, v.ProjectIdentity, v.OperationID).Scan(&digest, &originalPayload); err != nil {
		return err
	}
	if digest != v.PayloadDigest {
		return errors.New("operation payload digest cannot change")
	}
	var original executionmodel.OperationRecord
	if err := json.Unmarshal([]byte(originalPayload), &original); err != nil {
		return err
	}
	if recordFingerprint(original.LifecycleIdentity) != recordFingerprint(v.LifecycleIdentity) {
		return errors.New("operation cannot replace its original lifecycle binding identity")
	}
	b, err := encodePayload(v)
	if err != nil {
		return err
	}
	res, err := t.tx.ExecContext(t.ctx, `UPDATE operations SET status=?,platform_request_id=?,payload=? WHERE project_identity=? AND operation_id=? AND payload_digest=?`, v.Status, v.PlatformRequestID, b, v.ProjectIdentity, v.OperationID, v.PayloadDigest)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err == nil && n != 1 {
		return errors.New("operation not found")
	}
	return err
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
		if old.Kind == "answer_decision" {
			var oldAnswer, newAnswer executionmodel.DecisionAnswer
			if decode(payload, &oldAnswer) != nil || decode(v.Payload, &newAnswer) != nil || !executionmodel.SameDecisionAnswer(oldAnswer, newAnswer) {
				return v, false, errors.New("idempotency key reused with a different decision answer")
			}
		} else if old.ExpectedVersion != v.ExpectedVersion || old.RequestedBy != v.RequestedBy || string(payload) != string(v.Payload) {
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
		var version uint64
		var status string
		var recordBytes []byte
		if err := t.tx.QueryRowContext(t.ctx, `SELECT version,status,payload FROM decisions WHERE project_identity=? AND decision_id=?`, v.ProjectIdentity, v.TargetID).Scan(&version, &status, &recordBytes); err != nil {
			return v, false, err
		}
		var record executionmodel.DecisionRecord
		var answer executionmodel.DecisionAnswer
		var request executionmodel.DecisionRequest
		if err := decode(recordBytes, &record); err != nil {
			return v, false, err
		}
		if err := decode(v.Payload, &answer); err != nil {
			return v, false, err
		}
		if err := decode(record.Request, &request); err != nil {
			return v, false, err
		}
		if answer.DecisionID != v.TargetID || answer.RunID != record.WorkflowRunID || answer.AnsweredBy != v.RequestedBy {
			return v, false, errors.New("answer target or requester mismatch")
		}
		if err := executionmodel.ValidateDecisionAnswer(&request, &answer); err != nil {
			return v, false, err
		}
		var acceptedCommandID string
		acceptedErr := t.tx.QueryRowContext(t.ctx, `SELECT command_id FROM decision_answers WHERE project_identity=? AND decision_id=? ORDER BY decision_version LIMIT 1`, v.ProjectIdentity, v.TargetID).Scan(&acceptedCommandID)
		if acceptedErr == nil {
			var first storageports.CommandRecord
			var firstPayload, firstResult []byte
			var firstCreated, firstUpdated string
			if err := t.tx.QueryRowContext(t.ctx, `SELECT command_id,kind,target_id,expected_version,idempotency_key,requested_by,payload,status,result,created_at,updated_at FROM commands WHERE project_identity=? AND command_id=?`, v.ProjectIdentity, acceptedCommandID).Scan(&first.CommandID, &first.Kind, &first.TargetID, &first.ExpectedVersion, &first.IdempotencyKey, &first.RequestedBy, &firstPayload, &first.Status, &firstResult, &firstCreated, &firstUpdated); err != nil {
				return v, false, err
			}
			var firstAnswer executionmodel.DecisionAnswer
			if err := decode(firstPayload, &firstAnswer); err != nil {
				return v, false, err
			}
			if !executionmodel.SameDecisionAnswer(firstAnswer, answer) {
				return v, false, storageports.ErrAnswerConflict
			}
			first.ProjectIdentity, first.Payload, first.Result = v.ProjectIdentity, firstPayload, firstResult
			first.CreatedAt, _ = time.Parse(time.RFC3339Nano, firstCreated)
			first.UpdatedAt, _ = time.Parse(time.RFC3339Nano, firstUpdated)
			return first, false, nil
		}
		if !errors.Is(acceptedErr, sql.ErrNoRows) {
			return v, false, acceptedErr
		}
		if version != v.ExpectedVersion && !(status == "answer_saved" && version == v.ExpectedVersion+1) {
			return v, false, fmt.Errorf("decision version conflict: expected %d, current %d, status %s", v.ExpectedVersion, version, status)
		}
	}
	if v.Status == "" {
		v.Status = "pending"
	}
	if v.CreatedAt.IsZero() {
		v.CreatedAt = now()
	}
	v.UpdatedAt = v.CreatedAt
	_, err = t.tx.ExecContext(t.ctx, `INSERT INTO commands(project_identity,command_id,kind,target_id,expected_version,idempotency_key,requested_by,payload,status,result,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, v.ProjectIdentity, v.CommandID, v.Kind, v.TargetID, v.ExpectedVersion, v.IdempotencyKey, v.RequestedBy, v.Payload, v.Status, v.Result, v.CreatedAt.UTC().Format(time.RFC3339Nano), v.UpdatedAt.UTC().Format(time.RFC3339Nano))
	return v, err == nil, err
}

func (t *sqliteTx) AcceptDecisionAnswer(command storageports.CommandRecord, answer executionmodel.DecisionAnswer) (executionmodel.DecisionRecord, bool, error) {
	var record executionmodel.DecisionRecord
	var version uint64
	var status string
	var recordBytes []byte
	if err := t.tx.QueryRowContext(t.ctx, `SELECT version,status,payload FROM decisions WHERE project_identity=? AND decision_id=?`, command.ProjectIdentity, command.TargetID).Scan(&version, &status, &recordBytes); err != nil {
		return record, false, err
	}
	if err := decode(recordBytes, &record); err != nil {
		return record, false, err
	}
	if answer.DecisionID != command.TargetID || answer.RunID == "" || answer.AnsweredBy != command.RequestedBy || answer.RunID != record.WorkflowRunID || answer.ExpectedDecisionVersion != 0 && answer.ExpectedDecisionVersion != command.ExpectedVersion {
		return record, false, errors.New("answer target or requester mismatch")
	}
	var previousCommandID string
	var previousPayload []byte
	err := t.tx.QueryRowContext(t.ctx, `SELECT da.command_id,c.payload FROM decision_answers da JOIN commands c ON c.project_identity=da.project_identity AND c.command_id=da.command_id WHERE da.project_identity=? AND da.decision_id=? ORDER BY da.decision_version LIMIT 1`, command.ProjectIdentity, command.TargetID).Scan(&previousCommandID, &previousPayload)
	if err == nil {
		if status == "pending" {
			return record, false, errors.New("unsupported prior answer reservation exists while this decision remains pending; preserve the state without importing it")
		}
		var previous executionmodel.DecisionAnswer
		if err = decode(previousPayload, &previous); err != nil {
			return record, false, err
		}
		if !executionmodel.SameDecisionAnswer(previous, answer) {
			return record, false, storageports.ErrAnswerConflict
		}
		result := mustJSON(map[string]any{"decision_id": command.TargetID, "status": record.Status, "version": record.Version, "original_command_id": previousCommandID, "idempotent": true})
		command.Status, command.Result = "accepted", result
		if err = t.UpdateCommand(command); err != nil {
			return record, false, err
		}
		return record, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return record, false, err
	}
	if version != command.ExpectedVersion || status != "pending" {
		return record, false, fmt.Errorf("decision version conflict: expected %d, current %d, status %s", command.ExpectedVersion, version, status)
	}
	if answer.ExpectedRunRevision == 0 || answer.ServiceInstanceID == "" {
		return record, false, errors.New("decision answer lacks its displayed service instance and run revision")
	}
	var projectionBytes []byte
	if err = t.tx.QueryRowContext(t.ctx, `SELECT payload FROM projection WHERE project_identity=?`, command.ProjectIdentity).Scan(&projectionBytes); err != nil {
		return record, false, fmt.Errorf("load current service projection: %w", err)
	}
	var projection storageports.ServiceCheckpoint
	if err = decode(projectionBytes, &projection); err != nil {
		return record, false, fmt.Errorf("decode current service projection: %w", err)
	}
	if projection.Service.InstanceID != answer.ServiceInstanceID || projection.Service.Status != "running" || !projection.Service.Ready {
		return record, false, errors.New("stale_instance: answer targets a service instance that is no longer ready")
	}
	var revision uint64
	var phase string
	var workflowBytes []byte
	var executionBytes []byte
	err = t.tx.QueryRowContext(t.ctx, `SELECT w.control_revision,w.phase,w.payload,e.payload FROM workflow_runs w JOIN executions e ON e.project_identity=w.project_identity AND e.workflow_run_id=w.workflow_run_id WHERE w.project_identity=? AND w.workflow_run_id=? AND e.execution_id=?`, command.ProjectIdentity, answer.RunID, record.ExecutionID).Scan(&revision, &phase, &workflowBytes, &executionBytes)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return record, false, persistenceError(err)
		}
		return record, false, errors.New("stale_revision: decision workflow or execution no longer exists")
	}
	var execution executionmodel.ExecutionRecord
	var workflow executionmodel.WorkflowRunRecord
	if err := decode(workflowBytes, &workflow); err != nil {
		return record, false, err
	}
	if workflow.Abandonment != nil {
		return record, false, nativeErr("candidate_inactive", "abandoned candidate cannot accept feedback")
	}
	if err = decode(executionBytes, &execution); err != nil {
		return record, false, err
	}
	if revision != answer.ExpectedRunRevision || phase != "waiting_input" && phase != "waiting_external" && phase != "blocked" || execution.DecisionID != record.DecisionID || record.WorkflowRunID != answer.RunID || execution.CurrentStep != "worker" {
		return record, false, errors.New("stale_revision: decision or run identity changed; refresh before answering")
	}
	var currentAttempt string
	if err = t.tx.QueryRowContext(t.ctx, `SELECT attempt_id FROM step_attempts WHERE project_identity=? AND execution_id=? AND step_id=? ORDER BY attempt_number DESC LIMIT 1`, command.ProjectIdentity, record.ExecutionID, execution.CurrentStep).Scan(&currentAttempt); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return record, false, persistenceError(err)
	}
	if err != nil || currentAttempt != record.AttemptID {
		return record, false, errors.New("stale_revision: decision attempt changed")
	}
	var request executionmodel.DecisionRequest
	if err = decode(record.Request, &request); err != nil {
		return record, false, err
	}
	if err = executionmodel.ValidateDecisionAnswer(&request, &answer); err != nil {
		return record, false, err
	}
	answer.AnsweredAt = now()
	record.Answer, err = json.Marshal(answer)
	if err != nil {
		return record, false, err
	}
	record.AnsweredBy = answer.AnsweredBy
	record.Status = "answer_saved"
	record.Version++
	record.UpdatedAt = now()
	if err = t.PutDecision(record); err != nil {
		return record, false, err
	}
	if _, err = t.tx.ExecContext(t.ctx, `INSERT INTO decision_answers(project_identity,decision_id,decision_version,command_id,payload_digest,created_at) VALUES(?,?,?,?,?,?)`, command.ProjectIdentity, command.TargetID, command.ExpectedVersion, command.CommandID, operationDigest(mustJSON(struct{ OptionID, Note string }{answer.OptionID, answer.Note})), now().Format(time.RFC3339Nano)); err != nil {
		return record, false, err
	}
	command.Status = "accepted"
	command.Result = mustJSON(map[string]any{"decision_id": command.TargetID, "status": record.Status, "version": record.Version, "idempotent": false})
	if err = t.UpdateCommand(command); err != nil {
		return record, false, err
	}
	return record, false, nil
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
	var checkpoint storageports.ServiceCheckpoint
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
