package steerline

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

func (t *sqliteTx) PutWorkflowRun(v WorkflowRunRecord) error {
	if v.ProjectIdentity == "" || v.WorkflowRunID == "" || v.TaskID == "" || v.Phase == "" {
		return errors.New("workflow run identity, task and phase are required")
	}
	if !validWorkflowPhase(v.Phase) {
		return fmt.Errorf("invalid workflow run phase %q", v.Phase)
	}
	if v.WorkflowVersion == WorkerFlowVersion && v.Phase != "queued" && v.Phase != "executing" && v.Phase != "waiting_decision" && v.Phase != "waiting_pr_merge" && v.Phase != "completed" {
		return fmt.Errorf("invalid worker flow phase %q", v.Phase)
	}
	var authority string
	authorityErr := t.tx.QueryRowContext(t.ctx, `SELECT authority FROM project_task_authorities WHERE project_identity=?`, v.ProjectIdentity).Scan(&authority)
	if authorityErr != nil && !errors.Is(authorityErr, sql.ErrNoRows) {
		return authorityErr
	}
	if authorityErr == nil && authority == "native" {
		var previous []byte
		err := t.tx.QueryRowContext(t.ctx, `SELECT payload FROM workflow_runs WHERE project_identity=? AND workflow_run_id=?`, v.ProjectIdentity, v.WorkflowRunID).Scan(&previous)
		if err == nil {
			var old WorkflowRunRecord
			if err = json.Unmarshal(previous, &old); err != nil {
				return err
			}
			oldPin, pinErr := nativeWorkflowPin(old)
			if pinErr != nil {
				return pinErr
			}
			newPin, pinErr := nativeWorkflowPin(v)
			if pinErr != nil {
				return pinErr
			}
			if old.ProjectIdentity != v.ProjectIdentity || old.WorkflowRunID != v.WorkflowRunID || old.TaskID != v.TaskID || !old.CreatedAt.Equal(v.CreatedAt) || !bytes.Equal(oldPin, newPin) || old.SelectedResult != "" && old.SelectedResult != v.SelectedResult {
				return nativeErr("immutable_task", "native workflow identity, definition, runner and selected result are fixed")
			}
			if old.WorkflowVersion == WorkerFlowVersion && old.PRURL != "" && old.PRURL != v.PRURL && old.PRState != "closed_unmerged" {
				return nativeErr("immutable_task", "bound worker PR cannot change before it is closed unmerged")
			}
			if old.Phase == "completed" && (v.Phase != old.Phase || v.PendingDecisions != old.PendingDecisions || !v.UpdatedAt.Equal(old.UpdatedAt)) {
				return nativeErr("immutable_task", "completed native workflow history cannot be changed")
			}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if v.Phase == "completed" {
			var deliveries int
			if err := t.tx.QueryRowContext(t.ctx, `SELECT COUNT(*) FROM task_deliveries WHERE project_identity=? AND task_id=? AND selected_workflow_run_id=?`, v.ProjectIdentity, v.TaskID, v.WorkflowRunID).Scan(&deliveries); err != nil {
				return err
			}
			if deliveries != 1 {
				return nativeErr("inconsistent_delivery", "native workflow cannot complete without its selected delivery")
			}
		}
	}
	if err := t.advanceControlRevision("workflow_runs", "workflow_run_id", v.ProjectIdentity, v.WorkflowRunID, v.WorkflowRunID, v); err != nil {
		return err
	}
	b, err := encodePayload(v)
	if err != nil {
		return err
	}
	result, err := t.tx.ExecContext(t.ctx, `INSERT INTO workflow_runs(project_identity,workflow_run_id,task_id,phase,payload) VALUES(?,?,?,?,?) ON CONFLICT(project_identity,workflow_run_id) DO UPDATE SET phase=excluded.phase,payload=excluded.payload WHERE workflow_runs.task_id=excluded.task_id`, v.ProjectIdentity, v.WorkflowRunID, v.TaskID, v.Phase, b)
	return requireIdentityWrite(result, err, "workflow identity cannot be rebound to another task")
}

func nativeWorkflowPin(v WorkflowRunRecord) ([]byte, error) {
	return encodePayload(struct {
		WorkflowVersion string
		ConfigDigest    string
		Runner          StepRunnerSettings
		Repository      string
		Branch          string
	}{v.WorkflowVersion, v.ConfigDigest, v.Runner, v.Repository, v.Branch})
}
func (t *sqliteTx) PutExecution(v ExecutionRecord) error {
	if v.ProjectIdentity == "" || v.ExecutionID == "" || v.WorkflowRunID == "" || v.Status == "" {
		return errors.New("execution identity, workflow and status are required")
	}
	if !validExecutionStatus(v.Status) {
		return fmt.Errorf("invalid execution status %q", v.Status)
	}
	b, err := encodePayload(v)
	if err != nil {
		return err
	}
	var previous []byte
	err = t.tx.QueryRowContext(t.ctx, `SELECT payload FROM executions WHERE project_identity=? AND execution_id=?`, v.ProjectIdentity, v.ExecutionID).Scan(&previous)
	if err == nil && !bytes.Equal(previous, b) {
		var old ExecutionRecord
		if err = json.Unmarshal(previous, &old); err != nil {
			return err
		}
		newWorkerGeneration := old.CurrentAttemptID != "" && v.CurrentAttemptID != old.CurrentAttemptID &&
			v.RecoveryVersion >= old.RecoveryVersion && v.RecoveryState == "resuming"
		if !newWorkerGeneration && old.SessionID != "" && old.SessionID != v.SessionID || v.TurnNumber < old.TurnNumber ||
			!newWorkerGeneration && v.TurnNumber == old.TurnNumber && old.CurrentTurnID != "" && old.CurrentTurnID != v.CurrentTurnID ||
			!newWorkerGeneration && v.TurnNumber == old.TurnNumber && old.LastStoppedTurnID != "" && old.LastStoppedTurnID != v.LastStoppedTurnID && v.LastStoppedTurnID != v.CurrentTurnID {
			return nativeErr("immutable_task", "worker session or observed turn identity cannot be replaced or rewound")
		}
		delivered, e := t.selectedNativeDeliveryExecution(v.ProjectIdentity, v.ExecutionID)
		if e != nil {
			return e
		}
		if delivered && deliveredExecutionFingerprint(old) != deliveredExecutionFingerprint(v) {
			return nativeErr("immutable_task", "selected delivered execution history cannot be changed")
		}
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err := t.advanceControlRevision("executions", "execution_id", v.ProjectIdentity, v.ExecutionID, v.WorkflowRunID, v); err != nil {
		return err
	}
	target, err := encodePayload(v.Target)
	if err != nil {
		return err
	}
	result, err := t.tx.ExecContext(t.ctx, `INSERT INTO executions(project_identity,execution_id,workflow_run_id,status,current_step,target_json,payload) VALUES(?,?,?,?,?,?,?) ON CONFLICT(project_identity,execution_id) DO UPDATE SET status=excluded.status,current_step=excluded.current_step,target_json=excluded.target_json,payload=excluded.payload WHERE executions.workflow_run_id=excluded.workflow_run_id`, v.ProjectIdentity, v.ExecutionID, v.WorkflowRunID, v.Status, v.CurrentStep, target, b)
	return requireIdentityWrite(result, err, "execution identity cannot be rebound to another workflow")
}
func (t *sqliteTx) PutStepAttempt(v StepAttemptRecord) error {
	if v.ProjectIdentity == "" || v.AttemptID == "" || v.ExecutionID == "" || v.StepID == "" || v.AttemptNumber < 1 || v.Status == "" {
		return errors.New("step attempt identity, step, positive number and status are required")
	}
	if !validStepAttemptStatus(v.Status) {
		return fmt.Errorf("invalid step attempt status %q", v.Status)
	}
	b, err := encodePayload(v)
	if err != nil {
		return err
	}
	var previous []byte
	err = t.tx.QueryRowContext(t.ctx, `SELECT payload FROM step_attempts WHERE project_identity=? AND attempt_id=?`, v.ProjectIdentity, v.AttemptID).Scan(&previous)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil && !bytes.Equal(previous, b) {
		var old StepAttemptRecord
		if err := json.Unmarshal(previous, &old); err != nil {
			return err
		}
		if old.WorkerProcess != nil && (v.WorkerProcess == nil || *old.WorkerProcess != *v.WorkerProcess) ||
			old.WorkerSessionID != "" && old.WorkerSessionID != v.WorkerSessionID ||
			old.Status == "stopped" && v.Status != "stopped" {
			return nativeErr("immutable_task", "retired worker Attempt identity or status cannot be replaced")
		}
		delivered, e := t.selectedNativeDeliveryExecution(v.ProjectIdentity, v.ExecutionID)
		if e != nil {
			return e
		}
		if delivered {
			return nativeErr("immutable_task", "selected delivered attempt history cannot be changed")
		}
	}
	result, err := t.tx.ExecContext(t.ctx, `INSERT INTO step_attempts(project_identity,attempt_id,execution_id,step_id,attempt_number,operation_id,status,payload) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(project_identity,attempt_id) DO UPDATE SET operation_id=excluded.operation_id,status=excluded.status,payload=excluded.payload WHERE step_attempts.execution_id=excluded.execution_id AND step_attempts.step_id=excluded.step_id AND step_attempts.attempt_number=excluded.attempt_number`, v.ProjectIdentity, v.AttemptID, v.ExecutionID, v.StepID, v.AttemptNumber, v.OperationID, v.Status, b)
	return requireIdentityWrite(result, err, "attempt identity cannot be rebound to another execution or step")
}

func (t *sqliteTx) selectedNativeDeliveryExecution(project, executionID string) (bool, error) {
	var runID string
	err := t.tx.QueryRowContext(t.ctx, `SELECT workflow_run_id FROM executions WHERE project_identity=? AND execution_id=?`, project, executionID).Scan(&runID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var taskID string
	var body []byte
	if err = t.tx.QueryRowContext(t.ctx, `SELECT task_id,payload FROM workflow_runs WHERE project_identity=? AND workflow_run_id=?`, project, runID).Scan(&taskID, &body); err != nil {
		return false, nativeErr("inconsistent_delivery", "selected execution has no matching workflow")
	}
	var workflow WorkflowRunRecord
	if err = json.Unmarshal(body, &workflow); err != nil {
		return false, err
	}
	if workflow.ProjectIdentity != project || workflow.WorkflowRunID != runID || workflow.TaskID != taskID {
		return false, nativeErr("inconsistent_delivery", "selected workflow payload differs from its stored identity")
	}
	if workflow.SelectedResult != executionID {
		return false, nil
	}
	var deliveries int
	if err = t.tx.QueryRowContext(t.ctx, `SELECT COUNT(*) FROM task_deliveries WHERE project_identity=? AND task_id=? AND selected_workflow_run_id=?`, project, taskID, runID).Scan(&deliveries); err != nil {
		return false, err
	}
	return deliveries == 1, nil
}
