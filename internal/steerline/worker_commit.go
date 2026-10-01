package steerline

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

// recordWrite declares one record and the exact value read before computing it.
// A nil Before declares creation. It is never populated by scanning Runtime.
type recordWrite[T any] struct {
	Before *T
	After  T
}

// workerCommit is the bounded write set of one worker transition. Operations
// and events may also be committed independently by their service owners.
type workerCommit struct {
	Binding            *executionapi.ExecutionRef
	ExecutionOperation *recordWrite[executionapi.Operation]
	Project            string
	TaskID             string
	WorkflowID         string
	ExecutionID        string
	AttemptID          string
	ControlRevision    uint64
	AuthorityRevision  uint64
	ExpectedAttemptID  string
	Workflow           *recordWrite[WorkflowRunRecord]
	Execution          *recordWrite[ExecutionRecord]
	Attempt            *recordWrite[StepAttemptRecord]
	RetiredAttempt     *recordWrite[StepAttemptRecord]
	Decisions          []recordWrite[DecisionRecord]
	Operations         []recordWrite[OperationRecord]
	Events             []DomainEvent
	Checkpoint         ServiceCheckpoint
}

type workerCommitResult struct {
	ControlRevision uint64
	Checkpoint      ServiceCheckpoint
}

func (s *SQLiteStore) commitWorker(ctx context.Context, request workerCommit) (workerCommitResult, error) {
	var result workerCommitResult
	err := s.transactSQLite(ctx, func(tx *sqliteTx) error {
		if request.Project == "" || request.Checkpoint.Repository.Identity != request.Project {
			return errors.New("worker commit requires its fixed project")
		}
		if request.AuthorityRevision != 0 {
			var revision uint64
			if err := tx.tx.QueryRowContext(ctx, "SELECT authority_version FROM project_task_authorities WHERE project_identity=?", request.Project).Scan(&revision); err != nil {
				return err
			}
			if revision != request.AuthorityRevision {
				return nativeErr("stale_revision", "worker authority changed before commitment")
			}

		}
		if request.WorkflowID != "" {
			var task, execution, attempt string
			var revision uint64
			if err := tx.tx.QueryRowContext(ctx, `SELECT task_id,control_revision FROM workflow_runs WHERE project_identity=? AND workflow_run_id=?`, request.Project, request.WorkflowID).Scan(&task, &revision); err != nil {
				return err
			}
			if task != request.TaskID || revision != request.ControlRevision {
				return nativeErr("stale_revision", fmt.Sprintf("worker control state changed before commitment: expected=%d current=%d task=%s/%s", request.ControlRevision, revision, request.TaskID, task))
			}
			if err := tx.tx.QueryRowContext(ctx, `SELECT workflow_run_id FROM executions WHERE project_identity=? AND execution_id=?`, request.Project, request.ExecutionID).Scan(&execution); err != nil {
				return err
			}
			if execution != request.WorkflowID {
				return nativeErr("authority_mismatch", "worker execution belongs to another workflow")
			}
			if err := tx.tx.QueryRowContext(ctx, `SELECT attempt_id FROM step_attempts WHERE project_identity=? AND execution_id=? AND step_id='worker' ORDER BY attempt_number DESC LIMIT 1`, request.Project, request.ExecutionID).Scan(&attempt); err != nil {
				return err
			}
			expected := request.ExpectedAttemptID
			if expected == "" {
				expected = request.AttemptID
			}
			if attempt != expected {
				return nativeErr("stale_revision", "worker attempt changed before commitment")
			}
		}
		// Check every declared precondition before any write advances a revision.
		if request.Workflow != nil {
			v := request.Workflow
			if v.After.ProjectIdentity != request.Project || v.After.WorkflowRunID != request.WorkflowID || v.After.TaskID != request.TaskID {
				return errors.New("worker workflow write has a different binding")
			}
			if err := checkRecordWrite(tx, "workflow_runs", "workflow_run_id", request.Project, v.After.WorkflowRunID, v.Before); err != nil {
				return err
			}
			if v.Before != nil && (recordFingerprint(v.Before.ExecutionProfile) != recordFingerprint(v.After.ExecutionProfile) || v.Before.ConfigDigest != v.After.ConfigDigest || v.Before.Repository != v.After.Repository || v.Before.Branch != v.After.Branch) {
				return errors.New("worker transition cannot replace its original execution snapshot")
			}
		}
		if request.Execution != nil {
			v := request.Execution
			if v.After.ProjectIdentity != request.Project || v.After.ExecutionID != request.ExecutionID || v.After.WorkflowRunID != request.WorkflowID {
				return errors.New("worker execution write has a different binding")
			}
			if err := checkRecordWrite(tx, "executions", "execution_id", request.Project, v.After.ExecutionID, v.Before); err != nil {
				return err
			}
		}
		if request.Attempt != nil {
			v := request.Attempt
			if v.After.ProjectIdentity != request.Project || v.After.AttemptID != request.AttemptID || v.After.ExecutionID != request.ExecutionID || v.After.StepID != "worker" {
				return errors.New("worker attempt write has a different binding")
			}
			if err := checkRecordWrite(tx, "step_attempts", "attempt_id", request.Project, v.After.AttemptID, v.Before); err != nil {
				return err
			}
		}
		if v := request.RetiredAttempt; v != nil {
			if v.Before == nil || v.After.AttemptID != request.ExpectedAttemptID || v.After.ExecutionID != request.ExecutionID ||
				v.After.ProjectIdentity != request.Project || v.After.StepID != "worker" ||
				request.Attempt == nil || request.Attempt.Before != nil || request.Attempt.After.AttemptNumber != v.Before.AttemptNumber+1 ||
				request.Execution == nil || request.Execution.Before == nil || request.Execution.After.CurrentAttemptID != request.AttemptID ||
				request.Execution.Before.Protocol != string(executionapi.ProtocolLocalCodexAppServer) || !request.Execution.Before.RecoveryRequired ||
				request.Execution.Before.RecoveryState != "interrupted" && request.Execution.Before.RecoveryState != "completed" ||
				v.After.InputRevision != v.Before.InputRevision || v.After.ContractDigest != v.Before.ContractDigest ||
				request.Attempt.After.InputRevision != v.Before.InputRevision || request.Attempt.After.ContractDigest != v.Before.ContractDigest ||
				recordFingerprint(v.After.ExternalBinding) != recordFingerprint(v.Before.ExternalBinding) ||
				!v.Before.CreatedAt.Equal(v.After.CreatedAt) || v.After.Status != "stopped" {
				return errors.New("recovery must retire the original worker Attempt and create its next generation")
			}
			if err := checkRecordWrite(tx, "step_attempts", "attempt_id", request.Project, v.After.AttemptID, v.Before); err != nil {
				return err
			}
		}
		for _, v := range request.Decisions {
			if v.After.ProjectIdentity != request.Project || v.After.WorkflowRunID != request.WorkflowID || v.After.ExecutionID != request.ExecutionID || v.After.AttemptID != request.AttemptID && (v.Before == nil || v.Before.AttemptID != v.After.AttemptID) {
				return errors.New("worker decision write has a different binding")
			}
			if err := checkRecordWrite(tx, "decisions", "decision_id", request.Project, v.After.DecisionID, v.Before); err != nil {
				return err
			}
		}
		for _, v := range request.Operations {
			if v.After.ProjectIdentity != request.Project {
				return errors.New("operation write belongs to another project")
			}
			if err := checkRecordWrite(tx, "operations", "operation_id", request.Project, v.After.OperationID, v.Before); err != nil {
				return err
			}
			if v.Before != nil && v.Before.Status != v.After.Status && !allowedOperationTransitions[v.Before.Status][v.After.Status] {
				return fmt.Errorf("operation transition %s -> %s is not allowed", v.Before.Status, v.After.Status)
			}
			if v.Before != nil && recordFingerprint(v.Before.LifecycleIdentity) != recordFingerprint(v.After.LifecycleIdentity) {
				return errors.New("worker operation cannot replace its original lifecycle binding identity")
			}
			if v.Before == nil && v.After.Status != "pending" && v.After.Status != "in_flight" {
				return errors.New("new operation requires an original pending or in-flight intent")
			}
		}
		if request.WorkflowID != "" {
			var status string
			if err := tx.tx.QueryRowContext(ctx, `SELECT status FROM tasks WHERE project_identity=? AND task_id=?`, request.Project, request.TaskID).Scan(&status); err != nil {
				return err
			}
			if status != "in_progress" {
				// Completed/discarded tasks may retain diagnostic and cleanup
				// receipts, but cannot restart business work or replace evidence.

				if v := request.Workflow; v != nil && recordWriteChanged(*v) {
					return nativeErr("task_not_active", "worker workflow is frozen after task settlement")
				}
				if v := request.Execution; v != nil && (v.Before == nil || deliveredExecutionFingerprint(*v.Before) != deliveredExecutionFingerprint(v.After)) {
					return nativeErr("task_not_active", "worker execution is frozen after task settlement")
				}
				if v := request.Attempt; v != nil && recordWriteChanged(*v) {
					return nativeErr("task_not_active", "worker attempt is frozen after task settlement")
				}
				for _, v := range request.Decisions {
					if recordWriteChanged(v) {
						return nativeErr("task_not_active", "worker decisions are frozen after task settlement")
					}
				}
				for _, v := range request.Operations {
					if v.Before == nil && v.After.Kind != "orca_native_terminal_close" && v.After.Kind != "orca_remote_release" && v.After.Kind != "orca_remote_coordinator_close" && v.After.Kind != "cleanup_resources" {
						return nativeErr("task_not_active", "settled worker cannot start another action")
					}
				}
			}
		}
		for _, v := range request.Decisions {
			if v.Before == nil {
				continue
			}
			old, next := *v.Before, v.After
			if old.WorkflowRunID != next.WorkflowRunID || old.ExecutionID != next.ExecutionID || old.AttemptID != next.AttemptID ||
				old.Scope != next.Scope || old.QuestionID != next.QuestionID || !old.CreatedAt.Equal(next.CreatedAt) || !bytes.Equal(old.Request, next.Request) {
				return errors.New("worker decision question identity and request are fixed")
			}
			if next.Version < old.Version || old.Status != next.Status && next.Version <= old.Version {
				return errors.New("worker decision version cannot regress")
			}
			if old.Status != next.Status && !(old.Status == "pending" && (next.Status == "answer_saved" || next.Status == "superseded") ||
				old.Status == "answer_saved" && (next.Status == "reply_accepted" || next.Status == "superseded" || next.Status == "applied")) {
				return errors.New("worker decision status transition is not allowed")
			}
			if len(old.Answer) != 0 && (!bytes.Equal(old.Answer, next.Answer) || old.AnsweredBy != next.AnsweredBy) {
				return errors.New("worker decision saved answer cannot be changed")
			}
		}

		if v := request.Workflow; v != nil && recordWriteChanged(*v) {
			if err := tx.PutWorkflowRun(v.After); err != nil {
				return err
			}
		}
		if v := request.Execution; v != nil && recordWriteChanged(*v) {
			if err := tx.PutExecution(v.After); err != nil {
				return err
			}
		}
		for _, v := range []*recordWrite[StepAttemptRecord]{request.RetiredAttempt, request.Attempt} {
			if v != nil && recordWriteChanged(*v) {
				if err := tx.PutStepAttempt(v.After); err != nil {
					return err
				}
			}
		}
		for _, v := range request.Decisions {
			if recordWriteChanged(v) {
				if err := tx.PutDecision(v.After); err != nil {
					return err
				}
			}
		}
		for _, v := range request.Operations {
			if !recordWriteChanged(v) {
				continue
			}
			persisted, _, err := tx.PutOperation(v.After)
			if err != nil {
				return err
			}
			if persisted.OperationID != v.After.OperationID {
				return errors.New("operation intent already has another identity")
			}
		}
		for _, event := range request.Events {
			if event.ProjectIdentity != request.Project || request.WorkflowID != "" && (event.WorkflowRunID != request.WorkflowID || event.TaskID != request.TaskID || event.ExecutionID != request.ExecutionID || event.AttemptID != "" && event.AttemptID != request.AttemptID && event.AttemptID != request.ExpectedAttemptID) {
				return errors.New("worker event has a different binding")
			}
			if _, err := tx.AppendEvent(event); err != nil {
				return err
			}
		}
		if ref := request.Binding; ref != nil {
			if request.Execution == nil || ref.RunID != request.WorkflowID || ref.AttemptID != request.AttemptID || request.Execution.After.Process == nil || ref.External != *request.Execution.After.Process || ref.WorkspacePath != request.Execution.After.Workspace.Worktree || ref.WorkspaceBranch != request.Execution.After.Workspace.Branch || ref.BaseCommit != request.Execution.After.Workspace.Base {
				return errors.New("execution binding differs from the committed receipt")
			}
			execution := request.Execution.After
			if string(ref.Backend) != execution.Backend || string(ref.Protocol) != execution.Protocol || ref.Agent.Name != execution.Agent || ref.Target.ID != execution.Target.ID || ref.Target.ConnectionID != execution.Target.ConnectionID || ref.Target.ExternalRuntime != execution.Target.ExternalRuntime || ref.Generation != execution.SessionGeneration {
				return errors.New("execution binding changed the fixed backend or generation")
			}
			if err := tx.putExecutionBinding(*ref); err != nil {
				return err
			}
		}
		if write := request.ExecutionOperation; write != nil {
			if write.After.RunID != request.WorkflowID || write.After.AttemptID != request.AttemptID || write.After.Ref == nil || request.Execution == nil || request.Execution.After.Process == nil || write.After.Ref.External != *request.Execution.After.Process {
				return errors.New("execution operation differs from its original worker")
			}
			expected := ""
			if write.Before != nil {
				expected = recordFingerprint(*write.Before)
			}
			if err := tx.putExecutionOperation(write.After, expected); err != nil {
				return err
			}
		}
		if request.WorkflowID != "" {
			if err := tx.tx.QueryRowContext(ctx, `SELECT control_revision FROM workflow_runs WHERE project_identity=? AND workflow_run_id=?`, request.Project, request.WorkflowID).Scan(&result.ControlRevision); err != nil {
				return err
			}
		}
		var err error
		result.Checkpoint, err = tx.publishCheckpoint(request.Checkpoint)
		return err
	})
	return result, err
}

func recordWriteChanged[T any](write recordWrite[T]) bool {
	return write.Before == nil || recordFingerprint(*write.Before) != recordFingerprint(write.After)
}

func checkRecordWrite[T any](tx *sqliteTx, table, column, project, id string, expected *T) error {
	var body []byte
	var err error
	if table == "workflow_runs" {
		var revision uint64
		err = tx.tx.QueryRowContext(tx.ctx, `SELECT payload,control_revision FROM workflow_runs WHERE project_identity=? AND workflow_run_id=?`, project, id).Scan(&body, &revision)
		if err == nil {
			var workflow WorkflowRunRecord
			if err = json.Unmarshal(body, &workflow); err != nil {
				return err
			}
			workflow.ControlRevision = revision
			body = mustJSON(workflow)
		}
	} else {
		err = tx.tx.QueryRowContext(tx.ctx, "SELECT payload FROM "+table+" WHERE project_identity=? AND "+column+"=?", project, id).Scan(&body)
	}
	if errors.Is(err, sql.ErrNoRows) && expected == nil {
		return nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err != nil || expected == nil || operationDigest(body) != recordFingerprint(*expected) {
		return nativeErr("stale_revision", "worker record changed before commitment: "+table+":"+id)
	}
	return nil
}
