package steerline

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

type nativePageContract struct {
	id      string
	digest  string
	payload []byte
}

type nativePageRelations struct {
	statuses   map[string]string
	deliveries map[string]TaskDeliveryRecord
	contracts  map[string]nativePageContract
	runs       map[string][]WorkflowRunRecord
}

// Keep IN lists below SQLite's older 999-variable limit, including project_id.
const nativePageQueryBatch = 400

func nativePageIDFilter(project string, ids []string) (string, []any) {
	args := make([]any, 0, len(ids)+1)
	args = append(args, project)
	for _, id := range ids {
		args = append(args, id)
	}
	return "project_identity=? AND task_id IN (" + strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",") + ")", args
}

func loadNativePageRelations(ctx context.Context, tx *sql.Tx, project, targetBranch string, tasks []TaskRecord, edges []TaskDependencyRecord) (nativePageRelations, error) {
	out := nativePageRelations{statuses: map[string]string{}, deliveries: map[string]TaskDeliveryRecord{}, contracts: map[string]nativePageContract{}, runs: map[string][]WorkflowRunRecord{}}
	selected := make(map[string]bool, len(tasks))
	selectedIDs := make([]string, 0, len(tasks))
	for _, task := range tasks {
		selected[task.TaskID] = true
		selectedIDs = append(selectedIDs, task.TaskID)
		out.statuses[task.TaskID] = task.Status
	}
	relevant := append([]string(nil), selectedIDs...)
	predecessorIDs := []string{}
	seen := make(map[string]bool, len(selected))
	for id := range selected {
		seen[id] = true
	}
	for _, edge := range edges {
		if selected[edge.SuccessorID] && !seen[edge.PredecessorID] {
			seen[edge.PredecessorID] = true
			relevant = append(relevant, edge.PredecessorID)
			predecessorIDs = append(predecessorIDs, edge.PredecessorID)
		}
	}
	for start := 0; start < len(predecessorIDs); start += nativePageQueryBatch {
		end := min(start+nativePageQueryBatch, len(predecessorIDs))
		filter, args := nativePageIDFilter(project, predecessorIDs[start:end])
		rows, err := tx.QueryContext(ctx, `SELECT task_id,status,version,payload FROM tasks WHERE `+filter, args...)
		if err != nil {
			return out, err
		}
		for rows.Next() {
			var id, status string
			var version uint64
			var body []byte
			if err = rows.Scan(&id, &status, &version, &body); err != nil {
				break
			}
			var task TaskRecord
			if err = json.Unmarshal(body, &task); err != nil {
				break
			}
			if task.ProjectIdentity != project || task.TaskID != id || task.Source != "native" || !canonicalDecimalID(id) || task.Status != status || task.Version != version || !validTaskStatus(status) || version == 0 {
				err = nativeErr("contract_mismatch", "predecessor task identity or status differs from its stored row")
				break
			}
			out.statuses[id] = status
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		if err != nil {
			return out, err
		}
	}
	for start := 0; start < len(relevant); start += nativePageQueryBatch {
		end := min(start+nativePageQueryBatch, len(relevant))
		filter, args := nativePageIDFilter(project, relevant[start:end])
		rows, err := tx.QueryContext(ctx, `SELECT task_id,selected_workflow_run_id,pr_instance,pr_repository,pr_number,pr_url,merge_commit,merged_at,recorded_at,evidence FROM task_deliveries WHERE `+filter, args...)
		if err != nil {
			return out, err
		}
		for rows.Next() {
			var delivery TaskDeliveryRecord
			var merged, recorded string
			delivery.ProjectIdentity = project
			if err = rows.Scan(&delivery.TaskID, &delivery.SelectedWorkflowRunID, &delivery.PRInstance, &delivery.PRRepository, &delivery.PRNumber, &delivery.PRURL, &delivery.MergeCommit, &merged, &recorded, &delivery.Evidence); err != nil {
				break
			}
			if delivery.MergedAt, err = time.Parse(time.RFC3339Nano, merged); err != nil {
				err = nativeErr("inconsistent_delivery", "native task delivery merge time is invalid")
				break
			}
			if delivery.RecordedAt, err = time.Parse(time.RFC3339Nano, recorded); err != nil {
				err = nativeErr("inconsistent_delivery", "native task delivery recording time is invalid")
				break
			}
			if err = validateNativeTaskDelivery(delivery, targetBranch); err != nil {
				break
			}
			out.deliveries[delivery.TaskID] = delivery
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		if err != nil {
			return out, err
		}
	}
	for start := 0; start < len(selectedIDs); start += nativePageQueryBatch {
		end := min(start+nativePageQueryBatch, len(selectedIDs))
		filter, args := nativePageIDFilter(project, selectedIDs[start:end])
		rows, err := tx.QueryContext(ctx, `SELECT task_id,contract_id,digest,payload FROM task_contracts WHERE `+filter, args...)
		if err != nil {
			return out, err
		}
		for rows.Next() {
			var id string
			var contract nativePageContract
			if err = rows.Scan(&id, &contract.id, &contract.digest, &contract.payload); err != nil {
				break
			}
			out.contracts[id] = contract
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		if err != nil {
			return out, err
		}
		rows, err = tx.QueryContext(ctx, `SELECT task_id,workflow_run_id,phase,payload FROM workflow_runs WHERE `+filter+` ORDER BY task_id,workflow_run_id`, args...)
		if err != nil {
			return out, err
		}
		for rows.Next() {
			var taskID, runID, phase string
			var body []byte
			if err = rows.Scan(&taskID, &runID, &phase, &body); err != nil {
				break
			}
			var run WorkflowRunRecord
			if err = json.Unmarshal(body, &run); err != nil {
				break
			}
			if run.ProjectIdentity != project || run.TaskID != taskID || run.WorkflowRunID != runID || run.Phase != phase {
				err = nativeErr("contract_mismatch", "workflow payload differs from its stored task binding")
				break
			}
			out.runs[taskID] = append(out.runs[taskID], run)
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		if err != nil {
			return out, err
		}
	}
	return out, nil
}
