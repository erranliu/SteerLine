package task

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func validateNativePredecessorAddition(graph taskmodel.Snapshot, taskID string, predecessorIDs []string) error {
	return taskmodel.ValidatePredecessorAddition(graph, taskID, predecessorIDs)
}

func (c Commands) AddNativeTaskPredecessors(ctx context.Context, input taskmodel.PredecessorAddInput) (taskmodel.PredecessorAddReceipt, error) {
	if err := c.validateInput(); err != nil {
		return taskmodel.PredecessorAddReceipt{}, err
	}
	var empty taskmodel.PredecessorAddReceipt
	store := c.Store
	if store == nil || c.Input.Project == "" || c.Input.Authority != "native" {
		return empty, taskError("authority_mismatch", "predecessor addition requires the configured native project")
	}
	if err := taskmodel.ValidateID(input.TaskID); err != nil {
		return empty, err
	}
	if !controlIDPattern.MatchString(input.RequestID) || input.ExpectedTaskVersion == 0 || strings.TrimSpace(input.RequestedBy) == "" {
		return empty, taskError("invalid_batch", "predecessor addition requires request, task version and requester")
	}
	reason, err := validateConfirmationBasis(input.Reason)
	if err != nil {
		return empty, err
	}
	input.Reason = reason
	input.PredecessorIDs, err = taskmodel.NormalizePredecessorIDs(input.PredecessorIDs)
	if err != nil {
		return empty, err
	}
	if input.WorkflowRunID != "" || input.ConfirmedProcessStopped {
		return empty, taskError("unsupported_schema", "reopening a claimed task through the old workflow is unsupported")
	}
	request, _ := taskmodel.CanonicalJSON(mustJSON(input))
	replay := func(command Command) (taskmodel.PredecessorAddReceipt, error) {
		if command.Kind != "add_task_predecessors" || command.TargetID != input.TaskID || command.ExpectedVersion != input.ExpectedTaskVersion || command.RequestedBy != input.RequestedBy || !bytes.Equal(command.Payload, request) || command.Status != "accepted" {
			return empty, taskError("idempotency_conflict", "request ID belongs to a different or incomplete command")
		}
		var receipt taskmodel.PredecessorAddReceipt
		if err := json.Unmarshal(command.Result, &receipt); err != nil {
			return empty, err
		}
		return receipt, nil
	}
	if previous, lookupErr := lookupTaskCommand(ctx, store, c.Input.Project, input.RequestID); lookupErr == nil {
		return replay(previous)
	} else if !errors.Is(lookupErr, errCommandMissing) {
		return empty, lookupErr
	}
	if err = c.checkTarget(ctx); err != nil {
		return empty, err
	}
	var receipt taskmodel.PredecessorAddReceipt
	err = store.TransactPredecessors(ctx, func(tx PredecessorTx) error {
		if previous, found, lookupErr := tx.LookupCommand(ctx, c.Input.Project, input.RequestID); lookupErr != nil {
			return lookupErr
		} else if found {
			var replayErr error
			receipt, replayErr = replay(previous)
			if replayErr != nil {
				return replayErr
			}
			return taskBatchReplay{}
		}
		graph, e := tx.TaskSnapshot(ctx, c.Input.Project)
		if e != nil {
			return e
		}
		if graph.Authority.GraphRevision != input.ExpectedGraphRevision {
			return taskError("stale_version", "native graph changed before predecessor addition")
		}
		task, found := graph.ByID[input.TaskID]
		if !found {
			return taskError("task_not_found", "successor is not present in the native project graph")
		}
		if task.Version != input.ExpectedTaskVersion {
			return taskError("stale_version", "task version changed before predecessor addition")
		}
		if task.Status != "open" {
			if task.Status == "in_progress" {
				return taskError("stop_pending", "claimed successor requires its exact stopped workflow run and reset evidence")
			}
			return taskError("immutable_task", "successor cannot acquire a predecessor in its current state")
		}
		if e = validateNativePredecessorAddition(graph, input.TaskID, input.PredecessorIDs); e != nil {
			return e
		}
		stamp := time.Now().UTC()
		for _, predecessorID := range input.PredecessorIDs {
			if e = tx.PutTaskDependency(taskmodel.TaskDependencyRecord{ProjectIdentity: c.Input.Project, PredecessorID: predecessorID, SuccessorID: input.TaskID}); e != nil {
				return e
			}
		}
		nextTask := task
		nextTask.Version++
		nextTask.UpdatedAt = stamp
		if e = tx.BumpOpenTaskVersionCAS(nextTask, task.Version); e != nil {
			return e
		}
		graphRevision, e := tx.AdvanceTaskGraphRevision(c.Input.Project, graph.Authority.AuthorityVersion, graph.Authority.GraphRevision)
		if e != nil {
			return e
		}
		receipt = taskmodel.PredecessorAddReceipt{RequestID: input.RequestID, TaskID: input.TaskID, PredecessorIDs: input.PredecessorIDs, TaskVersion: nextTask.Version, GraphRevision: graphRevision, Status: "open"}
		result, _ := json.Marshal(receipt)
		command := Command{ProjectIdentity: c.Input.Project, CommandID: input.RequestID, Kind: "add_task_predecessors", TargetID: input.TaskID, ExpectedVersion: input.ExpectedTaskVersion, IdempotencyKey: input.RequestID, RequestedBy: input.RequestedBy, Payload: request, Status: "accepted", Result: result, CreatedAt: stamp, UpdatedAt: stamp}
		if _, made, enqueueErr := tx.EnqueueCommand(command); enqueueErr != nil || !made {
			return errors.Join(enqueueErr, taskError("idempotency_conflict", "predecessor command already exists"))
		}
		if e = tx.RecordPredecessorAudit(command); e != nil {
			return e
		}
		body, _ := json.Marshal(map[string]any{"receipt": receipt, "reason": input.Reason, "requested_by": input.RequestedBy})
		_, e = tx.AppendEvent(Event{ProjectIdentity: c.Input.Project, TaskID: input.TaskID, ExternalSource: "native-task-command", ExternalID: input.RequestID, Type: "task.predecessors.added", ReceivedAt: stamp, Payload: body})
		return e
	})
	var already taskBatchReplay
	if errors.As(err, &already) {
		return receipt, nil
	}
	if err != nil {
		return empty, err
	}
	return receipt, nil
}
