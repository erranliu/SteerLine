package steerline

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/task"
)

type taskGraphSnapshot = task.Snapshot

func (s *SQLiteStore) taskGraphSnapshot(ctx context.Context, project string) (taskGraphSnapshot, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return taskGraphSnapshot{}, err
	}
	defer tx.Rollback()
	out, err := s.taskGraphSnapshotReadTx(ctx, tx, project)
	if err != nil {
		return out, err
	}
	return out, tx.Commit()
}

func (s *SQLiteStore) taskGraphSnapshotReadTx(ctx context.Context, tx *sql.Tx, project string) (taskGraphSnapshot, error) {
	out := taskGraphSnapshot{ByID: map[string]TaskRecord{}, RepositoryTaskIDs: map[string]bool{}, ContractIDs: map[string]bool{}, Deliveries: map[string]TaskDeliveryRecord{}, SampledAt: time.Now().UTC()}
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
	if s.nativeBinding != nil && (s.nativeBinding.ProjectIdentity != project || s.nativeBinding.Repository != out.Authority.Repository || s.nativeBinding.Remote != out.Authority.Remote || s.nativeBinding.Branch != out.Authority.Branch) {
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
		var task TaskRecord
		if err = rows.Scan(&id, &status, &version, &body); err != nil {
			rows.Close()
			return out, err
		}
		if err = json.Unmarshal(body, &task); err != nil {
			rows.Close()
			return out, err
		}
		if task.ProjectIdentity != project || task.TaskID != id || task.Source != "native" || !canonicalDecimalID(id) || task.Status != status || task.Version != version || !validTaskStatus(status) || task.Version == 0 {
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
		var edge TaskDependencyRecord
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
		var d TaskDeliveryRecord
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
		if err = validateNativeTaskDelivery(d, out.Authority.Branch); err != nil {
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

func taskGraphFromStore(ctx context.Context, store DomainStore, project string) (taskGraphSnapshot, error) {
	if sqlite, ok := store.(*SQLiteStore); ok {
		return sqlite.taskGraphSnapshot(ctx, project)
	}
	return taskGraphSnapshot{}, errors.New("native DAG commands require the center SQLite store")
}

func taskGraphSnapshotTx(ctx context.Context, t *sqliteTx, project string) (taskGraphSnapshot, error) {
	out := taskGraphSnapshot{ByID: map[string]TaskRecord{}, RepositoryTaskIDs: map[string]bool{}, ContractIDs: map[string]bool{}, Deliveries: map[string]TaskDeliveryRecord{}, SampledAt: time.Now().UTC()}
	var paused int
	var created, updated string
	err := t.tx.QueryRowContext(ctx, `SELECT authority,authority_version,graph_revision,repository,remote,branch,dispatch_mode,dispatch_paused,created_at,updated_at FROM project_task_authorities WHERE project_identity=?`, project).Scan(&out.Authority.Authority, &out.Authority.AuthorityVersion, &out.Authority.GraphRevision, &out.Authority.Repository, &out.Authority.Remote, &out.Authority.Branch, &out.Authority.DispatchMode, &paused, &created, &updated)
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
	var occupied int
	if err = t.tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM executions WHERE project_identity=? AND status IN ('pending','active','waiting_decision','blocked')`, project).Scan(&occupied); err != nil {
		return out, err
	}
	out.ExecutionSlotBusy = occupied > 0
	rows, err := t.tx.QueryContext(ctx, `SELECT t.task_id FROM tasks t JOIN project_task_authorities p ON p.project_identity=t.project_identity WHERE p.authority='native' AND p.repository=?`, out.Authority.Repository)
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
	rows, err = t.tx.QueryContext(ctx, `SELECT c.contract_id FROM task_contracts c JOIN project_task_authorities p ON p.project_identity=c.project_identity WHERE p.authority='native' AND p.repository=?`, out.Authority.Repository)
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
	rows, err = t.tx.QueryContext(ctx, `SELECT task_id,status,version,payload FROM tasks WHERE project_identity=? ORDER BY length(task_id),task_id`, project)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var id, status string
		var version uint64
		var body []byte
		var task TaskRecord
		if err = rows.Scan(&id, &status, &version, &body); err != nil {
			rows.Close()
			return out, err
		}
		if err = json.Unmarshal(body, &task); err != nil {
			rows.Close()
			return out, err
		}
		if task.ProjectIdentity != project || task.TaskID != id || task.Status != status || task.Version != version || task.Source != "native" {
			rows.Close()
			return out, nativeErr("contract_mismatch", "task graph contains a mismatched native task")
		}
		out.Tasks = append(out.Tasks, task)
		out.ByID[id] = task
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return out, err
	}
	rows.Close()
	rows, err = t.tx.QueryContext(ctx, `SELECT predecessor_id,successor_id FROM task_dependencies WHERE project_identity=? ORDER BY length(predecessor_id),predecessor_id,length(successor_id),successor_id`, project)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var edge TaskDependencyRecord
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
	rows, err = t.tx.QueryContext(ctx, `SELECT task_id,selected_workflow_run_id,pr_instance,pr_repository,pr_number,pr_url,merge_commit,merged_at,recorded_at,evidence FROM task_deliveries WHERE project_identity=? ORDER BY length(task_id),task_id`, project)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var delivery TaskDeliveryRecord
		var merged, recorded string
		delivery.ProjectIdentity = project
		if err = rows.Scan(&delivery.TaskID, &delivery.SelectedWorkflowRunID, &delivery.PRInstance, &delivery.PRRepository, &delivery.PRNumber, &delivery.PRURL, &delivery.MergeCommit, &merged, &recorded, &delivery.Evidence); err != nil {
			rows.Close()
			return out, err
		}
		delivery.MergedAt, err = time.Parse(time.RFC3339Nano, merged)
		if err != nil {
			rows.Close()
			return out, nativeErr("contract_mismatch", "invalid delivery merge timestamp")
		}
		delivery.RecordedAt, err = time.Parse(time.RFC3339Nano, recorded)
		if err != nil {
			rows.Close()
			return out, nativeErr("contract_mismatch", "invalid delivery recording timestamp")
		}
		if err = validateNativeTaskDelivery(delivery, out.Authority.Branch); err != nil {
			rows.Close()
			return out, err
		}
		out.Deliveries[delivery.TaskID] = delivery
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return out, err
	}
	rows.Close()
	return out, nil
}
