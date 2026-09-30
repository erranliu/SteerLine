package steerline

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

// commitWorkerFeedbackPhase persists the transition before feedback can reach
// an external worker. The service applies the returned record to its actor view.
func (s *SQLiteStore) commitWorkerFeedbackPhase(ctx context.Context, expectedWorkflow WorkflowRunRecord, expectedExecution ExecutionRecord, expectedDecision DecisionRecord) (WorkflowRunRecord, error) {
	var committed WorkflowRunRecord
	err := s.transactSQLite(ctx, func(tx *sqliteTx) error {
		project := expectedWorkflow.ProjectIdentity
		if project == "" || expectedWorkflow.WorkflowRunID == "" || expectedExecution.ExecutionID == "" || expectedDecision.DecisionID == "" {
			return errors.New("worker feedback phase requires fixed workflow, execution and decision identities")
		}
		var workflowPayload, executionPayload, decisionPayload []byte
		var revision uint64
		if err := tx.tx.QueryRowContext(ctx, `SELECT payload,control_revision FROM workflow_runs WHERE project_identity=? AND workflow_run_id=?`, project, expectedWorkflow.WorkflowRunID).Scan(&workflowPayload, &revision); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nativeErr("stale_revision", "worker feedback workflow is unavailable")
			}
			return err
		}
		if err := tx.tx.QueryRowContext(ctx, `SELECT payload FROM executions WHERE project_identity=? AND execution_id=?`, project, expectedExecution.ExecutionID).Scan(&executionPayload); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nativeErr("stale_revision", "worker feedback execution is unavailable")
			}
			return err
		}
		if err := tx.tx.QueryRowContext(ctx, `SELECT payload FROM decisions WHERE project_identity=? AND decision_id=?`, project, expectedDecision.DecisionID).Scan(&decisionPayload); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nativeErr("stale_revision", "saved worker feedback is unavailable")
			}
			return err
		}
		var workflow WorkflowRunRecord
		var execution ExecutionRecord
		var decision DecisionRecord
		if err := json.Unmarshal(workflowPayload, &workflow); err != nil {
			return err
		}
		if err := json.Unmarshal(executionPayload, &execution); err != nil {
			return err
		}
		if err := json.Unmarshal(decisionPayload, &decision); err != nil {
			return err
		}
		workflow.ControlRevision = revision
		if revision != expectedWorkflow.ControlRevision || recordFingerprint(workflow) != recordFingerprint(expectedWorkflow) ||
			recordFingerprint(execution) != recordFingerprint(expectedExecution) || recordFingerprint(decision) != recordFingerprint(expectedDecision) {
			return nativeErr("stale_revision", "worker feedback state changed before phase commitment")
		}
		var currentAttempt string
		if err := tx.tx.QueryRowContext(ctx, `SELECT attempt_id FROM step_attempts WHERE project_identity=? AND execution_id=? AND step_id=? ORDER BY attempt_number DESC LIMIT 1`, project, execution.ExecutionID, execution.CurrentStep).Scan(&currentAttempt); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nativeErr("stale_revision", "worker feedback attempt is unavailable")
			}
			return err
		}
		if currentAttempt != decision.AttemptID {
			return nativeErr("stale_revision", "saved worker feedback belongs to another attempt")
		}
		if err := BeginWorkerFeedbackDelivery(&workflow, execution, decision, now()); err != nil {
			return err
		}
		if err := tx.PutWorkflowRun(workflow); err != nil {
			return err
		}
		if err := tx.tx.QueryRowContext(ctx, `SELECT control_revision FROM workflow_runs WHERE project_identity=? AND workflow_run_id=?`, project, workflow.WorkflowRunID).Scan(&workflow.ControlRevision); err != nil {
			return err
		}
		committed = workflow
		return nil
	})
	return committed, err
}
