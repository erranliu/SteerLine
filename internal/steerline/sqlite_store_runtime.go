package steerline

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
)

func (s *SQLiteStore) SaveRuntime(ctx context.Context, runtime Runtime) (Runtime, error) {
	return s.saveRuntime(ctx, runtime, nil)
}

func (s *SQLiteStore) saveRuntime(ctx context.Context, runtime Runtime, baseline map[string]string) (Runtime, error) {
	project := runtime.Repository.Identity
	if project == "" {
		return runtime, errors.New("runtime repository identity is required for SQLite persistence")
	}
	pendingEvents := append([]DomainEvent(nil), runtime.PendingEvents...)
	err := s.transactSQLite(ctx, func(tx *sqliteTx) error {
		var revision uint64
		if err := tx.tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(revision),0) FROM projection WHERE project_identity=?`, project).Scan(&revision); err != nil {
			return err
		}
		var authority string
		authorityErr := tx.tx.QueryRowContext(ctx, `SELECT authority FROM project_task_authorities WHERE project_identity=?`, project).Scan(&authority)
		if authorityErr != nil && !errors.Is(authorityErr, sql.ErrNoRows) {
			return authorityErr
		}
		if authority != "native" {
			return nativeErr("authority_mismatch", "runtime publication requires the native task authority")
		}
		runtime.Revision = revision + 1
		if runtime.SchemaVersion != WorkerFlowRuntimeSchema {
			return fmt.Errorf("unsupported runtime schema %d", runtime.SchemaVersion)
		}
		runtime.Operations = runtimeOperations(runtime)
		changed := func(key string, value any) bool { return baseline == nil || baseline[key] != recordFingerprint(value) }
		for _, record := range runtime.WorkflowRuns {
			if changed("workflow:"+record.WorkflowRunID, record) {
				if err := tx.checkPublicationBaseline("workflow_runs", "workflow_run_id", project, record.WorkflowRunID, "workflow:"+record.WorkflowRunID, baseline); err != nil {
					return err
				}
				if err := tx.PutWorkflowRun(record); err != nil {
					return err
				}
			}
		}
		for _, record := range runtime.Executions {
			if changed("execution:"+record.ExecutionID, record) {
				if err := tx.checkPublicationBaseline("executions", "execution_id", project, record.ExecutionID, "execution:"+record.ExecutionID, baseline); err != nil {
					return err
				}
				if err := tx.PutExecution(record); err != nil {
					return err
				}
			}
		}
		for _, record := range runtime.Attempts {
			if changed("attempt:"+record.AttemptID, record) {
				if err := tx.checkPublicationBaseline("step_attempts", "attempt_id", project, record.AttemptID, "attempt:"+record.AttemptID, baseline); err != nil {
					return err
				}
				if err := tx.PutStepAttempt(record); err != nil {
					return err
				}
			}
		}
		for _, record := range runtime.Decisions {
			if changed("decision:"+record.DecisionID, record) {
				if err := tx.checkPublicationBaseline("decisions", "decision_id", project, record.DecisionID, "decision:"+record.DecisionID, baseline); err != nil {
					return err
				}
				if err := tx.PutDecision(record); err != nil {
					return err
				}
			}
		}
		for _, operation := range runtime.Operations {
			if !changed("operation:"+operation.OperationID, operation) {
				continue
			}
			persisted, _, err := tx.PutOperation(operation)
			if err != nil {
				return err
			}
			if persisted.OperationID != operation.OperationID {
				return errors.New("runtime contains a second identity for an existing operation intent")
			}
		}
		for _, event := range pendingEvents {
			if event.ProjectIdentity == "" {
				event.ProjectIdentity = project
			}
			if _, err := tx.AppendEvent(event); err != nil {
				return err
			}
		}
		for i := range runtime.WorkflowRuns {
			record := &runtime.WorkflowRuns[i]
			if err := tx.tx.QueryRowContext(ctx, `SELECT control_revision FROM workflow_runs WHERE project_identity=? AND workflow_run_id=?`, project, record.WorkflowRunID).Scan(&record.ControlRevision); err != nil {
				return err
			}
		}
		payload, err := json.Marshal(runtimeCheckpoint(runtime))
		if err != nil {
			return err
		}
		return tx.PutProjection(project, runtime.Revision, payload)
	})
	if err == nil {
		runtime.PendingEvents = nil
	}
	return runtime, err
}

func runtimeOperations(runtime Runtime) []OperationRecord {
	project := runtime.Repository.Identity
	out := append([]OperationRecord(nil), runtime.Operations...)
	indices := map[string]int{}
	for i, op := range out {
		indices[op.OperationID] = i
	}
	if op := runtime.Control.Operation; op != nil && !centerOwnedNativeControlAction(op.Action) {
		status := "outcome_unknown"
		if op.Phase == "applying" {
			status = "in_flight"
		} else if op.Phase == "completed" && op.Result != nil {
			if op.Result.Status == "applied" || op.Result.Status == "accepted" {
				status = "confirmed"
			} else {
				status = "rejected"
			}
		}
		digest := op.RequestDigest
		if len(digest) != 64 {
			digest = operationDigest(op.Payload)
		}
		var receipt []byte
		if op.Result != nil {
			receipt, _ = json.Marshal(op.Result)
		}
		control := OperationRecord{ProjectIdentity: project, OperationID: op.RequestID, IdempotencyKey: op.RequestID, Kind: op.Action, Status: status, PayloadDigest: digest, Request: append([]byte(nil), op.Payload...), Receipt: receipt, CreatedAt: now(), UpdatedAt: now()}
		if index, exists := indices[op.RequestID]; exists {
			control.CreatedAt = out[index].CreatedAt
			out[index] = control
		} else {
			out = append(out, control)
		}
	}
	return out
}

func centerOwnedNativeControlAction(action string) bool {
	switch action {
	case "start_task", "set_dispatch_mode", "pause_dispatch", "resume_dispatch":
		return true
	default:
		return false
	}
}

func operationDigest(payload []byte) string {
	h := sha256.Sum256(payload)
	return hex.EncodeToString(h[:])
}
