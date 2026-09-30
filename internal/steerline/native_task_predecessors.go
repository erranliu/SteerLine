package steerline

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/task"
)

// TaskPredecessorAddInput adds hard dependencies to an existing native task.
// A claimed task must identify its stopped run so the same transaction can
// abandon that run and make the task claimable from a fresh base.
type TaskPredecessorAddInput = task.TaskPredecessorAddInput
type TaskPredecessorAddReceipt = task.TaskPredecessorAddReceipt

func normalizePredecessorIDs(ids []string) ([]string, error) {
	return task.NormalizePredecessorIDs(ids)
}

func validateNativePredecessorAddition(graph taskGraphSnapshot, taskID string, predecessorIDs []string) error {
	return task.ValidatePredecessorAddition(graph, taskID, predecessorIDs)
}

func (c CommandService) AddNativeTaskPredecessors(ctx context.Context, config Config, input TaskPredecessorAddInput) (TaskPredecessorAddReceipt, error) {
	var empty TaskPredecessorAddReceipt
	store := c.Store
	if store == nil || c.Project == "" || c.Project != config.Identity() || config.TaskAuthority != "native" {
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
	err = store.transactSQLite(ctx, func(tx *sqliteTx) error {
		if previous, found, lookupErr := tx.lookupNativeCommand(ctx, c.Project, input.RequestID); lookupErr != nil {
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
