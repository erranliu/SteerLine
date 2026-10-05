package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	queryrules "github.com/erranliu/SteerLine/internal/product/query"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

var requestIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

func nativeErr(code, message string) error { return &taskmodel.Error{Code: code, Message: message} }

func (s Reader) NativeTaskPage(ctx context.Context, project, taskID, expectedDatabaseID string, expectedDB uint64, offset, limit int) (contracts.NativeTaskPage, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return contracts.NativeTaskPage{}, err
	}
	defer tx.Rollback()
	page, err := s.NativeTaskPageTx(ctx, tx, project, taskID, expectedDatabaseID, expectedDB, offset, limit)
	if err != nil {
		return page, err
	}
	return page, tx.Commit()
}

func (s Reader) NativeTaskPageTx(ctx context.Context, tx *sql.Tx, project, taskID, expectedDatabaseID string, expectedDB uint64, offset, limit int) (contracts.NativeTaskPage, error) {
	page := contracts.NativeTaskPage{Protocol: queryrules.NativeTaskProtocol, ProjectID: project, Tasks: []contracts.NativeTaskDetail{}, Capabilities: contracts.NativeTaskCapabilities{DependencyReadiness: "provided", DependencyBlockers: "provided", StartupBlockers: "provided"}}
	if offset < 0 {
		return page, nativeErr("cursor_invalidated", "negative page offset")
	}
	if project == "" {
		return page, nativeErr("authority_mismatch", "project identity is required")
	}
	var err error
	var schemaVersion int
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&schemaVersion); err != nil || schemaVersion != SchemaVersion {
		if expectedDB != 0 {
			return page, nativeErr("cursor_invalidated", "cursor database schema is unavailable")
		}
		return page, nativeErr("unsupported_schema", "native task query requires the current database schema")
	}
	var targetRepository, targetRemote, targetBranch, dispatchMode string
	var dispatchPaused int
	err = tx.QueryRowContext(ctx, `SELECT authority_version,graph_revision,repository,remote,branch,dispatch_mode,dispatch_paused FROM project_task_authorities WHERE project_identity=? AND authority='native'`, project).Scan(&page.AuthorityVersion, &page.GraphRevision, &targetRepository, &targetRemote, &targetBranch, &dispatchMode, &dispatchPaused)
	if errors.Is(err, sql.ErrNoRows) {
		if expectedDB != 0 {
			return page, nativeErr("cursor_invalidated", "project authority could not be read for this cursor")
		}
		return page, nativeErr("authority_mismatch", "project has no native task authority")
	}
	if err != nil {
		if expectedDB != 0 {
			return page, nativeErr("cursor_invalidated", "project authority could not be read for this cursor")
		}
		return page, nativeErr("unsupported_schema", "native project authority schema is unavailable")
	}
	var dbRevision uint64
	if err = tx.QueryRowContext(ctx, `SELECT database_id,revision FROM database_state WHERE singleton=1`).Scan(&page.DatabaseID, &dbRevision); err != nil {
		if expectedDB != 0 {
			return page, nativeErr("cursor_invalidated", "database sampling revision is unavailable")
		}
		return page, nativeErr("unsupported_schema", "database sampling revision is unavailable")
	}
	if expectedDB != 0 && (expectedDB != dbRevision || expectedDatabaseID != page.DatabaseID) {
		return page, nativeErr("cursor_invalidated", "database revision changed between pages")
	}
	page.DatabaseRevision = dbRevision
	if binding := s.nativeBinding; binding != nil && (binding.ProjectIdentity != project || binding.Repository != targetRepository || binding.Remote != targetRemote || binding.Branch != targetBranch) {
		return page, nativeErr("authority_mismatch", "project binding changed after opening the query reader")
	}
	page.SampledAt = time.Now().UTC()
	if limit < 1 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	query := `SELECT task_id,status,version,payload FROM tasks WHERE project_identity=?`
	args := []any{project}
	if taskID != "" {
		query += ` AND task_id=?`
		args = append(args, taskID)
	}
	// Canonical decimal IDs sort numerically by length then text, without casts
	// or loading and sorting the entire project in memory.
	query += ` ORDER BY length(task_id),task_id LIMIT ? OFFSET ?`
	args = append(args, limit+1, offset)
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return page, err
	}
	var all []taskmodel.TaskRecord
	for rows.Next() {
		var id string
		var status string
		var version uint64
		var payload []byte
		if err = rows.Scan(&id, &status, &version, &payload); err != nil {
			rows.Close()
			return page, err
		}
		if len(all) == limit {
			page.HasMore = true
			continue
		}
		var record taskmodel.TaskRecord
		if err = json.Unmarshal(payload, &record); err != nil {
			rows.Close()
			return page, err
		}
		if record.Source != "native" {
			rows.Close()
			return page, nativeErr("authority_mismatch", "native authority contains a non-native task record")
		}
		if record.ProjectIdentity != project || record.TaskID != id || !taskmodel.CanonicalDecimalID(record.TaskID) {
			rows.Close()
			return page, nativeErr("contract_mismatch", "native task ID is not canonical")
		}
		if record.Status != status || record.Version != version || !taskmodel.ValidStatus(record.Status) || record.Version == 0 || record.Type == "" || record.Priority < 0 || record.Priority > 4 {
			rows.Close()
			return page, nativeErr("contract_mismatch", "native task record has invalid status, type, priority or version")
		}
		if record.Status == "discarded" && (record.DiscardedAt == nil || strings.TrimSpace(record.DiscardReason) == "" || strings.TrimSpace(record.DiscardedBy) == "" || !requestIDPattern.MatchString(record.DiscardRequestID)) || record.Status != "discarded" && (record.DiscardedAt != nil || record.DiscardReason != "" || record.DiscardedBy != "" || record.DiscardRequestID != "" || len(record.CleanupPending) > 0) {
			rows.Close()
			return page, nativeErr("contract_mismatch", "native discard identity and cleanup fields do not match task status")
		}
		if record.Status == "open" && record.Claimant != "" || (record.Status == "in_progress" || record.Status == "closed") && record.Claimant == "" {
			rows.Close()
			return page, nativeErr("contract_mismatch", "native task claim does not match task status")
		}
		all = append(all, record)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return page, err
	}
	rows.Close()
	if offset > 0 && len(all) == 0 {
		return page, nativeErr("cursor_invalidated", "cursor offset is outside the current snapshot")
	}
	// Keep the whole relation snapshot, but decode only paged tasks and their
	// predecessors so an unrelated damaged task cannot break this page.
	edges := []taskmodel.TaskDependencyRecord{}
	rows, err = tx.QueryContext(ctx, `SELECT predecessor_id,successor_id FROM task_dependencies WHERE project_identity=? ORDER BY length(predecessor_id),predecessor_id,length(successor_id),successor_id`, project)
	if err != nil {
		return page, err
	}
	for rows.Next() {
		var edge taskmodel.TaskDependencyRecord
		edge.ProjectIdentity = project
		if err = rows.Scan(&edge.PredecessorID, &edge.SuccessorID); err != nil {
			rows.Close()
			return page, err
		}
		edges = append(edges, edge)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return page, err
	}
	rows.Close()
	relations, err := loadNativePageRelations(ctx, tx, project, targetBranch, all, edges)
	if err != nil {
		return page, err
	}
	incoming := map[string][]taskmodel.TaskDependencyRecord{}
	outgoing := map[string][]string{}
	for _, edge := range edges {
		incoming[edge.SuccessorID] = append(incoming[edge.SuccessorID], edge)
		outgoing[edge.PredecessorID] = append(outgoing[edge.PredecessorID], edge.SuccessorID)
	}
	var executionSlotBusy int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM executions WHERE project_identity=? AND status IN ('pending','active','waiting_decision','blocked')`, project).Scan(&executionSlotBusy); err != nil {
		return page, err
	}
	for _, record := range all {
		detail := contracts.NativeTaskDetail{TaskRecord: record, ProjectID: project, Priority: record.Priority, WorkflowRuns: []executionmodel.WorkflowRunRecord{}, Predecessors: []taskmodel.TaskDependencyStatus{}, Successors: []string{}, UnmetPredecessors: []taskmodel.TaskDependencyStatus{}, StartupBlockers: []string{}}
		detail.Successors = append(detail.Successors, outgoing[record.TaskID]...)
		predecessors := make([]taskmodel.TaskDependencyStatus, 0, len(incoming[record.TaskID]))
		for _, edge := range incoming[record.TaskID] {
			_, delivered := relations.deliveries[edge.PredecessorID]
			predecessors = append(predecessors, taskmodel.TaskDependencyStatus{TaskID: edge.PredecessorID, Status: relations.statuses[edge.PredecessorID], Delivered: delivered})
		}
		delivery, hasDelivery := relations.deliveries[record.TaskID]
		detail.DependencyReady, detail.Predecessors, detail.UnmetPredecessors = taskmodel.DependencyState(record.Status, hasDelivery, predecessors)
		detail.StartupBlockers = executionmodel.StartupBlockers(record.Status, hasDelivery, dispatchPaused != 0, dispatchMode, executionSlotBusy > 0)
		contractRow, ok := relations.contracts[record.TaskID]
		if !ok {
			return page, nativeErr("contract_mismatch", "native task has no matching fixed contract")
		}
		if err = json.Unmarshal(contractRow.payload, &detail.Contract); err != nil {
			return page, err
		}
		if detail.Contract.ContractID != record.ContractID || contractRow.id != record.ContractID || detail.Contract.Digest != contractRow.digest {
			return page, nativeErr("contract_mismatch", "task contract identity differs from task record")
		}
		if err = taskmodel.ValidateContract(detail.Contract); err != nil {
			return page, err
		}
		if detail.Contract.Repository != targetRepository || detail.Contract.Remote != targetRemote || detail.Contract.Branch != targetBranch {
			return page, nativeErr("contract_mismatch", "task contract target differs from persisted project authority")
		}
		detail.WorkflowRuns = append(detail.WorkflowRuns, relations.runs[record.TaskID]...)
		if hasDelivery {
			detail.Delivery = &delivery
		}
		page.Tasks = append(page.Tasks, detail)
	}
	return page, nil
}

type nativePageContract struct {
	id      string
	digest  string
	payload []byte
}

type nativePageRelations struct {
	statuses   map[string]string
	deliveries map[string]executionmodel.TaskDeliveryRecord
	contracts  map[string]nativePageContract
	runs       map[string][]executionmodel.WorkflowRunRecord
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

func loadNativePageRelations(ctx context.Context, tx *sql.Tx, project, targetBranch string, tasks []taskmodel.TaskRecord, edges []taskmodel.TaskDependencyRecord) (nativePageRelations, error) {
	out := nativePageRelations{statuses: map[string]string{}, deliveries: map[string]executionmodel.TaskDeliveryRecord{}, contracts: map[string]nativePageContract{}, runs: map[string][]executionmodel.WorkflowRunRecord{}}
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
			var task taskmodel.TaskRecord
			if err = json.Unmarshal(body, &task); err != nil {
				break
			}
			if task.ProjectIdentity != project || task.TaskID != id || task.Source != "native" || !taskmodel.CanonicalDecimalID(id) || task.Status != status || task.Version != version || !taskmodel.ValidStatus(status) || version == 0 {
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
			var delivery executionmodel.TaskDeliveryRecord
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
			if err = executionmodel.ValidateDelivery(delivery, targetBranch); err != nil {
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
			var run executionmodel.WorkflowRunRecord
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
