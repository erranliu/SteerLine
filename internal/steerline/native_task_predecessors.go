package steerline

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/steerline/persistence"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func validateNativePredecessorAddition(graph taskGraphSnapshot, taskID string, predecessorIDs []string) error {
	return taskmodel.ValidatePredecessorAddition(graph.TaskGraph(), taskID, predecessorIDs)
}

func (c CommandService) AddNativeTaskPredecessors(ctx context.Context, config Config, input productapi.TaskPredecessorAddInput) (productapi.TaskPredecessorAddReceipt, error) {
	if err := validateWorkflowConfig(&config, nil); err != nil {
		return productapi.TaskPredecessorAddReceipt{}, err
	}
	var empty productapi.TaskPredecessorAddReceipt
	store := c.Store
	if store == nil || c.Project == "" || c.Project != config.Identity() || config.TaskAuthority != "native" {
		return empty, nativeErr("authority_mismatch", "predecessor addition requires the configured native project")
	}
	if err := taskmodel.ValidateID(input.TaskID); err != nil {
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
	input.PredecessorIDs, err = taskmodel.NormalizePredecessorIDs(input.PredecessorIDs)
	if err != nil {
		return empty, err
	}
	if input.WorkflowRunID != "" || input.ConfirmedProcessStopped {
		return empty, nativeErr("unsupported_schema", "reopening a claimed task through the old workflow is unsupported")
	}
	request, _ := taskmodel.CanonicalJSON(mustJSON(input))
	replay := func(command storageports.CommandRecord) (productapi.TaskPredecessorAddReceipt, error) {
		if command.Kind != "add_task_predecessors" || command.TargetID != input.TaskID || command.ExpectedVersion != input.ExpectedTaskVersion || command.RequestedBy != input.RequestedBy || !bytes.Equal(command.Payload, request) || command.Status != "accepted" {
			return empty, nativeErr("idempotency_conflict", "request ID belongs to a different or incomplete command")
		}
		var receipt productapi.TaskPredecessorAddReceipt
		if err := json.Unmarshal(command.Result, &receipt); err != nil {
			return empty, err
		}
		return receipt, nil
	}
	if previous, lookupErr := store.CommandByIdempotency(ctx, c.Project, input.RequestID); lookupErr == nil {
		return replay(previous)
	} else if !errors.Is(lookupErr, storageports.ErrNotFound) {
		return empty, lookupErr
	}
	authority, err := store.NativeAuthority(ctx, c.Project)
	if err != nil {
		return empty, err
	}
	if err = validateNativeConfiguredTarget(ctx, config, authority); err != nil {
		return empty, err
	}
	var receipt productapi.TaskPredecessorAddReceipt
	err = store.TransactPredecessors(ctx, func(tx persistence.PredecessorTx) error {
		if previous, found, lookupErr := tx.LookupCommand(ctx, c.Project, input.RequestID); lookupErr != nil {
			return lookupErr
		} else if found {
			var replayErr error
			receipt, replayErr = replay(previous)
			if replayErr != nil {
				return replayErr
			}
			return taskBatchReplay{}
		}
		graph, e := tx.TaskGraphSnapshot(ctx, c.Project)
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
			if e = tx.PutTaskDependency(taskmodel.TaskDependencyRecord{ProjectIdentity: c.Project, PredecessorID: predecessorID, SuccessorID: input.TaskID}); e != nil {
				return e
			}
		}
		nextTask := task
		nextTask.Version++
		nextTask.UpdatedAt = stamp
		if e = tx.BumpOpenTaskVersionCAS(nextTask, task.Version); e != nil {
			return e
		}
		graphRevision, e := tx.AdvanceTaskGraphRevision(c.Project, graph.Authority.AuthorityVersion, graph.Authority.GraphRevision)
		if e != nil {
			return e
		}
		receipt = productapi.TaskPredecessorAddReceipt{RequestID: input.RequestID, TaskID: input.TaskID, PredecessorIDs: input.PredecessorIDs, TaskVersion: nextTask.Version, GraphRevision: graphRevision, Status: "open"}
		result, _ := json.Marshal(receipt)
		command := storageports.CommandRecord{ProjectIdentity: c.Project, CommandID: input.RequestID, Kind: "add_task_predecessors", TargetID: input.TaskID, ExpectedVersion: input.ExpectedTaskVersion, IdempotencyKey: input.RequestID, RequestedBy: input.RequestedBy, Payload: request, Status: "accepted", Result: result, CreatedAt: stamp, UpdatedAt: stamp}
		if _, made, enqueueErr := tx.EnqueueCommand(command); enqueueErr != nil || !made {
			return errors.Join(enqueueErr, nativeErr("idempotency_conflict", "predecessor command already exists"))
		}
		operation := executionmodel.OperationRecord{ProjectIdentity: c.Project, OperationID: input.RequestID, IdempotencyKey: input.RequestID, Kind: "add_task_predecessors", Status: "confirmed", PayloadDigest: digestCanonical(request), Request: request, Receipt: result, CreatedAt: stamp, UpdatedAt: stamp}
		if _, made, opErr := tx.PutOperation(operation); opErr != nil || !made {
			return errors.Join(opErr, nativeErr("idempotency_conflict", "predecessor operation already exists"))
		}
		body, _ := json.Marshal(map[string]any{"receipt": receipt, "reason": input.Reason, "requested_by": input.RequestedBy})
		_, e = tx.AppendEvent(storageports.DomainEvent{ProjectIdentity: c.Project, TaskID: input.TaskID, ExternalSource: "native-task-command", ExternalID: input.RequestID, Type: "task.predecessors.added", ReceivedAt: stamp, Payload: body})
		return e
	})
	var already taskBatchReplay
	if errors.As(err, &already) {
		return receipt, nil
	}
	return receipt, err
}
