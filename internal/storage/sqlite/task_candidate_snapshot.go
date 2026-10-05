package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

// ReadTaskCandidateSnapshot returns the complete DAG plus every candidate for
// one task from a single WAL snapshot.
func (s Reader) ReadTaskCandidateSnapshot(ctx context.Context, project, taskID string) (storageports.TaskCandidateSnapshot, error) {
	if s.db == nil {
		return storageports.TaskCandidateSnapshot{}, errors.New("task candidate reader is required")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return storageports.TaskCandidateSnapshot{}, err
	}
	defer tx.Rollback()
	snapshot, err := readTaskCandidateSnapshot(ctx, tx, project, taskID, s.nativeBinding)
	if err != nil {
		return storageports.TaskCandidateSnapshot{}, err
	}
	if err := tx.Commit(); err != nil {
		return storageports.TaskCandidateSnapshot{}, err
	}
	return snapshot, nil
}

func (tx *sqliteTx) ReadTaskCandidateSnapshot(ctx context.Context, project, taskID string) (storageports.TaskCandidateSnapshot, error) {
	return readTaskCandidateSnapshot(ctx, tx.tx, project, taskID, nil)
}

func readTaskCandidateSnapshot(ctx context.Context, db graphSnapshotQuery, project, taskID string, binding *executionmodel.ProjectTaskAuthority) (storageports.TaskCandidateSnapshot, error) {
	out := storageports.TaskCandidateSnapshot{ProjectIdentity: project, TaskID: taskID, WorkflowRuns: []executionmodel.WorkflowRunRecord{}, Executions: []executionmodel.ExecutionRecord{}}
	if project == "" {
		return out, nativeErr("authority_mismatch", "project identity is required")
	}
	if err := taskmodel.ValidateID(taskID); err != nil {
		return out, err
	}
	graph, err := TaskGraphSnapshotTx(ctx, db, project, binding)
	if err != nil {
		return out, err
	}
	if _, ok := graph.ByID[taskID]; !ok {
		return out, nativeErr("task_not_found", "native task was not found")
	}
	out.Graph = graph

	rows, err := db.QueryContext(ctx, `SELECT workflow_run_id,task_id,control_revision,payload FROM workflow_runs WHERE project_identity=? AND task_id=? ORDER BY workflow_run_id`, project, taskID)
	if err != nil {
		return out, err
	}
	workflowIDs := map[string]bool{}
	for rows.Next() {
		var workflowID, rowTaskID string
		var controlRevision uint64
		var body []byte
		if err = rows.Scan(&workflowID, &rowTaskID, &controlRevision, &body); err != nil {
			rows.Close()
			return out, err
		}
		var workflow executionmodel.WorkflowRunRecord
		if err = json.Unmarshal(body, &workflow); err != nil {
			rows.Close()
			return out, err
		}
		workflow.ControlRevision = controlRevision
		if workflow.ProjectIdentity != project || workflow.WorkflowRunID != workflowID || workflow.TaskID != taskID || rowTaskID != taskID {
			rows.Close()
			return out, errors.New("candidate workflow row and payload identity differ")
		}
		workflowIDs[workflowID] = true
		out.WorkflowRuns = append(out.WorkflowRuns, workflow)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return out, err
	}
	rows.Close()

	rows, err = db.QueryContext(ctx, `SELECT e.execution_id,e.workflow_run_id,e.payload FROM executions e JOIN workflow_runs w ON w.project_identity=e.project_identity AND w.workflow_run_id=e.workflow_run_id WHERE e.project_identity=? AND w.task_id=? ORDER BY e.workflow_run_id,e.execution_id`, project, taskID)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var executionID, workflowID string
		var body []byte
		if err = rows.Scan(&executionID, &workflowID, &body); err != nil {
			rows.Close()
			return out, err
		}
		var record executionmodel.ExecutionRecord
		if err = json.Unmarshal(body, &record); err != nil {
			rows.Close()
			return out, err
		}
		if record.ProjectIdentity != project || record.ExecutionID != executionID || record.WorkflowRunID != workflowID || !workflowIDs[workflowID] {
			rows.Close()
			return out, errors.New("candidate execution row and payload identity differ")
		}
		out.Executions = append(out.Executions, record)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return out, err
	}
	rows.Close()
	return out, nil
}
