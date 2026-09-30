package task

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/taskgraph"
)

const BatchSchemaVersion = "task-batch/v1"

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

type Snapshot struct {
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

type TaskDependencyStatus struct {
	TaskID    string `json:"task_id"`
	Status    string `json:"status"`
	Delivered bool   `json:"delivered"`
	Blocker   string `json:"blocker,omitempty"`
}

func DecodeBatch(data []byte) (TaskBatchInput, []byte, error) {
	canonical, err := CanonicalJSON(data)
	if err != nil {
		return TaskBatchInput{}, nil, newError("invalid_batch", err.Error())
	}
	var batch TaskBatchInput
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err = d.Decode(&batch); err != nil {
		return batch, nil, newError("invalid_batch", err.Error())
	}
	var extra any
	if err = d.Decode(&extra); err != io.EOF {
		return batch, nil, newError("invalid_batch", "expected exactly one JSON object")
	}
	var shape map[string]json.RawMessage
	if err = json.Unmarshal(data, &shape); err != nil {
		return batch, nil, newError("invalid_batch", err.Error())
	}
	if shape["schema_version"] == nil || shape["project_id"] == nil || shape["tasks"] == nil {
		return batch, nil, newError("invalid_batch", "schema_version, project_id and tasks are required")
	}
	if raw := shape["dependencies"]; raw != nil && (bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || len(bytes.TrimSpace(raw)) == 0 || bytes.TrimSpace(raw)[0] != '[') {
		return batch, nil, newError("invalid_batch", "dependencies must be an array when provided")
	}
	if batch.SchemaVersion != BatchSchemaVersion {
		return batch, nil, newError("invalid_batch", "schema_version must be task-batch/v1")
	}
	if strings.TrimSpace(batch.ProjectID) == "" || len(batch.Tasks) == 0 {
		return batch, nil, newError("invalid_batch", "project_id and a nonempty tasks array are required")
	}
	var taskShapes []map[string]json.RawMessage
	if err = json.Unmarshal(shape["tasks"], &taskShapes); err != nil || len(taskShapes) != len(batch.Tasks) {
		return batch, nil, newError("invalid_batch", "tasks must be an array of objects")
	}
	for i, taskShape := range taskShapes {
		for _, name := range []string{"local_key", "task_id", "title", "type", "contract"} {
			if taskShape[name] == nil || bytes.Equal(bytes.TrimSpace(taskShape[name]), []byte("null")) {
				return batch, nil, newError("invalid_batch", fmt.Sprintf("tasks[%d].%s is required", i, name))
			}
		}
		if raw := taskShape["priority"]; raw != nil && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return batch, nil, newError("invalid_batch", fmt.Sprintf("tasks[%d].priority must be an integer when provided", i))
		}
		if strings.TrimSpace(batch.Tasks[i].LocalKey) == "" {
			return batch, nil, newError("invalid_batch", fmt.Sprintf("tasks[%d].local_key is empty", i))
		}
		if batch.Tasks[i].TaskID < 0 {
			return batch, nil, newError("invalid_batch", fmt.Sprintf("tasks[%d].task_id must be a nonnegative integer", i))
		}
		if strings.TrimSpace(batch.Tasks[i].Title) == "" {
			return batch, nil, newError("invalid_batch", fmt.Sprintf("tasks[%d].title is empty", i))
		}
	}
	return batch, canonical, nil
}

func ResolveBatchEdges(batch TaskBatchInput) []TaskDependencyRecord {
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

func BatchValidationError(issues []TaskBatchIssue, cycle []string) error {
	code := "invalid_batch"
	if len(issues) > 0 && issues[0].Code != "" {
		code = issues[0].Code
	}
	if len(cycle) > 0 {
		code = "dependency_cycle"
	}
	return newError(code, formatTaskBatchIssues(issues, cycle))
}

func ValidateBatchAgainstGraph(batch TaskBatchInput, graph Snapshot, executableTypes []string, verifyContract func(TaskContract) error) ([]TaskBatchIssue, []string) {
	issues := []TaskBatchIssue{}
	if verifyContract == nil {
		verifyContract = ValidateContract
	}
	issue := func(code, node, message string) {
		issues = append(issues, TaskBatchIssue{Code: code, Node: node, Message: message})
	}
	if batch.ProjectID != graph.Authority.ProjectIdentity {
		issue("authority_mismatch", "project_id", "batch project does not match the sampled native project")
	}
	allowedTypes := map[string]bool{"task": true, "bug": true, "feature": true, "chore": true}
	configured := map[string]bool{}
	types := executableTypes
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
		if err := verifyContract(task.Contract); err != nil {
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

func dependencyCycle(edges []TaskDependencyRecord) []string {
	graphEdges := make([]taskgraph.Edge, 0, len(edges))
	for _, edge := range edges {
		graphEdges = append(graphEdges, taskgraph.Edge{Predecessor: edge.PredecessorID, Successor: edge.SuccessorID})
	}
	return taskgraph.Cycle(graphEdges)
}
