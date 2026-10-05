package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

// ReadWorkflow returns only the workflow and child facts selected by identity.
func (s Reader) ReadWorkflow(ctx context.Context, project, workflowRunID string) (storageports.WorkflowSnapshot, error) {
	if s.db == nil {
		return storageports.WorkflowSnapshot{}, errors.New("workflow reader is required")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return storageports.WorkflowSnapshot{}, err
	}
	defer tx.Rollback()
	snapshot, err := readWorkflow(ctx, tx, project, workflowRunID)
	if err != nil {
		return storageports.WorkflowSnapshot{}, err
	}
	if err := tx.Commit(); err != nil {
		return storageports.WorkflowSnapshot{}, err
	}
	return snapshot, nil
}

func readWorkflow(ctx context.Context, db graphSnapshotQuery, project, workflowRunID string) (storageports.WorkflowSnapshot, error) {
	out := storageports.WorkflowSnapshot{ProjectIdentity: project, WorkflowRuns: []executionmodel.WorkflowRunRecord{}, Executions: []executionmodel.ExecutionRecord{}, Attempts: []executionmodel.StepAttemptRecord{}, Decisions: []executionmodel.DecisionRecord{}}
	if project == "" || workflowRunID == "" {
		return out, errors.New("project and workflow identity are required")
	}
	var payload []byte
	var controlRevision uint64
	err := db.QueryRowContext(ctx, `SELECT payload,control_revision FROM workflow_runs WHERE project_identity=? AND workflow_run_id=?`, project, workflowRunID).Scan(&payload, &controlRevision)
	if errors.Is(err, sql.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	var workflow executionmodel.WorkflowRunRecord
	if err := json.Unmarshal(payload, &workflow); err != nil {
		return out, err
	}
	workflow.ControlRevision = controlRevision
	if workflow.ProjectIdentity != project || workflow.WorkflowRunID != workflowRunID {
		return out, errors.New("workflow row and payload identity differ")
	}
	out.WorkflowRuns = append(out.WorkflowRuns, workflow)

	rows, err := db.QueryContext(ctx, `SELECT execution_id,workflow_run_id,payload FROM executions WHERE project_identity=? AND workflow_run_id=? ORDER BY execution_id`, project, workflowRunID)
	if err != nil {
		return out, err
	}
	executionIDs := map[string]bool{}
	for rows.Next() {
		var executionID, rowWorkflowID string
		var body []byte
		if err = rows.Scan(&executionID, &rowWorkflowID, &body); err != nil {
			rows.Close()
			return out, err
		}
		var record executionmodel.ExecutionRecord
		if err = json.Unmarshal(body, &record); err != nil {
			rows.Close()
			return out, err
		}
		if record.ProjectIdentity != project || record.ExecutionID != executionID || record.WorkflowRunID != rowWorkflowID || rowWorkflowID != workflowRunID {
			rows.Close()
			return out, errors.New("execution row and payload identity differ")
		}
		executionIDs[executionID] = true
		out.Executions = append(out.Executions, record)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return out, err
	}
	rows.Close()

	rows, err = db.QueryContext(ctx, `SELECT a.attempt_id,a.execution_id,a.payload FROM step_attempts a JOIN executions e ON e.project_identity=a.project_identity AND e.execution_id=a.execution_id WHERE a.project_identity=? AND e.workflow_run_id=? ORDER BY a.execution_id,a.step_id,a.attempt_number`, project, workflowRunID)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var attemptID, executionID string
		var body []byte
		if err = rows.Scan(&attemptID, &executionID, &body); err != nil {
			rows.Close()
			return out, err
		}
		var record executionmodel.StepAttemptRecord
		if err = json.Unmarshal(body, &record); err != nil {
			rows.Close()
			return out, err
		}
		if record.ProjectIdentity != project || record.AttemptID != attemptID || record.ExecutionID != executionID || !executionIDs[executionID] {
			rows.Close()
			return out, errors.New("attempt row and payload identity differ")
		}
		out.Attempts = append(out.Attempts, record)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return out, err
	}
	rows.Close()

	rows, err = db.QueryContext(ctx, `SELECT decision_id,payload FROM decisions WHERE project_identity=? AND workflow_run_id=? ORDER BY decision_id`, project, workflowRunID)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var decisionID string
		var body []byte
		if err = rows.Scan(&decisionID, &body); err != nil {
			rows.Close()
			return out, err
		}
		var record executionmodel.DecisionRecord
		if err = json.Unmarshal(body, &record); err != nil {
			rows.Close()
			return out, err
		}
		if record.ProjectIdentity != project || record.DecisionID != decisionID || record.WorkflowRunID != workflowRunID {
			rows.Close()
			return out, errors.New("decision row and payload identity differ")
		}
		out.Decisions = append(out.Decisions, record)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return out, err
	}
	rows.Close()
	return out, nil
}
