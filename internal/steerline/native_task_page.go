package steerline

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/task"
	"github.com/erranliu/SteerLine/internal/steerline/taskview"
)

const nativeTaskProtocol = "native-tasks/v1"

func canonicalDecimalID(value string) bool { return task.CanonicalDecimalID(value) }

type NativeTaskCapabilities struct {
	DependencyReadiness string `json:"dependency_readiness"`
	DependencyBlockers  string `json:"dependency_blockers"`
	StartupBlockers     string `json:"startup_blockers"`
}

type NativeTaskDetail struct {
	TaskRecord
	ProjectID         string                 `json:"project_id"`
	Priority          int                    `json:"priority"`
	Contract          TaskContract           `json:"contract"`
	WorkflowRuns      []WorkflowRunRecord    `json:"workflow_runs"`
	Delivery          *TaskDeliveryRecord    `json:"delivery,omitempty"`
	Predecessors      []TaskDependencyStatus `json:"predecessors"`
	Successors        []string               `json:"successors"`
	DependencyReady   bool                   `json:"dependency_ready"`
	UnmetPredecessors []TaskDependencyStatus `json:"unmet_predecessors"`
	StartupBlockers   []string               `json:"startup_blockers"`
}

type TaskDependencyStatus = task.TaskDependencyStatus

type NativeTaskPage struct {
	Protocol         string                 `json:"protocol"`
	ProjectID        string                 `json:"project_id"`
	AuthorityVersion uint64                 `json:"authority_version"`
	GraphRevision    uint64                 `json:"graph_revision"`
	DatabaseRevision uint64                 `json:"database_revision"`
	DatabaseID       string                 `json:"database_id"`
	SampledAt        time.Time              `json:"sampled_at"`
	Capabilities     NativeTaskCapabilities `json:"capabilities"`
	Tasks            []NativeTaskDetail     `json:"tasks"`
	NextCursor       string                 `json:"next_cursor,omitempty"`
	HasMore          bool                   `json:"has_more"`
}

func (p NativeTaskPage) MarshalJSON() ([]byte, error) {
	type plain NativeTaskPage
	body, err := json.Marshal(plain(p))
	if err != nil {
		return nil, err
	}
	return taskview.EscapeBidiJSON(body), nil
}

type nativeTaskCursor struct {
	Version          int    `json:"v"`
	ProjectID        string `json:"p"`
	AuthorityVersion uint64 `json:"a"`
	GraphRevision    uint64 `json:"g"`
	DatabaseRevision uint64 `json:"d"`
	DatabaseID       string `json:"i"`
	Offset           int    `json:"o"`
}

type NativeTaskError = task.Error

func nativeErr(code, message string) error { return &task.Error{Code: code, Message: message} }

func ValidateNativeTaskID(id string) error { return task.ValidateID(id) }

func (q QueryService) ListNativeTasks(ctx context.Context, cursor string, limit int) (NativeTaskPage, error) {
	if q.Store == nil || q.Project == "" {
		return NativeTaskPage{}, errors.New("query service store and project identity are required")
	}
	if limit < 1 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	offset := 0
	expectedDB := uint64(0)
	expectedDatabaseID := ""
	expectedAuthority, expectedGraph := uint64(0), uint64(0)
	if cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil {
			return NativeTaskPage{}, nativeErr("cursor_invalidated", "cursor encoding is invalid")
		}
		var c nativeTaskCursor
		if decode(raw, &c) != nil || c.Version != 1 || c.ProjectID != q.Project || c.Offset < 0 || c.DatabaseRevision == 0 || len(c.DatabaseID) != 32 {
			return NativeTaskPage{}, nativeErr("cursor_invalidated", "cursor identity or sampling revision is invalid")
		}
		offset, expectedDB = c.Offset, c.DatabaseRevision
		expectedDatabaseID = c.DatabaseID
		expectedAuthority, expectedGraph = c.AuthorityVersion, c.GraphRevision
	}
	page, err := q.Store.NativeTaskPage(ctx, q.Project, "", expectedDatabaseID, expectedDB, offset, limit)
	if err != nil {
		return NativeTaskPage{}, err
	}
	if expectedDB != 0 && (page.AuthorityVersion != expectedAuthority || page.GraphRevision != expectedGraph) {
		return NativeTaskPage{}, nativeErr("cursor_invalidated", "project authority changed between pages")
	}
	setNativeTaskPageCursor(&page, offset)
	return page, nil
}

func setNativeTaskPageCursor(page *NativeTaskPage, offset int) {
	if page.HasMore {
		c := nativeTaskCursor{Version: 1, ProjectID: page.ProjectID, AuthorityVersion: page.AuthorityVersion, GraphRevision: page.GraphRevision, DatabaseID: page.DatabaseID, DatabaseRevision: page.DatabaseRevision, Offset: offset + len(page.Tasks)}
		b, _ := json.Marshal(c)
		page.NextCursor = base64.RawURLEncoding.EncodeToString(b)
	}
}

func (q QueryService) ShowNativeTask(ctx context.Context, id string) (NativeTaskPage, error) {
	if q.Store == nil || q.Project == "" {
		return NativeTaskPage{}, errors.New("query service store and project identity are required")
	}
	if err := ValidateNativeTaskID(id); err != nil {
		return NativeTaskPage{}, err
	}
	page, err := q.Store.NativeTaskPage(ctx, q.Project, id, "", 0, 0, 1)
	if err != nil {
		return NativeTaskPage{}, err
	}
	if len(page.Tasks) == 0 {
		return NativeTaskPage{}, nativeErr("task_not_found", "native task was not found")
	}
	return page, nil
}

func (s *SQLiteStore) NativeTaskPage(ctx context.Context, project, taskID, expectedDatabaseID string, expectedDB uint64, offset, limit int) (NativeTaskPage, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return NativeTaskPage{}, err
	}
	defer tx.Rollback()
	page, err := s.nativeTaskPageTx(ctx, tx, project, taskID, expectedDatabaseID, expectedDB, offset, limit)
	if err != nil {
		return page, err
	}
	return page, tx.Commit()
}

func (s *SQLiteStore) nativeTaskPageTx(ctx context.Context, tx *sql.Tx, project, taskID, expectedDatabaseID string, expectedDB uint64, offset, limit int) (NativeTaskPage, error) {
	page := NativeTaskPage{Protocol: nativeTaskProtocol, ProjectID: project, Tasks: []NativeTaskDetail{}, Capabilities: NativeTaskCapabilities{DependencyReadiness: "provided", DependencyBlockers: "provided", StartupBlockers: "provided"}}
	if offset < 0 {
		return page, nativeErr("cursor_invalidated", "negative page offset")
	}
	if project == "" {
		return page, nativeErr("authority_mismatch", "project identity is required")
	}
	var err error
	var schemaVersion int
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&schemaVersion); err != nil || schemaVersion != domainSchemaVersion {
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
	var all []TaskRecord
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
		var record TaskRecord
		if err = json.Unmarshal(payload, &record); err != nil {
			rows.Close()
			return page, err
		}
		if record.Source != "native" {
			rows.Close()
			return page, nativeErr("authority_mismatch", "native authority contains a non-native task record")
		}
		if record.ProjectIdentity != project || record.TaskID != id || !canonicalDecimalID(record.TaskID) {
			rows.Close()
			return page, nativeErr("contract_mismatch", "native task ID is not canonical")
		}
		if record.Status != status || record.Version != version || !validTaskStatus(record.Status) || record.Version == 0 || record.Type == "" || record.Priority < 0 || record.Priority > 4 {
			rows.Close()
			return page, nativeErr("contract_mismatch", "native task record has invalid status, type, priority or version")
		}
		if record.Status == "discarded" && (record.DiscardedAt == nil || strings.TrimSpace(record.DiscardReason) == "" || strings.TrimSpace(record.DiscardedBy) == "" || !controlIDPattern.MatchString(record.DiscardRequestID)) || record.Status != "discarded" && (record.DiscardedAt != nil || record.DiscardReason != "" || record.DiscardedBy != "" || record.DiscardRequestID != "" || len(record.CleanupPending) > 0) {
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
	edges := []TaskDependencyRecord{}
	rows, err = tx.QueryContext(ctx, `SELECT predecessor_id,successor_id FROM task_dependencies WHERE project_identity=? ORDER BY length(predecessor_id),predecessor_id,length(successor_id),successor_id`, project)
	if err != nil {
		return page, err
	}
	for rows.Next() {
		var edge TaskDependencyRecord
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
	incoming := map[string][]TaskDependencyRecord{}
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
		detail := NativeTaskDetail{TaskRecord: record, ProjectID: project, Priority: record.Priority, WorkflowRuns: []WorkflowRunRecord{}, Predecessors: []TaskDependencyStatus{}, Successors: []string{}, UnmetPredecessors: []TaskDependencyStatus{}, StartupBlockers: []string{}}
		detail.Successors = append(detail.Successors, outgoing[record.TaskID]...)
		predecessors := make([]TaskDependencyStatus, 0, len(incoming[record.TaskID]))
		for _, edge := range incoming[record.TaskID] {
			_, delivered := relations.deliveries[edge.PredecessorID]
			predecessors = append(predecessors, TaskDependencyStatus{TaskID: edge.PredecessorID, Status: relations.statuses[edge.PredecessorID], Delivered: delivered})
		}
		delivery, hasDelivery := relations.deliveries[record.TaskID]
		detail.DependencyReady, detail.Predecessors, detail.UnmetPredecessors = nativeDependencyState(record.Status, hasDelivery, predecessors)
		detail.StartupBlockers = nativeStartupBlockersFor(record.Status, hasDelivery, dispatchPaused != 0, dispatchMode, executionSlotBusy > 0)
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
		if err = ValidateTaskContract(detail.Contract); err != nil {
			return page, err
		}
		if !sameRepositoryTarget(detail.Contract.Repository, targetRepository) || detail.Contract.Remote != targetRemote || detail.Contract.Branch != targetBranch {
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

// OpenNativeTaskProject initializes one native project authority in a clean
// state directory. Once initialized, each read verifies the persisted binding.
