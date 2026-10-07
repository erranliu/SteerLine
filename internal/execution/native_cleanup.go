package execution

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

type NativeExecutionStopReceipt struct {
	WorkflowRunID string                       `json:"workflow_run_id"`
	ExecutionID   string                       `json:"execution_id"`
	AttemptID     string                       `json:"attempt_id"`
	Operation     executionbackend.Operation   `json:"operation"`
	Observation   executionbackend.Observation `json:"observation"`
}

// RecordNativeStopReceipt settles one exact stop item from a discarded
// native task. It never changes task identity, discarded status, or readiness.
func (c CommandService) RecordNativeStopReceipt(ctx context.Context, taskID, requestID string, expectedVersion uint64, stopItem, requestedBy string, receipt json.RawMessage) (taskmodel.TaskRecord, error) {
	sqlite := c.Store
	if sqlite == nil || c.Project == "" || !controlIDPattern.MatchString(requestID) || expectedVersion == 0 || stopItem == "" || !json.Valid(receipt) {
		return taskmodel.TaskRecord{}, nativeErr("contract_mismatch", "native stop receipt requires project, request, version, stop identity and JSON evidence")
	}
	if err := taskmodel.ValidateID(taskID); err != nil {
		return taskmodel.TaskRecord{}, err
	}
	if strings.TrimSpace(requestedBy) == "" {
		requestedBy = c.defaultRequestedBy()
	}
	request, err := taskmodel.CanonicalJSON(mustJSON(struct {
		TaskID      string          `json:"task_id"`
		StopItem    string          `json:"stop_item"`
		RequestedBy string          `json:"requested_by"`
		Receipt     json.RawMessage `json:"receipt"`
	}{taskID, stopItem, requestedBy, receipt}))
	if err != nil {
		return taskmodel.TaskRecord{}, err
	}
	if old, found, lookupErr := sqlite.LookupCommand(ctx, c.Project, requestID); lookupErr != nil {
		return taskmodel.TaskRecord{}, lookupErr
	} else if found {
		return replayNativeStopReceipt(old, request, c.Project, taskID, requestedBy)
	}
	nowAt := time.Now().UTC()
	var result taskmodel.TaskRecord
	err = sqlite.TransactStopReceipt(ctx, func(tx storageports.StopReceiptTx) error {
		if old, found, lookupErr := tx.LookupCommand(ctx, c.Project, requestID); lookupErr != nil {
			return lookupErr
		} else if found {
			replayed, replayErr := replayNativeStopReceipt(old, request, c.Project, taskID, requestedBy)
			if replayErr != nil {
				return replayErr
			}
			result = replayed
			return commandReplayed{Status: "accepted"}
		}
		oldTask, err := tx.Task(ctx, c.Project, taskID)
		if err != nil {
			if errors.Is(err, storageports.ErrNotFound) {
				return nativeErr("task_not_found", "discarded native task was not found")
			}
			return err
		}
		if oldTask.Source != "native" || oldTask.Status != "discarded" || oldTask.Version != expectedVersion {
			return nativeErr("stale_version", "stop receipt requires the current discarded task version")
		}
		index := -1
		for i, pending := range oldTask.StopPending {
			if pending == stopItem {
				index = i
				break
			}
		}
		if index < 0 {
			return nativeErr("stale_version", "stop item is no longer pending for this task")
		}
		if !strings.HasPrefix(stopItem, "execution_stop_manual:") {
			return nativeErr("contract_mismatch", "unsupported execution stop identity")
		}
		if err := commitNativeExecutionStopReceipt(ctx, tx, c.Project, oldTask, stopItem, receipt); err != nil {
			return err
		}
		authority, err := tx.NativeAuthority(ctx, c.Project)
		if err != nil || authority.Authority != "native" {
			return nativeErr("authority_mismatch", "stop receipt requires the native project authority")
		}
		oldTask.StopPending = append(oldTask.StopPending[:index], oldTask.StopPending[index+1:]...)
		oldTask.Version++
		oldTask.UpdatedAt = nowAt
		newPayload, err := json.Marshal(oldTask)
		if err != nil {
			return err
		}
		if err = tx.UpdateDiscardedTaskCAS(oldTask, expectedVersion); err != nil {
			return err
		}
		operation := executionmodel.OperationRecord{ProjectIdentity: c.Project, OperationID: requestID, IdempotencyKey: requestID, Kind: "native_execution_stop", Status: "confirmed", PayloadDigest: operationDigest(request), Request: request, Receipt: append([]byte(nil), receipt...), CreatedAt: nowAt, UpdatedAt: nowAt}
		if _, made, err := tx.PutOperation(operation); err != nil {
			return err
		} else if !made {
			return nativeErr("idempotency_conflict", "stop request ID is already in use")
		}
		command := storageports.CommandRecord{ProjectIdentity: c.Project, CommandID: requestID, Kind: "record_execution_stop", TargetID: taskID, ExpectedVersion: expectedVersion, IdempotencyKey: requestID, RequestedBy: requestedBy, Payload: request, Result: newPayload, Status: "accepted", CreatedAt: nowAt, UpdatedAt: nowAt}
		if _, made, err := tx.EnqueueCommand(command); err != nil {
			return nativeErr("idempotency_conflict", err.Error())
		} else if !made {
			return nativeErr("idempotency_conflict", "stop command request ID is already in use")
		}
		if _, err := tx.AdvanceTaskGraphRevision(c.Project, authority.AuthorityVersion, authority.GraphRevision); err != nil {
			return err
		}
		body, _ := json.Marshal(map[string]any{"task_id": taskID, "stop_item": stopItem, "request_id": requestID, "receipt": receipt})
		if _, err := tx.AppendEvent(storageports.DomainEvent{ProjectIdentity: c.Project, TaskID: taskID, ExternalSource: "native-task-stop", ExternalID: requestID, Type: "task.execution_stop.confirmed", ReceivedAt: nowAt, Payload: body}); err != nil {
			return err
		}
		result = oldTask
		return nil
	})
	var replay commandReplayed
	if errors.As(err, &replay) {
		return result, nil
	}
	return result, err
}

func commitNativeExecutionStopReceipt(ctx context.Context, tx storageports.StopReceiptTx, project string, task taskmodel.TaskRecord, stopItem string, receipt json.RawMessage) error {
	var evidence NativeExecutionStopReceipt
	if err := decode(receipt, &evidence); err != nil || evidence.ExecutionID == "" || stopItem != "execution_stop_manual:"+evidence.ExecutionID || evidence.WorkflowRunID == "" || evidence.AttemptID == "" {
		return nativeErr("contract_mismatch", "native stop receipt does not match the pending execution identity")
	}
	op := evidence.Operation
	if executionbackend.ValidateExecutionOperation(op) != nil || op.Status != executionbackend.OperationConfirmed || op.Action != "stop" || op.RunID != evidence.WorkflowRunID || op.AttemptID != evidence.AttemptID || op.Ref == nil || op.Ref.RunID != op.RunID || op.Ref.AttemptID != op.AttemptID {
		return nativeErr("contract_mismatch", "native stop receipt lacks a confirmed fixed worker identity")
	}
	if evidence.Observation.Liveness != "exited" || evidence.Observation.ObservedAt.IsZero() || evidence.Observation.Source == "" {
		return nativeErr("contract_mismatch", "native stop receipt has no confirmed process-absent observation")
	}
	workflow, err := tx.Workflow(ctx, project, evidence.WorkflowRunID, task.TaskID)
	if err != nil {
		return nativeErr("contract_mismatch", "native stop receipt workflow does not belong to the discarded task")
	}
	execution, err := tx.Execution(ctx, project, evidence.ExecutionID, evidence.WorkflowRunID)
	if err != nil {
		return nativeErr("contract_mismatch", "native stop receipt execution is not bound to the discarded workflow")
	}
	if workflow.TaskID != task.TaskID || execution.ExecutionID != evidence.ExecutionID || execution.WorkflowRunID != evidence.WorkflowRunID || execution.Process == nil || *execution.Process != op.Ref.External || execution.Target != op.Ref.Target || execution.CurrentAttemptID != evidence.AttemptID || execution.Agent != op.Ref.Agent.Name || execution.SessionGeneration != op.Ref.Generation || execution.Workspace.Worktree != op.Ref.WorkspacePath || execution.Workspace.Branch != op.Ref.WorkspaceBranch || execution.Workspace.Base != op.Ref.BaseCommit || execution.Backend != string(op.Backend) || execution.Protocol != string(op.Protocol) {
		return nativeErr("authority_mismatch", "native stop receipt does not match the center's fixed execution identity")
	}
	attempt, err := tx.Attempt(ctx, project, evidence.AttemptID, evidence.ExecutionID)
	if err != nil {
		return nativeErr("contract_mismatch", "native stop receipt attempt is not bound to the fixed execution")
	}
	if attempt.Status != "succeeded" && attempt.Status != "stopped" && attempt.Status != "failed" && attempt.Status != "rejected" {
		attempt.Status, attempt.UpdatedAt = "stopped", now()
		if err := tx.PutStepAttempt(attempt); err != nil {
			return err
		}
	}
	execution.ResourceStopOperationID, execution.ResourceStoppedAt = op.ID, ptr(evidence.Observation.ObservedAt)
	execution.Status, execution.UpdatedAt = "stopped", now()
	if err := tx.PutExecution(execution); err != nil {
		return err
	}
	// Resource stop does not rewrite the candidate's experiment/phase history.
	return nil
}

func replayNativeStopReceipt(old storageports.CommandRecord, request []byte, project, taskID, requestedBy string) (taskmodel.TaskRecord, error) {
	if old.ProjectIdentity != project || old.Kind != "record_execution_stop" || old.TargetID != taskID || old.RequestedBy != requestedBy || old.Status != "accepted" || !bytes.Equal(old.Payload, request) || len(old.Result) == 0 {
		return taskmodel.TaskRecord{}, nativeErr("idempotency_conflict", "stop request ID was used for different task evidence")
	}
	var task taskmodel.TaskRecord
	if err := decode(old.Result, &task); err != nil || task.TaskID != taskID || task.Status != "discarded" {
		return taskmodel.TaskRecord{}, nativeErr("unsupported_schema", "saved native stop receipt is invalid")
	}
	return task, nil
}
