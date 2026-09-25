package steerline

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"
)

const taskBatchSchemaVersion = "task-batch/v1"

// TaskBatchInput is a creation command. It is never stored as an editable task
// state; the normalized payload digest and resolved identity map are persisted.
type TaskBatchInput struct {
	SchemaVersion string          `json:"schema_version"`
	ProjectID     string          `json:"project_id"`
	Tasks         []TaskBatchTask `json:"tasks"`
	Dependencies  []TaskBatchEdge `json:"dependencies"`
}

type TaskBatchTask struct {
	LocalKey string       `json:"local_key"`
	TaskID   int64        `json:"task_id"`
	Title    string       `json:"title"`
	Type     string       `json:"type"`
	Priority *int         `json:"priority,omitempty"`
	Contract TaskContract `json:"contract"`
	GoalIDs  []string     `json:"goal_ids,omitempty"`
}

type TaskBatchReference struct {
	LocalKey string `json:"local_key,omitempty"`
	TaskID   *int64 `json:"task_id,omitempty"`
}

type TaskBatchEdge struct {
	Predecessor TaskBatchReference `json:"predecessor"`
	Successor   TaskBatchReference `json:"successor"`
}

type TaskBatchIssue struct {
	Code    string `json:"code"`
	Node    string `json:"node,omitempty"`
	Message string `json:"message"`
}

type TaskBatchValidation struct {
	Valid            bool             `json:"valid"`
	ProjectID        string           `json:"project_id"`
	TaskCount        int              `json:"task_count"`
	DependencyCount  int              `json:"dependency_count"`
	AuthorityVersion uint64           `json:"authority_version"`
	GraphRevision    uint64           `json:"graph_revision"`
	DatabaseRevision uint64           `json:"database_revision"`
	DatabaseID       string           `json:"database_id"`
	SampledAt        time.Time        `json:"sampled_at"`
	Cycle            []string         `json:"cycle,omitempty"`
	Issues           []TaskBatchIssue `json:"issues"`
}

type TaskBatchReceipt struct {
	RequestID     string            `json:"request_id"`
	BatchID       string            `json:"batch_id"`
	TaskIDs       map[string]string `json:"task_ids"`
	GraphRevision uint64            `json:"graph_revision"`
	PayloadDigest string            `json:"payload_digest"`
	Status        string            `json:"status"`
	DispatchNote  string            `json:"dispatch_note"`
}

type TaskDiscardReceipt struct {
	RequestID      string   `json:"request_id"`
	TaskID         string   `json:"task_id"`
	Status         string   `json:"status"`
	Code           string   `json:"code,omitempty"`
	Message        string   `json:"message,omitempty"`
	GraphRevision  uint64   `json:"graph_revision"`
	CleanupPending []string `json:"cleanup_pending,omitempty"`
}

type taskDiscardIntent struct {
	TaskID          string   `json:"task_id"`
	ExpectedVersion uint64   `json:"expected_version"`
	Reason          string   `json:"reason"`
	RequestedBy     string   `json:"requested_by"`
	PRURL           string   `json:"pr_url,omitempty"`
	UncertainPR     bool     `json:"uncertain_pr,omitempty"`
	CleanupPending  []string `json:"cleanup_pending,omitempty"`
}

type TaskGraphNode struct {
	TaskID          string                 `json:"task_id"`
	Status          string                 `json:"status"`
	Priority        int                    `json:"priority"`
	DependencyReady bool                   `json:"dependency_ready"`
	Blockers        []TaskDependencyStatus `json:"blockers"`
	StartupBlockers []string               `json:"startup_blockers"`
	Delivery        *TaskDeliveryRecord    `json:"delivery,omitempty"`
}

type TaskGraphEdge struct {
	Predecessor string `json:"predecessor"`
	Successor   string `json:"successor"`
}

type TaskGraphPage struct {
	Protocol         string          `json:"protocol"`
	ProjectID        string          `json:"project_id"`
	AuthorityVersion uint64          `json:"authority_version"`
	GraphRevision    uint64          `json:"graph_revision"`
	DatabaseRevision uint64          `json:"database_revision"`
	DatabaseID       string          `json:"database_id"`
	SampledAt        time.Time       `json:"sampled_at"`
	Nodes            []TaskGraphNode `json:"nodes"`
	Edges            []TaskGraphEdge `json:"edges"`
	NextCursor       string          `json:"next_cursor,omitempty"`
	HasMore          bool            `json:"has_more"`
}

type taskGraphCursor struct {
	Version          int    `json:"v"`
	ProjectID        string `json:"p"`
	AuthorityVersion uint64 `json:"a"`
	GraphRevision    uint64 `json:"g"`
	DatabaseRevision uint64 `json:"d"`
	DatabaseID       string `json:"i"`
	Offset           int    `json:"o"`
}

type taskGraphSnapshot struct {
	Authority         ProjectTaskAuthority
	DatabaseID        string
	DatabaseRevision  uint64
	SampledAt         time.Time
	Tasks             []TaskRecord
	ByID              map[string]TaskRecord
	RepositoryTaskIDs map[string]bool
	ContractIDs       map[string]bool
	Edges             []TaskDependencyRecord
	Deliveries        map[string]TaskDeliveryRecord
	ExecutionSlotBusy bool
}

func decodeTaskBatch(data []byte) (TaskBatchInput, []byte, error) {
	canonical, err := CanonicalJSON(data)
	if err != nil {
		return TaskBatchInput{}, nil, nativeErr("invalid_batch", err.Error())
	}
	var batch TaskBatchInput
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err = d.Decode(&batch); err != nil {
		return batch, nil, nativeErr("invalid_batch", err.Error())
	}
	var extra any
	if err = d.Decode(&extra); err != io.EOF {
		return batch, nil, nativeErr("invalid_batch", "expected exactly one JSON object")
	}
	var shape map[string]json.RawMessage
	if err = json.Unmarshal(data, &shape); err != nil {
		return batch, nil, nativeErr("invalid_batch", err.Error())
	}
	if shape["schema_version"] == nil || shape["project_id"] == nil || shape["tasks"] == nil {
		return batch, nil, nativeErr("invalid_batch", "schema_version, project_id and tasks are required")
	}
	if raw := shape["dependencies"]; raw != nil && (bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || len(bytes.TrimSpace(raw)) == 0 || bytes.TrimSpace(raw)[0] != '[') {
		return batch, nil, nativeErr("invalid_batch", "dependencies must be an array when provided")
	}
	if batch.SchemaVersion != taskBatchSchemaVersion {
		return batch, nil, nativeErr("invalid_batch", "schema_version must be task-batch/v1")
	}
	if strings.TrimSpace(batch.ProjectID) == "" || len(batch.Tasks) == 0 {
		return batch, nil, nativeErr("invalid_batch", "project_id and a nonempty tasks array are required")
	}
	var taskShapes []map[string]json.RawMessage
	if err = json.Unmarshal(shape["tasks"], &taskShapes); err != nil || len(taskShapes) != len(batch.Tasks) {
		return batch, nil, nativeErr("invalid_batch", "tasks must be an array of objects")
	}
	for i, taskShape := range taskShapes {
		for _, name := range []string{"local_key", "task_id", "title", "type", "contract"} {
			if taskShape[name] == nil || bytes.Equal(bytes.TrimSpace(taskShape[name]), []byte("null")) {
				return batch, nil, nativeErr("invalid_batch", fmt.Sprintf("tasks[%d].%s is required", i, name))
			}
		}
		if raw := taskShape["priority"]; raw != nil && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return batch, nil, nativeErr("invalid_batch", fmt.Sprintf("tasks[%d].priority must be an integer when provided", i))
		}
		if strings.TrimSpace(batch.Tasks[i].LocalKey) == "" {
			return batch, nil, nativeErr("invalid_batch", fmt.Sprintf("tasks[%d].local_key is empty", i))
		}
		if batch.Tasks[i].TaskID < 0 {
			return batch, nil, nativeErr("invalid_batch", fmt.Sprintf("tasks[%d].task_id must be a nonnegative integer", i))
		}
		if strings.TrimSpace(batch.Tasks[i].Title) == "" {
			return batch, nil, nativeErr("invalid_batch", fmt.Sprintf("tasks[%d].title is empty", i))
		}
	}
	return batch, canonical, nil
}

func (s *SQLiteStore) taskGraphSnapshot(ctx context.Context, project string) (taskGraphSnapshot, error) {
	out := taskGraphSnapshot{ByID: map[string]TaskRecord{}, RepositoryTaskIDs: map[string]bool{}, ContractIDs: map[string]bool{}, Deliveries: map[string]TaskDeliveryRecord{}, SampledAt: time.Now().UTC()}
	if project == "" {
		return out, nativeErr("authority_mismatch", "project identity is required")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	var paused int
	var created, updated string
	err = tx.QueryRowContext(ctx, `SELECT authority,authority_version,graph_revision,repository,remote,branch,dispatch_mode,dispatch_paused,created_at,updated_at FROM project_task_authorities WHERE project_identity=?`, project).Scan(&out.Authority.Authority, &out.Authority.AuthorityVersion, &out.Authority.GraphRevision, &out.Authority.Repository, &out.Authority.Remote, &out.Authority.Branch, &out.Authority.DispatchMode, &paused, &created, &updated)
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
	rows, err = tx.QueryContext(ctx, `SELECT contract_id FROM task_contracts WHERE project_identity=?`, project)
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
		out.Deliveries[d.TaskID] = d
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return out, err
	}
	rows.Close()
	if err = tx.Commit(); err != nil {
		return out, err
	}
	return out, nil
}

func (c CommandService) ValidateTaskBatch(ctx context.Context, config Config, data []byte) (TaskBatchValidation, error) {
	batch, _, err := decodeTaskBatch(data)
	if err != nil {
		return TaskBatchValidation{}, err
	}
	if c.Store == nil || c.Project == "" || config.Identity() != c.Project || batch.ProjectID != c.Project {
		return TaskBatchValidation{}, nativeErr("authority_mismatch", "batch project_id, command target and configured project must match")
	}
	snapshot, err := taskGraphFromStore(ctx, c.Store, c.Project)
	if err != nil {
		return TaskBatchValidation{}, err
	}
	result := TaskBatchValidation{ProjectID: c.Project, TaskCount: len(batch.Tasks), DependencyCount: len(batch.Dependencies), AuthorityVersion: snapshot.Authority.AuthorityVersion, GraphRevision: snapshot.Authority.GraphRevision, DatabaseRevision: snapshot.DatabaseRevision, DatabaseID: snapshot.DatabaseID, SampledAt: snapshot.SampledAt, Issues: []TaskBatchIssue{}}
	issues, cycle := validateTaskBatchAgainstGraph(ctx, config, batch, snapshot, true)
	result.Issues, result.Cycle = issues, cycle
	result.Valid = len(issues) == 0
	return result, nil
}

func taskGraphFromStore(ctx context.Context, store DomainStore, project string) (taskGraphSnapshot, error) {
	if sqlite, ok := store.(*SQLiteStore); ok {
		return sqlite.taskGraphSnapshot(ctx, project)
	}
	return taskGraphSnapshot{}, errors.New("native DAG commands require the center SQLite store")
}

func (q QueryService) TaskGraph(ctx context.Context, cursor string, limit int) (TaskGraphPage, error) {
	if q.Store == nil || q.Project == "" {
		return TaskGraphPage{}, errors.New("query service store and project identity are required")
	}
	if limit < 1 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	offset := 0
	var expected taskGraphCursor
	if cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil {
			return TaskGraphPage{}, nativeErr("cursor_invalidated", "cursor encoding is invalid")
		}
		if err = decode(raw, &expected); err != nil || expected.Version != 1 || expected.ProjectID != q.Project || expected.Offset < 0 || expected.DatabaseRevision == 0 || len(expected.DatabaseID) != 32 {
			return TaskGraphPage{}, nativeErr("cursor_invalidated", "cursor identity or sampling revision is invalid")
		}
		offset = expected.Offset
	}
	snapshot, err := taskGraphFromStore(ctx, q.Store, q.Project)
	if err != nil {
		return TaskGraphPage{}, err
	}
	if cursor != "" && (expected.AuthorityVersion != snapshot.Authority.AuthorityVersion || expected.GraphRevision != snapshot.Authority.GraphRevision || expected.DatabaseRevision != snapshot.DatabaseRevision || expected.DatabaseID != snapshot.DatabaseID) {
		return TaskGraphPage{}, nativeErr("cursor_invalidated", "database or project graph changed between pages")
	}
	page := TaskGraphPage{Protocol: nativeTaskProtocol, ProjectID: q.Project, AuthorityVersion: snapshot.Authority.AuthorityVersion, GraphRevision: snapshot.Authority.GraphRevision, DatabaseRevision: snapshot.DatabaseRevision, DatabaseID: snapshot.DatabaseID, SampledAt: snapshot.SampledAt, Nodes: []TaskGraphNode{}, Edges: []TaskGraphEdge{}}
	if offset > len(snapshot.Tasks) || offset == len(snapshot.Tasks) && offset > 0 {
		return page, nativeErr("cursor_invalidated", "cursor offset is outside the current snapshot")
	}
	end := offset + limit
	if end > len(snapshot.Tasks) {
		end = len(snapshot.Tasks)
	}
	page.HasMore = end < len(snapshot.Tasks)
	selected := map[string]bool{}
	for _, task := range snapshot.Tasks[offset:end] {
		selected[task.TaskID] = true
		ready, blockers := taskDependencyState(task, snapshot)
		node := TaskGraphNode{TaskID: task.TaskID, Status: task.Status, Priority: task.Priority, DependencyReady: ready, Blockers: blockers, StartupBlockers: nativeStartupBlockers(task, snapshot)}
		if delivery, ok := snapshot.Deliveries[task.TaskID]; ok {
			copy := delivery
			node.Delivery = &copy
		}
		page.Nodes = append(page.Nodes, node)
	}
	for _, edge := range snapshot.Edges {
		if selected[edge.SuccessorID] {
			page.Edges = append(page.Edges, TaskGraphEdge{Predecessor: edge.PredecessorID, Successor: edge.SuccessorID})
		}
	}
	if page.HasMore {
		c := taskGraphCursor{Version: 1, ProjectID: q.Project, AuthorityVersion: page.AuthorityVersion, GraphRevision: page.GraphRevision, DatabaseRevision: page.DatabaseRevision, DatabaseID: page.DatabaseID, Offset: end}
		raw, _ := json.Marshal(c)
		page.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
	}
	return page, nil
}

func nativeStartupBlockers(task TaskRecord, graph taskGraphSnapshot) []string {
	blockers := []string{}
	if nativeDeliveryInconsistent(task, graph) {
		blockers = append(blockers, "inconsistent_delivery")
	}
	if task.Status != "open" {
		blockers = append(blockers, "task_not_open")
	}
	if graph.Authority.DispatchPaused {
		blockers = append(blockers, "dispatch_paused")
	}
	if graph.ExecutionSlotBusy {
		blockers = append(blockers, "execution_slot_busy")
	}
	if task.Status == "open" {
		blockers = append(blockers, "native_execution_unavailable")
	}
	return blockers
}

func nativeDeliveryInconsistent(task TaskRecord, graph taskGraphSnapshot) bool {
	_, delivered := graph.Deliveries[task.TaskID]
	return (task.Status == "closed") != delivered
}

func taskDependencyState(task TaskRecord, graph taskGraphSnapshot) (bool, []TaskDependencyStatus) {
	ready := task.Status == "open" && !nativeDeliveryInconsistent(task, graph)
	blockers := []TaskDependencyStatus{}
	for _, edge := range graph.Edges {
		if edge.SuccessorID != task.TaskID {
			continue
		}
		pred, ok := graph.ByID[edge.PredecessorID]
		if !ok {
			blockers = append(blockers, TaskDependencyStatus{TaskID: edge.PredecessorID, Blocker: "inconsistent_delivery"})
			ready = false
			continue
		}
		delivered := false
		if _, ok = graph.Deliveries[edge.PredecessorID]; ok {
			delivered = true
		}
		state := TaskDependencyStatus{TaskID: pred.TaskID, Status: pred.Status, Delivered: delivered}
		if pred.Status == "discarded" {
			state.Blocker = "discarded_predecessor"
		} else if pred.Status == "closed" && !delivered || delivered && pred.Status != "closed" {
			state.Blocker = "inconsistent_delivery"
		} else if !delivered {
			state.Blocker = "predecessor_not_delivered"
		}
		if state.Blocker != "" {
			blockers = append(blockers, state)
			ready = false
		}
	}
	return ready, blockers
}

// ReadyNativeTasks returns a read-only candidate snapshot in dispatch order.
// Callers must still claim inside a transaction; this snapshot is not a permit.
func (q QueryService) ReadyNativeTasks(ctx context.Context) ([]TaskRecord, error) {
	graph, err := taskGraphFromStore(ctx, q.Store, q.Project)
	if err != nil {
		return nil, err
	}
	ready := []TaskRecord{}
	for _, task := range graph.Tasks {
		if task.Status != "open" {
			continue
		}
		if allowed, _ := taskDependencyState(task, graph); allowed {
			ready = append(ready, task)
		}
	}
	sort.Slice(ready, func(i, j int) bool {
		if ready[i].Priority != ready[j].Priority {
			return ready[i].Priority < ready[j].Priority
		}
		if !ready[i].CreatedAt.Equal(ready[j].CreatedAt) {
			return ready[i].CreatedAt.Before(ready[j].CreatedAt)
		}
		return decimalIDLess(ready[i].TaskID, ready[j].TaskID)
	})
	return ready, nil
}

func validateTaskBatchAgainstGraph(ctx context.Context, config Config, batch TaskBatchInput, graph taskGraphSnapshot, verifyFiles bool) ([]TaskBatchIssue, []string) {
	issues := []TaskBatchIssue{}
	issue := func(code, node, message string) {
		issues = append(issues, TaskBatchIssue{Code: code, Node: node, Message: message})
	}
	if batch.ProjectID != graph.Authority.ProjectIdentity {
		issue("authority_mismatch", "project_id", "batch project does not match the sampled native project")
	}
	allowedTypes := map[string]bool{"task": true, "bug": true, "feature": true, "chore": true}
	configured := map[string]bool{}
	types := config.ExecutableTypes
	if len(types) == 0 {
		types = []string{"task", "bug", "feature", "chore"}
	}
	for _, typ := range types {
		configured[typ] = true
	}
	localIDs := map[string]string{}
	batchIDs := map[string]bool{}
	contractIDs := map[string]bool{}
	for _, task := range batch.Tasks {
		if _, ok := localIDs[task.LocalKey]; ok {
			issue("invalid_batch", task.LocalKey, "local_key is duplicated")
		} else {
			localIDs[task.LocalKey] = strconv.FormatInt(task.TaskID, 10)
		}
		id := strconv.FormatInt(task.TaskID, 10)
		if batchIDs[id] {
			issue("invalid_batch", task.LocalKey, "task_id is duplicated in the batch")
		}
		batchIDs[id] = true
		if task.TaskID < 0 {
			issue("invalid_batch", task.LocalKey, "task_id must be a nonnegative integer")
		}
		if graph.RepositoryTaskIDs[id] {
			issue("invalid_batch", task.LocalKey, "task_id is already used in the same repository identity")
		}
		if task.Contract.ContractID == "" || graph.ContractIDs[task.Contract.ContractID] || contractIDs[task.Contract.ContractID] {
			issue("invalid_batch", task.LocalKey, "contract_id must be unique in this repository identity")
		}
		contractIDs[task.Contract.ContractID] = true
		if strings.TrimSpace(task.Title) == "" {
			issue("invalid_batch", task.LocalKey, "title must not be empty")
		}
		if !allowedTypes[task.Type] || !configured[task.Type] {
			issue("invalid_batch", task.LocalKey, "type must be task, bug, feature or chore and enabled by the project")
		}
		priority := 2
		if task.Priority != nil {
			priority = *task.Priority
		}
		if priority < 0 || priority > 4 {
			issue("invalid_batch", task.LocalKey, "priority must be an integer from 0 to 4")
		}
		if verifyFiles {
			if err := VerifyTaskContractFiles(ctx, config, task.Contract); err != nil {
				issue("contract_mismatch", task.LocalKey, err.Error())
			}
		} else if err := ValidateTaskContract(task.Contract); err != nil {
			issue("contract_mismatch", task.LocalKey, err.Error())
		}
		if task.Contract.SpecRef.Commit != task.Contract.PlanRef.Commit {
			issue("contract_mismatch", task.LocalKey, "Spec and Plan must be pinned to the same committed Git object")
		}
		for _, goal := range task.GoalIDs {
			if strings.TrimSpace(goal) == "" {
				issue("invalid_batch", task.LocalKey, "goal_ids cannot contain an empty value")
			}
		}
	}
	if len(batch.Tasks) == 0 {
		issue("invalid_batch", "tasks", "tasks must be nonempty")
	}
	edges := append([]TaskDependencyRecord(nil), graph.Edges...)
	seen := map[string]bool{}
	for _, e := range graph.Edges {
		seen[e.PredecessorID+"\x00"+e.SuccessorID] = true
	}
	for i, edge := range batch.Dependencies {
		if edge.Successor.TaskID != nil || strings.TrimSpace(edge.Successor.LocalKey) == "" {
			issue("invalid_batch", fmt.Sprintf("dependencies[%d].successor", i), "successor must contain only a batch local_key")
			continue
		}
		successor, ok := localIDs[edge.Successor.LocalKey]
		if !ok {
			issue("invalid_batch", fmt.Sprintf("dependencies[%d].successor", i), "successor local_key is not in this batch")
			continue
		}
		pred := ""
		if strings.TrimSpace(edge.Predecessor.LocalKey) != "" && edge.Predecessor.TaskID == nil {
			pred, ok = localIDs[edge.Predecessor.LocalKey]
			if !ok {
				issue("missing_predecessor", fmt.Sprintf("dependencies[%d].predecessor", i), "predecessor local_key is not in this batch")
				continue
			}
		} else if strings.TrimSpace(edge.Predecessor.LocalKey) == "" && edge.Predecessor.TaskID != nil {
			if *edge.Predecessor.TaskID < 0 {
				issue("invalid_batch", fmt.Sprintf("dependencies[%d].predecessor", i), "predecessor task_id must be nonnegative")
				continue
			}
			pred = strconv.FormatInt(*edge.Predecessor.TaskID, 10)
			if task, exists := graph.ByID[pred]; !exists {
				if graph.RepositoryTaskIDs[pred] {
					issue("cross_project_dependency", fmt.Sprintf("dependencies[%d].predecessor", i), "dependencies cannot cross native project boundaries")
				} else {
					issue("missing_predecessor", fmt.Sprintf("dependencies[%d].predecessor", i), "predecessor is not present in the project graph")
				}
			} else if task.Status == "discarded" {
				issue("discarded_predecessor", fmt.Sprintf("dependencies[%d].predecessor", i), "discarded task cannot be a new predecessor")
			}
		} else {
			issue("invalid_batch", fmt.Sprintf("dependencies[%d].predecessor", i), "predecessor must contain exactly one local_key or task_id")
			continue
		}
		if pred == successor {
			issue("dependency_cycle", fmt.Sprintf("dependencies[%d]", i), "dependency cannot reference the same task")
			continue
		}
		key := pred + "\x00" + successor
		if seen[key] {
			issue("duplicate_dependency", fmt.Sprintf("dependencies[%d]", i), "dependency edge is duplicated")
			continue
		}
		seen[key] = true
		edges = append(edges, TaskDependencyRecord{ProjectIdentity: batch.ProjectID, PredecessorID: pred, SuccessorID: successor})
	}
	cycle := dependencyCycle(edges)
	if len(cycle) > 0 {
		issue("dependency_cycle", "dependencies", "batch dependencies would create a cycle")
	}
	return issues, cycle
}

type taskBatchReplay struct{ Receipt TaskBatchReceipt }

func (e taskBatchReplay) Error() string { return "task batch command already applied" }

// CreateTaskBatch verifies the fixed Git references before entering SQLite,
// then persists the batch, tasks, contracts, edges, event and command receipt
// in one center transaction.
func (c CommandService) CreateTaskBatch(ctx context.Context, config Config, data []byte, requestID string, expectedAuthorityVersion uint64, requestedBy string) (TaskBatchReceipt, error) {
	batch, canonicalPayload, err := decodeTaskBatch(data)
	if err != nil {
		return TaskBatchReceipt{}, err
	}
	sqlite, ok := c.Store.(*SQLiteStore)
	if !ok || c.Project == "" || config.Identity() != c.Project || batch.ProjectID != c.Project {
		return TaskBatchReceipt{}, nativeErr("authority_mismatch", "batch project_id, command target and configured project must match")
	}
	if !controlIDPattern.MatchString(requestID) || expectedAuthorityVersion == 0 {
		return TaskBatchReceipt{}, nativeErr("invalid_batch", "create requires a valid request_id and expected authority version")
	}
	if strings.TrimSpace(requestedBy) == "" {
		requestedBy = DefaultRequestedBy()
	}
	if existing, found, e := lookupTaskBatchCommand(ctx, sqlite, c.Project, requestID); e != nil {
		return TaskBatchReceipt{}, e
	} else if found {
		return replayTaskBatch(existing, canonicalPayload, batch.ProjectID, requestedBy)
	}
	validation, err := c.ValidateTaskBatch(ctx, config, data)
	if err != nil {
		return TaskBatchReceipt{}, err
	}
	if !validation.Valid {
		return TaskBatchReceipt{}, taskBatchValidationError(validation.Issues, validation.Cycle)
	}
	if validation.AuthorityVersion != expectedAuthorityVersion {
		return TaskBatchReceipt{}, nativeErr("stale_version", fmt.Sprintf("expected authority version %d, sampled %d", expectedAuthorityVersion, validation.AuthorityVersion))
	}
	payloadDigest := digestCanonical(canonicalPayload)
	var receipt TaskBatchReceipt
	err = c.Store.Transact(ctx, func(domainTx DomainTx) error {
		tx, ok := domainTx.(*sqliteTx)
		if !ok {
			return errors.New("task batch command requires the center SQLite transaction")
		}
		if old, found, e := lookupTaskBatchCommandTx(ctx, tx, c.Project, requestID); e != nil {
			return e
		} else if found {
			r, e := replayTaskBatch(old, canonicalPayload, batch.ProjectID, requestedBy)
			if e != nil {
				return e
			}
			receipt = r
			return taskBatchReplay{Receipt: r}
		}
		graph, e := taskGraphSnapshotTx(ctx, tx, c.Project)
		if e != nil {
			return e
		}
		if graph.Authority.AuthorityVersion != expectedAuthorityVersion {
			return nativeErr("stale_version", fmt.Sprintf("expected authority version %d, found %d", expectedAuthorityVersion, graph.Authority.AuthorityVersion))
		}
		issues, cycle := validateTaskBatchAgainstGraph(ctx, config, batch, graph, false)
		if len(issues) > 0 {
			return taskBatchValidationError(issues, cycle)
		}
		command := CommandRecord{ProjectIdentity: c.Project, CommandID: requestID, Kind: "create_task_batch", TargetID: c.Project, ExpectedVersion: expectedAuthorityVersion, IdempotencyKey: requestID, RequestedBy: requestedBy, Payload: append([]byte(nil), canonicalPayload...), Status: "in_flight", CreatedAt: time.Now().UTC()}
		createdCommand, created, e := tx.EnqueueCommand(command)
		if e != nil {
			return nativeErr("idempotency_conflict", e.Error())
		}
		if !created {
			return nativeErr("idempotency_conflict", "request_id already belongs to another command")
		}
		command = createdCommand
		mapping := make(map[string]string, len(batch.Tasks))
		for _, task := range batch.Tasks {
			mapping[task.LocalKey] = strconv.FormatInt(task.TaskID, 10)
		}
		batchID := newID()
		createdAt := time.Now().UTC()
		batchRecord := TaskBatchRecord{BatchID: batchID, ProjectIdentity: c.Project, SchemaVersion: taskBatchSchemaVersion, PayloadDigest: payloadDigest, CreatedAt: createdAt, CommandID: requestID, TaskIDs: mapping}
		if e = tx.PutTaskBatch(batchRecord, expectedAuthorityVersion, graph.Authority.GraphRevision); e != nil {
			return e
		}
		for _, input := range batch.Tasks {
			id := strconv.FormatInt(input.TaskID, 10)
			priority := 2
			if input.Priority != nil {
				priority = *input.Priority
			}
			task := TaskRecord{ProjectIdentity: c.Project, TaskID: id, BatchID: batchID, Title: input.Title, Type: input.Type, Priority: priority, GoalIDs: append([]string(nil), input.GoalIDs...), ContractID: input.Contract.ContractID, Source: "native", Status: "open", Version: 1, CreatedAt: createdAt, UpdatedAt: createdAt}
			if e = tx.PutTask(task); e != nil {
				return e
			}
			if e = tx.PutTaskContract(c.Project, id, input.Contract); e != nil {
				return e
			}
			body, _ := json.Marshal(map[string]any{"batch_id": batchID, "local_key": input.LocalKey, "task_id": id, "contract_digest": input.Contract.Digest, "payload_digest": payloadDigest})
			if _, e = tx.AppendEvent(DomainEvent{ProjectIdentity: c.Project, TaskID: id, ExternalSource: "native-task-command", ExternalID: requestID + ":" + input.LocalKey, Type: "task.created", ReceivedAt: createdAt, Payload: body}); e != nil {
				return e
			}
		}
		for _, edge := range resolveTaskBatchEdges(batch) {
			if e = tx.PutTaskDependency(edge); e != nil {
				if strings.Contains(strings.ToLower(e.Error()), "unique") {
					return nativeErr("duplicate_dependency", "dependency edge is already present")
				}
				return e
			}
		}
		graphRevision := graph.Authority.GraphRevision + 1
		body, _ := json.Marshal(map[string]any{"batch_id": batchID, "task_ids": mapping, "graph_revision": graphRevision, "payload_digest": payloadDigest})
		if _, e = tx.AppendEvent(DomainEvent{ProjectIdentity: c.Project, ExternalSource: "native-task-command", ExternalID: requestID, Type: "task.batch.created", ReceivedAt: createdAt, Payload: body}); e != nil {
			return e
		}
		receipt = TaskBatchReceipt{RequestID: requestID, BatchID: batchID, TaskIDs: mapping, GraphRevision: graphRevision, PayloadDigest: payloadDigest, Status: "accepted", DispatchNote: "Task creation does not start work. If dispatch is already auto and unpaused, normal polling may consider these tasks."}
		command.Result, _ = json.Marshal(receipt)
		command.Status = "accepted"
		if e = tx.UpdateCommand(command); e != nil {
			return e
		}
		return nil
	})
	var replay taskBatchReplay
	if errors.As(err, &replay) {
		return replay.Receipt, nil
	}
	if err != nil {
		return TaskBatchReceipt{}, err
	}
	return receipt, nil
}

func resolveTaskBatchEdges(batch TaskBatchInput) []TaskDependencyRecord {
	ids := map[string]string{}
	for _, task := range batch.Tasks {
		ids[task.LocalKey] = strconv.FormatInt(task.TaskID, 10)
	}
	out := make([]TaskDependencyRecord, 0, len(batch.Dependencies))
	for _, edge := range batch.Dependencies {
		pred := ""
		if edge.Predecessor.TaskID != nil {
			pred = strconv.FormatInt(*edge.Predecessor.TaskID, 10)
		} else {
			pred = ids[edge.Predecessor.LocalKey]
		}
		out = append(out, TaskDependencyRecord{ProjectIdentity: batch.ProjectID, PredecessorID: pred, SuccessorID: ids[edge.Successor.LocalKey]})
	}
	return out
}

func formatTaskBatchIssues(issues []TaskBatchIssue, cycle []string) string {
	parts := make([]string, 0, len(issues)+1)
	for _, issue := range issues {
		if issue.Node != "" {
			parts = append(parts, issue.Code+" at "+issue.Node+": "+issue.Message)
		} else {
			parts = append(parts, issue.Code+": "+issue.Message)
		}
	}
	if len(cycle) > 0 {
		parts = append(parts, "dependency_cycle path="+strings.Join(cycle, " -> "))
	}
	return strings.Join(parts, "; ")
}

func taskBatchValidationError(issues []TaskBatchIssue, cycle []string) error {
	code := "invalid_batch"
	if len(issues) > 0 && issues[0].Code != "" {
		code = issues[0].Code
	}
	if len(cycle) > 0 {
		code = "dependency_cycle"
	}
	return nativeErr(code, formatTaskBatchIssues(issues, cycle))
}

func lookupTaskBatchCommand(ctx context.Context, s *SQLiteStore, project, id string) (CommandRecord, bool, error) {
	var out CommandRecord
	var created, updated string
	err := s.db.QueryRowContext(ctx, `SELECT project_identity,command_id,kind,target_id,expected_version,idempotency_key,requested_by,payload,status,result,created_at,updated_at FROM commands WHERE project_identity=? AND idempotency_key=?`, project, id).Scan(&out.ProjectIdentity, &out.CommandID, &out.Kind, &out.TargetID, &out.ExpectedVersion, &out.IdempotencyKey, &out.RequestedBy, &out.Payload, &out.Status, &out.Result, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return out, false, nil
	}
	if err != nil {
		return out, false, err
	}
	out.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	out.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return out, true, nil
}

func lookupTaskBatchCommandTx(ctx context.Context, t *sqliteTx, project, id string) (CommandRecord, bool, error) {
	var out CommandRecord
	var created, updated string
	err := t.tx.QueryRowContext(ctx, `SELECT project_identity,command_id,kind,target_id,expected_version,idempotency_key,requested_by,payload,status,result,created_at,updated_at FROM commands WHERE project_identity=? AND idempotency_key=?`, project, id).Scan(&out.ProjectIdentity, &out.CommandID, &out.Kind, &out.TargetID, &out.ExpectedVersion, &out.IdempotencyKey, &out.RequestedBy, &out.Payload, &out.Status, &out.Result, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return out, false, nil
	}
	if err != nil {
		return out, false, err
	}
	out.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	out.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return out, true, nil
}

func replayTaskBatch(old CommandRecord, payload []byte, project, requestedBy string) (TaskBatchReceipt, error) {
	if old.Kind != "create_task_batch" || old.TargetID != project || old.RequestedBy != requestedBy || !bytes.Equal(old.Payload, payload) {
		return TaskBatchReceipt{}, nativeErr("idempotency_conflict", "request_id was already used with different command content")
	}
	if old.Status != "accepted" || len(old.Result) == 0 {
		return TaskBatchReceipt{}, nativeErr("idempotency_conflict", "request_id has no committed task batch receipt")
	}
	var receipt TaskBatchReceipt
	if err := json.Unmarshal(old.Result, &receipt); err != nil {
		return receipt, nativeErr("unsupported_schema", "saved task batch receipt is unreadable")
	}
	if receipt.RequestID != old.CommandID || receipt.Status != "accepted" {
		return TaskBatchReceipt{}, nativeErr("unsupported_schema", "saved task batch receipt identity is inconsistent")
	}
	return receipt, nil
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
		out.Deliveries[delivery.TaskID] = delivery
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return out, err
	}
	rows.Close()
	return out, nil
}

type taskDiscardPlan struct {
	PRURL          string
	UncertainPR    bool
	CleanupPending []string
}

func (c CommandService) DiscardNativeTask(ctx context.Context, config Config, taskID, requestID string, expectedVersion uint64, reason, requestedBy string) (TaskDiscardReceipt, error) {
	if _, ok := c.Store.(*SQLiteStore); !ok || c.Project == "" || config.Identity() != c.Project {
		return TaskDiscardReceipt{}, nativeErr("authority_mismatch", "discard requires the configured native project")
	}
	if err := ValidateNativeTaskID(taskID); err != nil {
		return TaskDiscardReceipt{}, err
	}
	if !controlIDPattern.MatchString(requestID) || expectedVersion == 0 || strings.TrimSpace(reason) == "" {
		return TaskDiscardReceipt{}, nativeErr("immutable_task", "discard requires request identity, expected version and reason")
	}
	if strings.TrimSpace(requestedBy) == "" {
		requestedBy = DefaultRequestedBy()
	}
	reason = strings.TrimSpace(reason)
	if len(reason) > 4000 {
		return TaskDiscardReceipt{}, nativeErr("immutable_task", "discard reason exceeds 4000 bytes")
	}
	sqlite := c.Store.(*SQLiteStore)
	request := struct {
		TaskID          string `json:"task_id"`
		ExpectedVersion uint64 `json:"expected_version"`
		Reason          string `json:"reason"`
		RequestedBy     string `json:"requested_by"`
	}{taskID, expectedVersion, reason, requestedBy}
	requestJSON, _ := json.Marshal(request)
	canonicalRequest, err := CanonicalJSON(requestJSON)
	if err != nil {
		return TaskDiscardReceipt{}, err
	}
	if old, found, e := lookupTaskBatchCommand(ctx, sqlite, c.Project, requestID); e != nil {
		return TaskDiscardReceipt{}, e
	} else if found {
		if old.Kind != "discard_task" || old.TargetID != taskID || old.ExpectedVersion != expectedVersion || old.RequestedBy != requestedBy || !bytes.Equal(old.Payload, canonicalRequest) {
			return TaskDiscardReceipt{}, nativeErr("idempotency_conflict", "request_id was already used with different discard content")
		}
		if old.Status == "accepted" || old.Status == "rejected" || old.Status == "pending_verification" {
			if old.Status != "pending_verification" {
				var saved TaskDiscardReceipt
				if json.Unmarshal(old.Result, &saved) == nil {
					return saved, nil
				}
			}
		}
	}
	plan, task, err := taskDiscardPlanFromStore(ctx, sqlite, c.Project, taskID)
	if err != nil {
		return TaskDiscardReceipt{}, err
	}
	if task.Version != expectedVersion {
		return TaskDiscardReceipt{}, nativeErr("stale_version", fmt.Sprintf("expected task version %d, found %d", expectedVersion, task.Version))
	}
	intent := taskDiscardIntent{TaskID: taskID, ExpectedVersion: expectedVersion, Reason: reason, RequestedBy: requestedBy, PRURL: plan.PRURL, UncertainPR: plan.UncertainPR, CleanupPending: uniqueSorted(plan.CleanupPending)}
	intentBytes, _ := json.Marshal(intent)
	canonicalIntent, _ := CanonicalJSON(intentBytes)
	var staged OperationRecord
	err = sqlite.Transact(ctx, func(domainTx DomainTx) error {
		tx, ok := domainTx.(*sqliteTx)
		if !ok {
			return errors.New("discard requires the center SQLite transaction")
		}
		if old, found, e := lookupTaskBatchCommandTx(ctx, tx, c.Project, requestID); e != nil {
			return e
		} else if found {
			if old.Kind != "discard_task" || old.TargetID != taskID || old.ExpectedVersion != expectedVersion || old.RequestedBy != requestedBy || !bytes.Equal(old.Payload, canonicalRequest) {
				return nativeErr("idempotency_conflict", "request_id was already used with different discard content")
			}
			return taskBatchReplay{Receipt: TaskBatchReceipt{RequestID: requestID, Status: "accepted"}}
		}
		var oldPayload []byte
		var status string
		var version uint64
		if e := tx.tx.QueryRowContext(ctx, `SELECT payload,status,version FROM tasks WHERE project_identity=? AND task_id=?`, c.Project, taskID).Scan(&oldPayload, &status, &version); e != nil {
			return nativeErr("task_not_found", "native task was not found")
		}
		if version != expectedVersion {
			return nativeErr("stale_version", fmt.Sprintf("expected task version %d, found %d", expectedVersion, version))
		}
		var deliveries int
		if e := tx.tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM task_deliveries WHERE project_identity=? AND task_id=?`, c.Project, taskID).Scan(&deliveries); e != nil {
			return e
		}
		if deliveries > 0 {
			return nativeErr("already_merged", "task already has a verified delivery")
		}
		if status != "open" && status != "in_progress" {
			return nativeErr("immutable_task", "only open or in_progress native tasks can be discarded")
		}
		command := CommandRecord{ProjectIdentity: c.Project, CommandID: requestID, Kind: "discard_task", TargetID: taskID, ExpectedVersion: expectedVersion, IdempotencyKey: requestID, RequestedBy: requestedBy, Payload: canonicalRequest, Status: "in_flight", CreatedAt: time.Now().UTC()}
		var created bool
		command, created, e := tx.EnqueueCommand(command)
		if e != nil {
			return nativeErr("idempotency_conflict", e.Error())
		}
		if !created {
			return nativeErr("idempotency_conflict", "request_id already belongs to another discard command")
		}
		staged = OperationRecord{ProjectIdentity: c.Project, OperationID: requestID, IdempotencyKey: requestID, Kind: "discard_task", Status: "pending", PayloadDigest: digestCanonical(canonicalRequest), Request: canonicalIntent, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
		if _, created, e = tx.PutOperation(staged); e != nil {
			return e
		} else if !created {
			return nativeErr("idempotency_conflict", "discard operation identity is already in use")
		}
		_, e = tx.AppendEvent(DomainEvent{ProjectIdentity: c.Project, TaskID: taskID, ExternalSource: "native-task-command", ExternalID: requestID + ":intent", Type: "task.discard.intent", ReceivedAt: time.Now().UTC(), Payload: canonicalIntent})
		return e
	})
	if replay, ok := err.(taskBatchReplay); ok && replay.Receipt.Status == "accepted" {
		old, found, e := lookupTaskBatchCommand(ctx, sqlite, c.Project, requestID)
		if e != nil {
			return TaskDiscardReceipt{}, e
		}
		if !found {
			return TaskDiscardReceipt{}, nativeErr("unsupported_schema", "discard command checkpoint disappeared")
		}
		if old.Status == "accepted" || old.Status == "rejected" {
			var saved TaskDiscardReceipt
			if e = json.Unmarshal(old.Result, &saved); e != nil {
				return TaskDiscardReceipt{}, e
			}
			return saved, nil
		}
		staged, e = lookupOperation(ctx, sqlite, c.Project, requestID)
		if e != nil {
			return TaskDiscardReceipt{}, e
		}
		if e = json.Unmarshal(staged.Request, &intent); e != nil {
			return TaskDiscardReceipt{}, nativeErr("unsupported_schema", "discard operation intent is unreadable")
		}
	} else if err != nil {
		return TaskDiscardReceipt{}, err
	}
	if staged.OperationID == "" {
		staged, err = lookupOperation(ctx, sqlite, c.Project, requestID)
		if err != nil {
			return TaskDiscardReceipt{}, err
		}
		if err = json.Unmarshal(staged.Request, &intent); err != nil {
			return TaskDiscardReceipt{}, nativeErr("unsupported_schema", "discard operation intent is unreadable")
		}
	}
	if len(staged.Receipt) > 0 {
		var checkpoint taskDiscardIntent
		if err = json.Unmarshal(staged.Receipt, &checkpoint); err != nil {
			return TaskDiscardReceipt{}, nativeErr("unsupported_schema", "discard reconciliation checkpoint is unreadable")
		}
		intent = checkpoint
	}
	currentPlan, currentTask, err := taskDiscardPlanFromStore(ctx, sqlite, c.Project, taskID)
	if err != nil {
		return TaskDiscardReceipt{}, err
	}
	if currentTask.Version != expectedVersion {
		return TaskDiscardReceipt{}, nativeErr("stale_version", fmt.Sprintf("expected task version %d, found %d", expectedVersion, currentTask.Version))
	}
	if intent.PRURL != "" && currentPlan.PRURL != intent.PRURL {
		return persistDiscardPending(ctx, sqlite, c.Project, requestID, staged, intent, "merge_state_unknown", "the fixed PR binding no longer matches the task's recorded publication")
	}
	currentCleanup := uniqueSorted(currentPlan.CleanupPending)
	if intent.PRURL != currentPlan.PRURL || intent.UncertainPR != currentPlan.UncertainPR || !sameStrings(intent.CleanupPending, currentCleanup) {
		intent.PRURL = currentPlan.PRURL
		intent.UncertainPR = currentPlan.UncertainPR
		intent.CleanupPending = currentCleanup
		if err = updateDiscardIntentReceipt(ctx, sqlite, &staged, intent); err != nil {
			return TaskDiscardReceipt{}, err
		}
	}
	var prInfo *PRInfo
	var queryErr error
	if intent.UncertainPR {
		return persistDiscardPending(ctx, sqlite, c.Project, requestID, staged, intent, "merge_state_unknown", "publication outcome is not conclusive")
	}
	if intent.PRURL != "" {
		target, targetErr := configuredRepositoryTarget(ctx, config)
		if targetErr != nil || config.PR.ValidateRemote(target) != nil {
			return persistDiscardPending(ctx, sqlite, c.Project, requestID, staged, intent, "merge_state_unknown", "PR provider configuration does not match the fixed repository target")
		}
		client := PRClient{Config: config.PR, Runner: Runner{Dir: config.Repository, Timeout: duration(config.CommandTimeout)}}
		info, e := client.Query(ctx, intent.PRURL)
		if e != nil {
			queryErr = e
		} else {
			prInfo = &info
		}
		if queryErr != nil {
			return persistDiscardPending(ctx, sqlite, c.Project, requestID, staged, intent, "merge_state_unknown", "PR merge state could not be verified: "+clip(queryErr.Error(), 1000))
		}
	}
	return c.commitDiscard(ctx, sqlite, requestID, staged, intent, prInfo, config.PR.Repository)
}

func updateDiscardIntentReceipt(ctx context.Context, s *SQLiteStore, op *OperationRecord, intent taskDiscardIntent) error {
	data, err := json.Marshal(intent)
	if err != nil {
		return err
	}
	next := *op
	next.Receipt = data
	next.Status = "pending"
	next.UpdatedAt = time.Now().UTC()
	if err = s.Transact(ctx, func(domainTx DomainTx) error { return domainTx.UpdateOperation(next) }); err != nil {
		return err
	}
	*op = next
	return nil
}

func taskDiscardPlanFromStore(ctx context.Context, s *SQLiteStore, project, taskID string) (taskDiscardPlan, TaskRecord, error) {
	snapshot, err := s.Query(ctx, project)
	if err != nil {
		return taskDiscardPlan{}, TaskRecord{}, err
	}
	return taskDiscardPlanFromSnapshot(snapshot, taskID)
}

func taskDiscardPlanFromSnapshot(snapshot DomainSnapshot, taskID string) (taskDiscardPlan, TaskRecord, error) {
	var task TaskRecord
	found := false
	for _, candidate := range snapshot.Tasks {
		if candidate.TaskID == taskID {
			task = candidate
			found = true
			break
		}
	}
	if !found {
		return taskDiscardPlan{}, task, nativeErr("task_not_found", "native task was not found")
	}
	runs := map[string]bool{}
	for _, run := range snapshot.WorkflowRuns {
		if run.TaskID == taskID {
			runs[run.WorkflowRunID] = true
		}
	}
	executions := map[string]ExecutionRecord{}
	for _, execution := range snapshot.Executions {
		if runs[execution.WorkflowRunID] {
			executions[execution.ExecutionID] = execution
		}
	}
	plan := taskDiscardPlan{CleanupPending: []string{}}
	prs := map[string]bool{}
	acceptedPR := map[string]bool{}
	plannedWithoutAction := map[string]bool{}
	for _, execution := range executions {
		if execution.Workspace.Worktree != "" || execution.Target.WorktreeID != "" {
			id := execution.Target.WorktreeID
			if id == "" {
				id = execution.Workspace.Worktree
			}
			plan.CleanupPending = append(plan.CleanupPending, "workspace_cleanup:"+id)
		}
		if execution.Status == "active" || execution.Status == "waiting_decision" || execution.Status == "blocked" || execution.Status == "pending" {
			plan.CleanupPending = append(plan.CleanupPending, "execution_stop_manual:"+execution.ExecutionID)
		}
	}
	for _, attempt := range snapshot.Attempts {
		if _, owned := executions[attempt.ExecutionID]; !owned {
			continue
		}
		var result StepGateResult
		resultValid := len(attempt.Result) > 0 && json.Unmarshal(attempt.Result, &result) == nil
		if attempt.StepID != "publish_pr" {
			if resultValid && result.PRURL != "" {
				plan.UncertainPR = true
			}
			continue
		}
		if attempt.Status == "planned" && attempt.OperationID == "" && len(attempt.Result) == 0 {
			plannedWithoutAction[attempt.ExecutionID] = true
		}
		if resultValid && result.PRURL != "" {
			prs[result.PRURL] = true
			if attempt.Status == "succeeded" && result.Succeeded && result.StepID == "publish_pr" {
				acceptedPR[attempt.ExecutionID] = true
			} else {
				plan.UncertainPR = true
			}
		}
		switch attempt.Status {
		case "dispatching", "dispatched", "running", "stopping", "outcome_unknown":
			plan.UncertainPR = true
		case "succeeded":
			if !resultValid || !result.Succeeded || result.StepID != "publish_pr" || result.PRURL == "" {
				plan.UncertainPR = true
			}
		}
		if attempt.OperationID != "" && len(attempt.Result) == 0 || len(attempt.Result) > 0 && !resultValid {
			plan.UncertainPR = true
		}
	}
	for id, execution := range executions {
		if execution.CurrentStep == "waiting_pr_merge" && !acceptedPR[id] || execution.CurrentStep == "publish_pr" && !acceptedPR[id] && !plannedWithoutAction[id] {
			plan.UncertainPR = true
		}
	}
	if len(prs) > 1 {
		return plan, task, nativeErr("immutable_task", "multiple PR identities are bound to this task; manual reconciliation is required")
	}
	for url := range prs {
		plan.PRURL = url
	}
	plan.CleanupPending = uniqueSorted(plan.CleanupPending)
	return plan, task, nil
}

func uniqueSorted(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}

func lookupOperation(ctx context.Context, s *SQLiteStore, project, id string) (OperationRecord, error) {
	var body []byte
	err := s.db.QueryRowContext(ctx, `SELECT payload FROM operations WHERE project_identity=? AND operation_id=?`, project, id).Scan(&body)
	if err != nil {
		return OperationRecord{}, err
	}
	var op OperationRecord
	if err = json.Unmarshal(body, &op); err != nil {
		return op, err
	}
	return op, nil
}

func persistDiscardPending(ctx context.Context, s *SQLiteStore, project, requestID string, op OperationRecord, intent taskDiscardIntent, code, message string) (TaskDiscardReceipt, error) {
	receipt := TaskDiscardReceipt{RequestID: requestID, TaskID: intent.TaskID, Status: "pending_verification", Code: code, Message: message, CleanupPending: uniqueSorted(intent.CleanupPending)}
	checkpoint, err := json.Marshal(intent)
	if err != nil {
		return TaskDiscardReceipt{}, err
	}
	err = s.Transact(ctx, func(domainTx DomainTx) error {
		tx := domainTx.(*sqliteTx)
		op.Status = "outcome_unknown"
		op.Receipt = checkpoint
		op.UnknownReason = message
		op.UpdatedAt = time.Now().UTC()
		if e := tx.UpdateOperation(op); e != nil {
			return e
		}
		raw, _ := json.Marshal(receipt)
		cmd := CommandRecord{ProjectIdentity: project, CommandID: requestID, Status: "pending_verification", Result: raw}
		if e := tx.UpdateCommand(cmd); e != nil {
			return e
		}
		body, _ := json.Marshal(receipt)
		_, e := tx.AppendEvent(DomainEvent{ProjectIdentity: project, TaskID: intent.TaskID, ExternalSource: "native-task-command", ExternalID: requestID + ":verification-unknown", Type: "task.discard.verification_unknown", ReceivedAt: time.Now().UTC(), Payload: body})
		return e
	})
	return receipt, err
}

func (c CommandService) commitDiscard(ctx context.Context, s *SQLiteStore, requestID string, op OperationRecord, intent taskDiscardIntent, info *PRInfo, expectedPRRepository string) (TaskDiscardReceipt, error) {
	var receipt TaskDiscardReceipt
	err := s.Transact(ctx, func(domainTx DomainTx) error {
		tx := domainTx.(*sqliteTx)
		graph, e := taskGraphSnapshotTx(ctx, tx, c.Project)
		if e != nil {
			return e
		}
		task, ok := graph.ByID[intent.TaskID]
		if !ok {
			return nativeErr("task_not_found", "native task was not found")
		}
		if task.Version != intent.ExpectedVersion {
			return nativeErr("stale_version", fmt.Sprintf("expected task version %d, found %d", intent.ExpectedVersion, task.Version))
		}
		var deliveries int
		if e = tx.tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM task_deliveries WHERE project_identity=? AND task_id=?`, c.Project, intent.TaskID).Scan(&deliveries); e != nil {
			return e
		}
		if deliveries > 0 {
			return nativeErr("already_merged", "task already has a verified delivery")
		}
		if task.Status != "open" && task.Status != "in_progress" {
			return nativeErr("immutable_task", "only open or in_progress native tasks can be discarded")
		}
		if intent.PRURL != "" && (info == nil || info.URL != intent.PRURL || expectedPRRepository != "" && info.Repository != expectedPRRepository || info.BaseBranch != graph.Authority.Branch) {
			return nativeErr("merge_state_unknown", "PR binding or target branch changed during discard verification")
		}
		if info != nil && info.Merged || info != nil && info.State == "merged" {
			receipt = TaskDiscardReceipt{RequestID: requestID, TaskID: intent.TaskID, Status: "rejected", Code: "already_merged", Message: "task PR is already merged", GraphRevision: graph.Authority.GraphRevision}
			return finishDiscardCommand(tx, op, requestID, receipt, "rejected", false)
		}
		if intent.UncertainPR {
			return nativeErr("merge_state_unknown", "PR publication state remains unknown")
		}
		currentPlan, currentTask, e := taskDiscardPlanTx(ctx, tx, c.Project, intent.TaskID)
		if e != nil {
			return e
		}
		if currentTask.Version != intent.ExpectedVersion {
			return nativeErr("stale_version", "task version changed during PR verification")
		}
		if currentPlan.UncertainPR || currentPlan.PRURL != intent.PRURL {
			return nativeErr("merge_state_unknown", "published PR binding changed or remains uncertain")
		}
		if info != nil && info.State != "open" && info.State != "closed_unmerged" {
			return nativeErr("merge_state_unknown", "PR provider did not confirm an unmerged PR state")
		}
		cleanup := append([]string(nil), currentPlan.CleanupPending...)
		if info != nil && info.State == "open" {
			cleanup = append(cleanup, "pull_request_close:"+intent.PRURL)
		}
		cleanup = uniqueSorted(cleanup)
		discardedAt := time.Now().UTC()
		task.Status = "discarded"
		task.Version++
		task.UpdatedAt = discardedAt
		task.DiscardedAt = &discardedAt
		task.DiscardReason = intent.Reason
		task.DiscardedBy = intent.RequestedBy
		task.DiscardRequestID = requestID
		task.CleanupPending = cleanup
		if e = tx.UpdateNativeTaskCAS(task, intent.ExpectedVersion); e != nil {
			return e
		}
		receipt = TaskDiscardReceipt{RequestID: requestID, TaskID: intent.TaskID, Status: "discarded", GraphRevision: graph.Authority.GraphRevision + 1, CleanupPending: cleanup}
		body, _ := json.Marshal(map[string]any{"task_id": intent.TaskID, "request_id": requestID, "reason": intent.Reason, "discarded_by": intent.RequestedBy, "discarded_at": discardedAt, "cleanup_pending": cleanup})
		if _, e = tx.AppendEvent(DomainEvent{ProjectIdentity: c.Project, TaskID: intent.TaskID, ExternalSource: "native-task-command", ExternalID: requestID + ":discarded", Type: "task.discarded", ReceivedAt: discardedAt, Payload: body}); e != nil {
			return e
		}
		return finishDiscardCommand(tx, op, requestID, receipt, "accepted", true)
	})
	return receipt, err
}

func finishDiscardCommand(tx *sqliteTx, op OperationRecord, requestID string, receipt TaskDiscardReceipt, status string, confirmed bool) error {
	receiptBytes, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	command := CommandRecord{ProjectIdentity: op.ProjectIdentity, CommandID: requestID, Status: status, Result: receiptBytes}
	if err = tx.UpdateCommand(command); err != nil {
		return err
	}
	if confirmed {
		op.Status = "confirmed"
	} else {
		op.Status = "rejected"
	}
	op.UpdatedAt = time.Now().UTC()
	if err = tx.UpdateOperation(op); err != nil {
		return err
	}
	return nil
}

func taskDiscardPlanTx(ctx context.Context, tx *sqliteTx, project, taskID string) (taskDiscardPlan, TaskRecord, error) {
	snapshot, err := querySnapshot(ctx, tx.tx, project)
	if err != nil {
		return taskDiscardPlan{}, TaskRecord{}, err
	}
	return taskDiscardPlanFromSnapshot(snapshot, taskID)
}

func dependencyCycle(edges []TaskDependencyRecord) []string {
	adj := map[string][]string{}
	for _, edge := range edges {
		adj[edge.PredecessorID] = append(adj[edge.PredecessorID], edge.SuccessorID)
		if _, ok := adj[edge.SuccessorID]; !ok {
			adj[edge.SuccessorID] = nil
		}
	}
	for n := range adj {
		sort.Slice(adj[n], func(i, j int) bool { return decimalIDLess(adj[n][i], adj[n][j]) })
	}
	state := map[string]uint8{}
	stack := []string{}
	positions := map[string]int{}
	var found []string
	var visit func(string) bool
	visit = func(n string) bool {
		state[n] = 1
		positions[n] = len(stack)
		stack = append(stack, n)
		for _, next := range adj[n] {
			if state[next] == 0 {
				if visit(next) {
					return true
				}
			} else if state[next] == 1 {
				found = append(append([]string(nil), stack[positions[next]:]...), next)
				return true
			}
		}
		stack = stack[:len(stack)-1]
		delete(positions, n)
		state[n] = 2
		return false
	}
	nodes := make([]string, 0, len(adj))
	for n := range adj {
		nodes = append(nodes, n)
	}
	sort.Slice(nodes, func(i, j int) bool { return decimalIDLess(nodes[i], nodes[j]) })
	for _, n := range nodes {
		if state[n] == 0 && visit(n) {
			return found
		}
	}
	return nil
}

func decimalIDLess(a, b string) bool {
	if len(a) != len(b) {
		return len(a) < len(b)
	}
	return a < b
}

func StableTaskOrder(tasks []TaskRecord, edges []TaskDependencyRecord) []string {
	byID := map[string]TaskRecord{}
	indegree := map[string]int{}
	children := map[string][]string{}
	for _, task := range tasks {
		byID[task.TaskID] = task
		indegree[task.TaskID] = 0
	}
	for _, edge := range edges {
		if _, ok := byID[edge.PredecessorID]; !ok {
			continue
		}
		if _, ok := byID[edge.SuccessorID]; !ok {
			continue
		}
		indegree[edge.SuccessorID]++
		children[edge.PredecessorID] = append(children[edge.PredecessorID], edge.SuccessorID)
	}
	less := func(a, b string) bool {
		x, y := byID[a], byID[b]
		if x.Priority != y.Priority {
			return x.Priority < y.Priority
		}
		if !x.CreatedAt.Equal(y.CreatedAt) {
			return x.CreatedAt.Before(y.CreatedAt)
		}
		return decimalIDLess(a, b)
	}
	ready := []string{}
	for id, n := range indegree {
		if n == 0 {
			ready = append(ready, id)
		}
	}
	sort.Slice(ready, func(i, j int) bool { return less(ready[i], ready[j]) })
	order := make([]string, 0, len(tasks))
	for len(ready) > 0 {
		id := ready[0]
		ready = ready[1:]
		order = append(order, id)
		for _, child := range children[id] {
			indegree[child]--
			if indegree[child] == 0 {
				ready = append(ready, child)
				sort.Slice(ready, func(i, j int) bool { return less(ready[i], ready[j]) })
			}
		}
	}
	return order
}

func digestCanonical(b []byte) string { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:]) }
