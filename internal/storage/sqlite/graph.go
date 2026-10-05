package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

type graphSnapshotQuery interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (s Reader) TaskGraphSnapshot(ctx context.Context, project string) (storageports.TaskGraphSnapshot, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return storageports.TaskGraphSnapshot{}, err
	}
	defer tx.Rollback()
	out, err := s.TaskGraphSnapshotTx(ctx, tx, project)
	if err != nil {
		return out, err
	}
	return out, tx.Commit()
}

func (s Reader) TaskGraphSnapshotTx(ctx context.Context, tx graphSnapshotQuery, project string) (storageports.TaskGraphSnapshot, error) {
	return TaskGraphSnapshotTx(ctx, tx, project, s.nativeBinding)
}

func TaskGraphSnapshotTx(ctx context.Context, tx graphSnapshotQuery, project string, binding *executionmodel.ProjectTaskAuthority) (storageports.TaskGraphSnapshot, error) {
	out := storageports.TaskGraphSnapshot{ByID: map[string]taskmodel.TaskRecord{}, RepositoryTaskIDs: map[string]bool{}, ContractIDs: map[string]bool{}, Deliveries: map[string]executionmodel.TaskDeliveryRecord{}, SampledAt: time.Now().UTC()}
	if project == "" {
		return out, nativeErr("authority_mismatch", "project identity is required")
	}
	var paused int
	var created, updated string
	err := tx.QueryRowContext(ctx, `SELECT authority,authority_version,graph_revision,repository,remote,branch,dispatch_mode,dispatch_paused,created_at,updated_at FROM project_task_authorities WHERE project_identity=?`, project).Scan(&out.Authority.Authority, &out.Authority.AuthorityVersion, &out.Authority.GraphRevision, &out.Authority.Repository, &out.Authority.Remote, &out.Authority.Branch, &out.Authority.DispatchMode, &paused, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return out, nativeErr("authority_mismatch", "project has no native task authority")
	}
	if err != nil {
		return out, err
	}
	out.Authority.ProjectIdentity = project
	out.Authority.DispatchPaused = paused != 0
	out.Authority.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	out.Authority.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	if out.Authority.Authority != "native" {
		return out, nativeErr("authority_mismatch", "project does not use native task authority")
	}
	if binding != nil && (binding.ProjectIdentity != project || binding.Repository != out.Authority.Repository || binding.Remote != out.Authority.Remote || binding.Branch != out.Authority.Branch) {
		return out, nativeErr("authority_mismatch", "project binding changed after opening the query reader")
	}
	if err = tx.QueryRowContext(ctx, `SELECT database_id,revision FROM database_state WHERE singleton=1`).Scan(&out.DatabaseID, &out.DatabaseRevision); err != nil {
		return out, nativeErr("unsupported_schema", "database sampling revision is unavailable")
	}
	var occupied int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM executions WHERE project_identity=? AND status IN ('pending','active','waiting_decision','blocked')`, project).Scan(&occupied); err != nil {
		return out, err
	}
	out.ExecutionSlotBusy = occupied > 0
	rows, err := tx.QueryContext(ctx, `SELECT t.task_id FROM tasks t JOIN project_task_authorities p ON p.project_identity=t.project_identity WHERE p.authority='native' AND p.repository=?`, out.Authority.Repository)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return out, err
		}
		out.RepositoryTaskIDs[id] = true
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return out, err
	}
	rows.Close()
	rows, err = tx.QueryContext(ctx, `SELECT c.contract_id FROM task_contracts c JOIN project_task_authorities p ON p.project_identity=c.project_identity WHERE p.authority='native' AND p.repository=?`, out.Authority.Repository)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return out, err
		}
		out.ContractIDs[id] = true
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return out, err
	}
	rows.Close()
	rows, err = tx.QueryContext(ctx, `SELECT task_id,status,version,payload FROM tasks WHERE project_identity=? ORDER BY length(task_id),task_id`, project)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var id, status string
		var version uint64
		var body []byte
		var task taskmodel.TaskRecord
		if err = rows.Scan(&id, &status, &version, &body); err != nil {
			rows.Close()
			return out, err
		}
		if err = json.Unmarshal(body, &task); err != nil {
			rows.Close()
			return out, err
		}
		if task.ProjectIdentity != project || task.TaskID != id || task.Source != "native" || !taskmodel.CanonicalDecimalID(id) || task.Status != status || task.Version != version || !taskmodel.ValidStatus(status) || task.Version == 0 {
			rows.Close()
			return out, nativeErr("contract_mismatch", "native task identity or status differs from its stored row")
		}
		out.Tasks = append(out.Tasks, task)
		out.ByID[id] = task
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return out, err
	}
	rows.Close()
	rows, err = tx.QueryContext(ctx, `SELECT predecessor_id,successor_id FROM task_dependencies WHERE project_identity=? ORDER BY length(predecessor_id),predecessor_id,length(successor_id),successor_id`, project)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var edge taskmodel.TaskDependencyRecord
		edge.ProjectIdentity = project
		if err = rows.Scan(&edge.PredecessorID, &edge.SuccessorID); err != nil {
			rows.Close()
			return out, err
		}
		out.Edges = append(out.Edges, edge)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return out, err
	}
	rows.Close()
	rows, err = tx.QueryContext(ctx, `SELECT task_id,selected_workflow_run_id,pr_instance,pr_repository,pr_number,pr_url,merge_commit,merged_at,recorded_at,evidence FROM task_deliveries WHERE project_identity=? ORDER BY length(task_id),task_id`, project)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var d executionmodel.TaskDeliveryRecord
		var merged, recorded string
		d.ProjectIdentity = project
		if err = rows.Scan(&d.TaskID, &d.SelectedWorkflowRunID, &d.PRInstance, &d.PRRepository, &d.PRNumber, &d.PRURL, &d.MergeCommit, &merged, &recorded, &d.Evidence); err != nil {
			rows.Close()
			return out, err
		}
		d.MergedAt, err = time.Parse(time.RFC3339Nano, merged)
		if err != nil {
			rows.Close()
			return out, nativeErr("contract_mismatch", "invalid delivery merge timestamp")
		}
		d.RecordedAt, err = time.Parse(time.RFC3339Nano, recorded)
		if err != nil {
			rows.Close()
			return out, nativeErr("contract_mismatch", "invalid delivery recording timestamp")
		}
		if err = executionmodel.ValidateDelivery(d, out.Authority.Branch); err != nil {
			rows.Close()
			return out, err
		}
		out.Deliveries[d.TaskID] = d
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return out, err
	}
	rows.Close()
	return out, nil
}
