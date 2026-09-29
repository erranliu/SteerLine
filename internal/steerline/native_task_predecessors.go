package steerline

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"
)

// TaskPredecessorAddInput adds hard dependencies to an existing native task.
// A claimed task must identify its stopped run so the same transaction can
// abandon that run and make the task claimable from a fresh base.
type TaskPredecessorAddInput struct {
	TaskID                  string   `json:"task_id"`
	PredecessorIDs          []string `json:"predecessor_ids"`
	RequestID               string   `json:"request_id"`
	ExpectedTaskVersion     uint64   `json:"expected_task_version"`
	ExpectedGraphRevision   uint64   `json:"expected_graph_revision"`
	WorkflowRunID           string   `json:"workflow_run_id,omitempty"`
	ConfirmedProcessStopped bool     `json:"confirmed_process_stopped,omitempty"`
	Reason                  string   `json:"reason"`
	RequestedBy             string   `json:"requested_by"`
}

type TaskPredecessorAddReceipt struct {
	RequestID      string   `json:"request_id"`
	TaskID         string   `json:"task_id"`
	PredecessorIDs []string `json:"predecessor_ids"`
	TaskVersion    uint64   `json:"task_version"`
	GraphRevision  uint64   `json:"graph_revision"`
	AbandonedRunID string   `json:"abandoned_run_id,omitempty"`
	Status         string   `json:"status"`
}

func normalizePredecessorIDs(ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, nativeErr("invalid_batch", "at least one predecessor is required")
	}
	out := append([]string(nil), ids...)
	for _, id := range out {
		if err := ValidateNativeTaskID(id); err != nil {
			return nil, err
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if len(out[i]) != len(out[j]) {
			return len(out[i]) < len(out[j])
		}
		return out[i] < out[j]
	})
	for i := 1; i < len(out); i++ {
		if out[i] == out[i-1] {
			return nil, nativeErr("duplicate_dependency", "predecessor appears more than once")
		}
	}
	return out, nil
}

func validateNativePredecessorAddition(graph taskGraphSnapshot, taskID string, predecessorIDs []string) error {
	successor, exists := graph.ByID[taskID]
	if !exists {
		return nativeErr("task_not_found", "successor is not present in the native project graph")
	}
	_, delivered := graph.Deliveries[taskID]
	if successor.Status == "closed" || delivered {
		return nativeErr("already_merged", "delivered task cannot acquire a new predecessor")
	}
	if successor.Status == "discarded" {
		return nativeErr("task_discarded", "discarded task cannot acquire a new predecessor")
	}
	edges := append([]TaskDependencyRecord(nil), graph.Edges...)
	seen := map[string]bool{}
	for _, edge := range graph.Edges {
		seen[edge.PredecessorID+"\x00"+edge.SuccessorID] = true
	}
	for _, predecessorID := range predecessorIDs {
		if predecessorID == taskID {
			return nativeErr("dependency_cycle", "task cannot depend on itself")
		}
		predecessor, found := graph.ByID[predecessorID]
		if !found {
			if graph.RepositoryTaskIDs[predecessorID] {
				return nativeErr("cross_project_dependency", "predecessor belongs to another native project")
			}
			return nativeErr("missing_predecessor", "predecessor is not present in the native project graph")
		}
		if predecessor.Status == "discarded" {
			return nativeErr("discarded_predecessor", "discarded task cannot become a new predecessor")
		}
		key := predecessorID + "\x00" + taskID
		if seen[key] {
			return nativeErr("duplicate_dependency", "dependency already exists")
		}
		seen[key] = true
		edges = append(edges, TaskDependencyRecord{ProjectIdentity: graph.Authority.ProjectIdentity, PredecessorID: predecessorID, SuccessorID: taskID})
	}
	if len(dependencyCycle(edges)) != 0 {
		return nativeErr("dependency_cycle", "new predecessor would create a dependency cycle")
	}
	return nil
}

func (c CommandService) AddNativeTaskPredecessors(ctx context.Context, config Config, input TaskPredecessorAddInput) (TaskPredecessorAddReceipt, error) {
	var empty TaskPredecessorAddReceipt
	store, ok := c.Store.(*SQLiteStore)
	if !ok || c.Project == "" || c.Project != config.Identity() || config.TaskAuthority != "native" {
		return empty, nativeErr("authority_mismatch", "predecessor addition requires the configured native project")
	}
	if err := ValidateNativeTaskID(input.TaskID); err != nil {
		return empty, err
	}
	if !controlIDPattern.MatchString(input.RequestID) || input.ExpectedTaskVersion == 0 || strings.TrimSpace(input.RequestedBy) == "" {
		return empty, nativeErr("invalid_batch", "predecessor addition requires request, task version and requester")
	}
	reason, err := validateConfirmationBasis(input.Reason)
	if err != nil {
		return empty, err
	}
	input.Reason = reason
	input.PredecessorIDs, err = normalizePredecessorIDs(input.PredecessorIDs)
	if err != nil {
		return empty, err
	}
	if input.WorkflowRunID != "" || input.ConfirmedProcessStopped {
		return empty, nativeErr("unsupported_schema", "reopening a claimed task through the old workflow is unsupported")
	}
	request, _ := CanonicalJSON(mustJSON(input))
	replay := func(command CommandRecord) (TaskPredecessorAddReceipt, error) {
		if command.Kind != "add_task_predecessors" || command.TargetID != input.TaskID || command.ExpectedVersion != input.ExpectedTaskVersion || command.RequestedBy != input.RequestedBy || !bytes.Equal(command.Payload, request) || command.Status != "accepted" {
			return empty, nativeErr("idempotency_conflict", "request ID belongs to a different or incomplete command")
		}
		var receipt TaskPredecessorAddReceipt
		if err := json.Unmarshal(command.Result, &receipt); err != nil {
			return empty, err
		}
		return receipt, nil
	}
	if previous, lookupErr := store.CommandByIdempotency(ctx, c.Project, input.RequestID); lookupErr == nil {
		return replay(previous)
	} else if !errors.Is(lookupErr, sql.ErrNoRows) {
		return empty, lookupErr
	}
	authority, err := store.nativeAuthority(ctx, c.Project)
	if err != nil {
		return empty, err
	}
	if err = validateNativeConfiguredTarget(ctx, config, authority); err != nil {
		return empty, err
	}
	var receipt TaskPredecessorAddReceipt
	err = store.Transact(ctx, func(domainTx DomainTx) error {
		tx := domainTx.(*sqliteTx)
		if previous, found, lookupErr := lookupTaskBatchCommandTx(ctx, tx, c.Project, input.RequestID); lookupErr != nil {
			return lookupErr
		} else if found {
			var replayErr error
			receipt, replayErr = replay(previous)
			if replayErr != nil {
				return replayErr
			}
			return taskBatchReplay{}
		}
		graph, e := taskGraphSnapshotTx(ctx, tx, c.Project)
		if e != nil {
			return e
		}
		if graph.Authority.GraphRevision != input.ExpectedGraphRevision {
			return nativeErr("stale_version", "native graph changed before predecessor addition")
		}
		task, found := graph.ByID[input.TaskID]
		if !found {
			return nativeErr("task_not_found", "successor is not present in the native project graph")
		}
		if task.Version != input.ExpectedTaskVersion {
			return nativeErr("stale_version", "task version changed before predecessor addition")
		}
		if task.Status != "open" {
			if task.Status == "in_progress" {
				return nativeErr("cleanup_pending", "claimed successor requires its exact stopped workflow run and reset evidence")
			}
			return nativeErr("immutable_task", "successor cannot acquire a predecessor in its current state")
		}
		if e = validateNativePredecessorAddition(graph, input.TaskID, input.PredecessorIDs); e != nil {
			return e
		}
		stamp := time.Now().UTC()
		for _, predecessorID := range input.PredecessorIDs {
			if e = tx.PutTaskDependency(TaskDependencyRecord{ProjectIdentity: c.Project, PredecessorID: predecessorID, SuccessorID: input.TaskID}); e != nil {
				return e
			}
		}
		nextTask := task
		nextTask.Version++
		nextTask.UpdatedAt = stamp
		if e = tx.bumpNativeOpenTaskVersionCAS(nextTask, task.Version); e != nil {
			return e
		}
		graphRevision, e := tx.AdvanceTaskGraphRevision(c.Project, graph.Authority.AuthorityVersion, graph.Authority.GraphRevision)
		if e != nil {
			return e
		}
		receipt = TaskPredecessorAddReceipt{RequestID: input.RequestID, TaskID: input.TaskID, PredecessorIDs: input.PredecessorIDs, TaskVersion: nextTask.Version, GraphRevision: graphRevision, Status: "open"}
		result, _ := json.Marshal(receipt)
		command := CommandRecord{ProjectIdentity: c.Project, CommandID: input.RequestID, Kind: "add_task_predecessors", TargetID: input.TaskID, ExpectedVersion: input.ExpectedTaskVersion, IdempotencyKey: input.RequestID, RequestedBy: input.RequestedBy, Payload: request, Status: "accepted", Result: result, CreatedAt: stamp, UpdatedAt: stamp}
		if _, made, enqueueErr := tx.EnqueueCommand(command); enqueueErr != nil || !made {
			return errors.Join(enqueueErr, nativeErr("idempotency_conflict", "predecessor command already exists"))
		}
		operation := OperationRecord{ProjectIdentity: c.Project, OperationID: input.RequestID, IdempotencyKey: input.RequestID, Kind: "add_task_predecessors", Status: "confirmed", PayloadDigest: digestCanonical(request), Request: request, Receipt: result, CreatedAt: stamp, UpdatedAt: stamp}
		if _, made, opErr := tx.PutOperation(operation); opErr != nil || !made {
			return errors.Join(opErr, nativeErr("idempotency_conflict", "predecessor operation already exists"))
		}
		body, _ := json.Marshal(map[string]any{"receipt": receipt, "reason": input.Reason, "requested_by": input.RequestedBy})
		_, e = tx.AppendEvent(DomainEvent{ProjectIdentity: c.Project, TaskID: input.TaskID, ExternalSource: "native-task-command", ExternalID: input.RequestID, Type: "task.predecessors.added", ReceivedAt: stamp, Payload: body})
		return e
	})
	var already taskBatchReplay
	if errors.As(err, &already) {
		return receipt, nil
	}
	return receipt, err
}

func (t *sqliteTx) bumpNativeOpenTaskVersionCAS(next TaskRecord, expectedVersion uint64) error {
	if next.Status != "open" || next.Claimant != "" || next.Version != expectedVersion+1 {
		return nativeErr("contract_mismatch", "predecessor addition requires an unclaimed open task")
	}
	var oldPayload []byte
	if err := t.tx.QueryRowContext(t.ctx, `SELECT payload FROM tasks WHERE project_identity=? AND task_id=? AND status='open' AND version=?`, next.ProjectIdentity, next.TaskID, expectedVersion).Scan(&oldPayload); err != nil {
		return nativeErr("stale_version", "task changed before predecessor addition")
	}
	var old TaskRecord
	if err := json.Unmarshal(oldPayload, &old); err != nil {
		return err
	}
	old.Version, old.UpdatedAt = next.Version, next.UpdatedAt
	if !sameTaskRecord(old, next) {
		return nativeErr("contract_mismatch", "predecessor addition cannot alter the task contract or identity")
	}
	var active, delivered int
	if err := t.tx.QueryRowContext(t.ctx, `SELECT COUNT(*) FROM workflow_runs WHERE project_identity=? AND task_id=? AND phase NOT IN ('completed','abandoned')`, next.ProjectIdentity, next.TaskID).Scan(&active); err != nil {
		return err
	}
	if err := t.tx.QueryRowContext(t.ctx, `SELECT COUNT(*) FROM task_deliveries WHERE project_identity=? AND task_id=?`, next.ProjectIdentity, next.TaskID).Scan(&delivered); err != nil {
		return err
	}
	if active != 0 || delivered != 0 {
		return nativeErr("inconsistent_delivery", "open task still has an active workflow or delivery")
	}
	body, err := json.Marshal(next)
	if err != nil {
		return err
	}
	result, err := t.tx.ExecContext(t.ctx, `UPDATE tasks SET version=?,payload=? WHERE project_identity=? AND task_id=? AND status='open' AND version=?`, next.Version, body, next.ProjectIdentity, next.TaskID, expectedVersion)
	return requireIdentityWrite(result, err, "stale_version: task changed before predecessor addition")
}

func sameTaskRecord(a, b TaskRecord) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return bytes.Equal(x, y)
}
