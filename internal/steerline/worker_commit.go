package steerline

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// workerRecordWrite names one row and the value the actor observed before this
// transition. An empty Expected value requires the row not to exist.
type workerRecordWrite[T any] struct {
	Expected string
	Record   T
}

type workerCommitRequest struct {
	Project, TaskID, WorkflowRunID, ExecutionID, AttemptID string
	AuthorityRevision                                      uint64
	ControlRevision                                        uint64
	Workflow                                               workerRecordWrite[WorkflowRunRecord]
	Execution                                              workerRecordWrite[ExecutionRecord]
	Attempt                                                workerRecordWrite[StepAttemptRecord]
	Decisions                                              []workerRecordWrite[DecisionRecord]
	DecisionReads                                          []workerRecordRead
	Operations                                             []workerRecordWrite[OperationRecord]
	Events                                                 []DomainEvent
	Checkpoint                                             ServiceCheckpoint
}

type workerRecordRead struct{ ID, Expected string }

type workerCommitResult struct {
	Workflow WorkflowRunRecord
	Revision uint64
}

func (s *SQLiteStore) workerWorkflowRevision(ctx context.Context, project, workflowID string) (uint64, error) {
	var revision uint64
	err := s.db.QueryRowContext(ctx, `SELECT control_revision FROM workflow_runs WHERE project_identity=? AND workflow_run_id=?`, project, workflowID).Scan(&revision)
	return revision, err
}

// commitWorkerTransition validates the complete declared read/write set and
// publishes its checkpoint in the same transaction. It never inspects Runtime.
func (s *SQLiteStore) commitWorkerTransition(ctx context.Context, request workerCommitRequest) (workerCommitResult, error) {
	var result workerCommitResult
	if request.Project == "" || request.TaskID == "" || request.WorkflowRunID == "" || request.ExecutionID == "" || request.AttemptID == "" ||
		request.Checkpoint.Repository.Identity != request.Project || request.Checkpoint.SchemaVersion != WorkerFlowRuntimeSchema {
		return result, errors.New("worker commit requires one fixed project, task, workflow, execution, attempt and checkpoint")
	}
	if w, e, a := request.Workflow.Record, request.Execution.Record, request.Attempt.Record; w.ProjectIdentity != request.Project || w.WorkflowRunID != request.WorkflowRunID || w.TaskID != request.TaskID ||
		e.ProjectIdentity != request.Project || e.ExecutionID != request.ExecutionID || e.WorkflowRunID != request.WorkflowRunID ||
		a.ProjectIdentity != request.Project || a.AttemptID != request.AttemptID || a.ExecutionID != request.ExecutionID || a.StepID != e.CurrentStep {
		return result, errors.New("worker commit records do not share the fixed identity")
	}
	err := s.transactSQLite(ctx, func(tx *sqliteTx) error {
		var authority string
		var control uint64
		if err := tx.tx.QueryRowContext(ctx, `SELECT authority,authority_version FROM project_task_authorities WHERE project_identity=?`, request.Project).Scan(&authority, &control); err != nil {
			return err
		}
		if authority != "native" || control != request.AuthorityRevision {
			return nativeErr("stale_revision", "worker task authority changed before commit")
		}
		var taskStatus string
		if err := tx.tx.QueryRowContext(ctx, `SELECT status FROM tasks WHERE project_identity=? AND task_id=?`, request.Project, request.TaskID).Scan(&taskStatus); err != nil {
			return err
		}
		if taskStatus == "closed" || taskStatus == "discarded" {
			return nativeErr("stale_revision", "worker task is terminal")
		}
		var currentAttempt string
		if err := tx.tx.QueryRowContext(ctx, `SELECT attempt_id FROM step_attempts WHERE project_identity=? AND execution_id=? AND step_id=? ORDER BY attempt_number DESC LIMIT 1`, request.Project, request.ExecutionID, request.Attempt.Record.StepID).Scan(&currentAttempt); err != nil {
			return err
		}
		if currentAttempt != request.AttemptID {
			return nativeErr("stale_revision", "worker attempt changed before commit")
		}
		if err := checkWorkerWrite(tx, "workflow_runs", "workflow_run_id", request.Project, request.WorkflowRunID, request.Workflow.Expected); err != nil {
			return err
		}
		if err := checkWorkerWrite(tx, "executions", "execution_id", request.Project, request.ExecutionID, request.Execution.Expected); err != nil {
			return err
		}
		if err := checkWorkerWrite(tx, "step_attempts", "attempt_id", request.Project, request.AttemptID, request.Attempt.Expected); err != nil {
			return err
		}
		var revision uint64
		if err := tx.tx.QueryRowContext(ctx, `SELECT control_revision FROM workflow_runs WHERE project_identity=? AND workflow_run_id=?`, request.Project, request.WorkflowRunID).Scan(&revision); err != nil {
			return err
		}
		if revision != request.ControlRevision {
			return nativeErr("stale_revision", "worker control revision changed before commit")
		}
		for _, d := range request.Decisions {
			v := d.Record
			if v.ProjectIdentity != request.Project || v.WorkflowRunID != request.WorkflowRunID || v.ExecutionID != request.ExecutionID || v.AttemptID != request.AttemptID {
				return errors.New("worker decision has another identity")
			}
			if err := checkWorkerWrite(tx, "decisions", "decision_id", request.Project, v.DecisionID, d.Expected); err != nil {
				return err
			}
			if d.Expected != "" {
				var body []byte
				if err := tx.tx.QueryRowContext(ctx, `SELECT payload FROM decisions WHERE project_identity=? AND decision_id=?`, request.Project, v.DecisionID).Scan(&body); err != nil {
					return err
				}
				var old DecisionRecord
				if err := json.Unmarshal(body, &old); err != nil {
					return err
				}
				if old.WorkflowRunID != request.WorkflowRunID || old.ExecutionID != request.ExecutionID || old.AttemptID != request.AttemptID {
					return errors.New("worker decision cannot be rebound")
				}
				if old.Scope != v.Scope || old.QuestionID != v.QuestionID || !old.CreatedAt.Equal(v.CreatedAt) || !bytes.Equal(old.Request, v.Request) {
					return errors.New("worker decision question identity and request are fixed")
				}
				if v.Version < old.Version || old.Status != v.Status && v.Version <= old.Version {
					return errors.New("worker decision version cannot regress")
				}
				if old.Status != v.Status && !(old.Status == "pending" && (v.Status == "answer_saved" || v.Status == "superseded") || old.Status == "answer_saved" && (v.Status == "reply_accepted" || v.Status == "superseded")) {
					return errors.New("worker decision status transition is not allowed")
				}
				if len(old.Answer) != 0 && (!bytes.Equal(old.Answer, v.Answer) || old.AnsweredBy != v.AnsweredBy) {
					return errors.New("worker decision saved answer cannot be changed")
				}
			}
		}
		for _, read := range request.DecisionReads {
			if read.ID == "" || read.Expected == "" {
				return errors.New("worker decision read requires an existing fixed identity")
			}
			if err := checkWorkerWrite(tx, "decisions", "decision_id", request.Project, read.ID, read.Expected); err != nil {
				return err
			}
			var workflowID, executionID, attemptID string
			if err := tx.tx.QueryRowContext(ctx, `SELECT workflow_run_id,execution_id,attempt_id FROM decisions WHERE project_identity=? AND decision_id=?`, request.Project, read.ID).Scan(&workflowID, &executionID, &attemptID); err != nil {
				return err
			}
			if workflowID != request.WorkflowRunID || executionID != request.ExecutionID || attemptID != request.AttemptID {
				return errors.New("worker decision read belongs to another execution")
			}
		}
		for _, o := range request.Operations {
			v := o.Record
			if v.ProjectIdentity != request.Project || v.OperationID == "" {
				return errors.New("worker operation has another identity")
			}
			var body []byte
			err := tx.tx.QueryRowContext(ctx, `SELECT payload FROM operations WHERE project_identity=? AND operation_id=?`, request.Project, v.OperationID).Scan(&body)
			if errors.Is(err, sql.ErrNoRows) && o.Expected == "" {
				continue
			}
			if errors.Is(err, sql.ErrNoRows) {
				return nativeErr("stale_revision", "worker operation changed before commit")
			}
			if err != nil {
				return err
			}
			if operationDigest(body) != o.Expected {
				return nativeErr("stale_revision", "worker operation changed before commit")
			}
			var old OperationRecord
			if err := json.Unmarshal(body, &old); err != nil {
				return err
			}
			if old.Status != v.Status && !allowedOperationTransitions[old.Status][v.Status] {
				return errors.New("worker operation status cannot regress")
			}
		}
		if err := tx.PutWorkflowRun(request.Workflow.Record); err != nil {
			return err
		}
		if err := tx.PutExecution(request.Execution.Record); err != nil {
			return err
		}
		if err := tx.PutStepAttempt(request.Attempt.Record); err != nil {
			return err
		}
		for _, d := range request.Decisions {
			if err := tx.PutDecision(d.Record); err != nil {
				return err
			}
		}
		for _, o := range request.Operations {
			persisted, _, err := tx.PutOperation(o.Record)
			if err != nil {
				return err
			}
			if persisted.OperationID != o.Record.OperationID {
				return errors.New("worker operation idempotency identity changed")
			}
		}
		for _, event := range request.Events {
			if event.ProjectIdentity != request.Project || event.TaskID != request.TaskID || event.WorkflowRunID != request.WorkflowRunID || event.ExecutionID != request.ExecutionID || event.AttemptID != "" && event.AttemptID != request.AttemptID {
				return errors.New("worker event has another identity")
			}
			if _, err := tx.AppendEvent(event); err != nil {
				return err
			}
		}
		result.Workflow = request.Workflow.Record
		if err := tx.tx.QueryRowContext(ctx, `SELECT control_revision FROM workflow_runs WHERE project_identity=? AND workflow_run_id=?`, request.Project, request.WorkflowRunID).Scan(&result.Workflow.ControlRevision); err != nil {
			return err
		}
		var oldRevision uint64
		if err := tx.tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(revision),0) FROM projection WHERE project_identity=?`, request.Project).Scan(&oldRevision); err != nil {
			return err
		}
		if oldRevision != request.Checkpoint.Revision {
			return nativeErr("stale_revision", "worker checkpoint changed before commit")
		}
		result.Revision = oldRevision + 1
		request.Checkpoint.Revision = result.Revision
		body, err := json.Marshal(request.Checkpoint)
		if err != nil {
			return err
		}
		return tx.PutProjection(request.Project, result.Revision, body)
	})
	return result, err
}

func checkWorkerWrite(tx *sqliteTx, table, column, project, id, expected string) error {
	var body []byte
	err := tx.tx.QueryRowContext(tx.ctx, "SELECT payload FROM "+table+" WHERE project_identity=? AND "+column+"=?", project, id).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) && expected == "" {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nativeErr("stale_revision", fmt.Sprintf("worker %s changed before commit", table))
	}
	if err != nil {
		return err
	}
	if operationDigest(body) != expected {
		return nativeErr("stale_revision", fmt.Sprintf("worker %s changed before commit", table))
	}
	return nil
}
