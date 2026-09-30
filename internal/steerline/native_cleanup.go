package steerline

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

type NativeStopCleanupReceipt struct {
	WorkflowRunID string                   `json:"workflow_run_id"`
	ExecutionID   string                   `json:"execution_id"`
	AttemptID     string                   `json:"attempt_id"`
	Operation     executionapi.Operation   `json:"operation"`
	Observation   executionapi.Observation `json:"observation"`
}

type NativePRCleanupReceipt struct {
	WorkflowRunID string    `json:"workflow_run_id"`
	ExecutionID   string    `json:"execution_id"`
	AttemptID     string    `json:"attempt_id"`
	PRURL         string    `json:"pr_url"`
	Repository    string    `json:"repository"`
	BaseBranch    string    `json:"base_branch"`
	HeadRevision  string    `json:"head_revision"`
	State         string    `json:"state"`
	Merged        bool      `json:"merged"`
	ObservedAt    time.Time `json:"observed_at"`
}

type NativeWorkspaceCleanupReceipt struct {
	WorkflowRunID string    `json:"workflow_run_id"`
	ExecutionID   string    `json:"execution_id"`
	Repository    string    `json:"repository"`
	Worktree      string    `json:"worktree"`
	Branch        string    `json:"branch"`
	BaseCommit    string    `json:"base_commit"`
	WorktreeGone  bool      `json:"worktree_absent"`
	BranchGone    bool      `json:"branch_absent"`
	ObservedAt    time.Time `json:"observed_at"`
}

// RecordNativeCleanupReceipt settles one exact cleanup item from a discarded
// native task. It never changes task identity, discarded status, or readiness.
func (c CommandService) RecordNativeCleanupReceipt(ctx context.Context, taskID, requestID string, expectedVersion uint64, cleanupItem, requestedBy string, receipt json.RawMessage) (TaskRecord, error) {
	sqlite := c.Store
	if sqlite == nil || c.Project == "" || !controlIDPattern.MatchString(requestID) || expectedVersion == 0 || cleanupItem == "" || !json.Valid(receipt) {
		return TaskRecord{}, nativeErr("contract_mismatch", "native cleanup receipt requires project, request, version, cleanup identity and JSON evidence")
	}
	if err := ValidateNativeTaskID(taskID); err != nil {
		return TaskRecord{}, err
	}
	if strings.TrimSpace(requestedBy) == "" {
		requestedBy = DefaultRequestedBy()
	}
	request, err := CanonicalJSON(mustJSON(struct {
		TaskID      string          `json:"task_id"`
		CleanupItem string          `json:"cleanup_item"`
		RequestedBy string          `json:"requested_by"`
		Receipt     json.RawMessage `json:"receipt"`
	}{taskID, cleanupItem, requestedBy, receipt}))
	if err != nil {
		return TaskRecord{}, err
	}
	if old, found, lookupErr := sqlite.lookupNativeCommand(ctx, c.Project, requestID); lookupErr != nil {
		return TaskRecord{}, lookupErr
	} else if found {
		return replayNativeCleanupReceipt(old, request, c.Project, taskID, requestedBy)
	}
	nowAt := time.Now().UTC()
	var result TaskRecord
	err = sqlite.transactSQLite(ctx, func(tx *sqliteTx) error {
		if old, found, lookupErr := tx.lookupNativeCommand(ctx, c.Project, requestID); lookupErr != nil {
			return lookupErr
		} else if found {
			replayed, replayErr := replayNativeCleanupReceipt(old, request, c.Project, taskID, requestedBy)
			if replayErr != nil {
				return replayErr
			}
			result = replayed
			return taskBatchReplay{Receipt: TaskBatchReceipt{RequestID: requestID, Status: "accepted"}}
		}
		var oldPayload []byte
		if err := tx.tx.QueryRowContext(ctx, `SELECT payload FROM tasks WHERE project_identity=? AND task_id=?`, c.Project, taskID).Scan(&oldPayload); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nativeErr("task_not_found", "discarded native task was not found")
			}
			return err
		}
		var oldTask TaskRecord
		if err := json.Unmarshal(oldPayload, &oldTask); err != nil {
			return err
		}
		if oldTask.Source != "native" || oldTask.Status != "discarded" || oldTask.Version != expectedVersion {
			return nativeErr("stale_version", "cleanup receipt requires the current discarded task version")
		}
		index := -1
		for i, pending := range oldTask.CleanupPending {
			if pending == cleanupItem {
				index = i
				break
			}
		}
		if index < 0 {
			return nativeErr("stale_version", "cleanup item is no longer pending for this task")
		}
		if strings.HasPrefix(cleanupItem, "execution_stop_manual:") {
			if err := commitNativeExecutionStopReceipt(ctx, tx, c.Project, oldTask, cleanupItem, receipt); err != nil {
				return err
			}
		} else if strings.HasPrefix(cleanupItem, "pull_request_close:") {
			if err := commitNativePRCleanupReceipt(ctx, tx, c.Project, oldTask, cleanupItem, receipt); err != nil {
				return err
			}
		} else if strings.HasPrefix(cleanupItem, "workspace_cleanup:") {
			for _, pending := range oldTask.CleanupPending {
				if strings.HasPrefix(pending, "execution_stop_manual:") {
					return nativeErr("slot_busy", "workspace cleanup cannot settle while worker stop is pending")
				}
			}
			if err := commitNativeWorkspaceCleanupReceipt(ctx, tx, c.Project, oldTask, cleanupItem, receipt); err != nil {
				return err
			}
		} else {
			return nativeErr("contract_mismatch", "unsupported native cleanup item")
		}
		var authority ProjectTaskAuthority
		var paused int
		var created, updatedAt string
		if err := tx.tx.QueryRowContext(ctx, `SELECT authority,authority_version,graph_revision,repository,remote,branch,dispatch_mode,dispatch_paused,created_at,updated_at FROM project_task_authorities WHERE project_identity=?`, c.Project).Scan(&authority.Authority, &authority.AuthorityVersion, &authority.GraphRevision, &authority.Repository, &authority.Remote, &authority.Branch, &authority.DispatchMode, &paused, &created, &updatedAt); err != nil {
			return nativeErr("authority_mismatch", "native project authority is unavailable during cleanup")
		}
		authority.ProjectIdentity = c.Project
		if authority.Authority != "native" {
			return nativeErr("authority_mismatch", "cleanup receipt requires the native project authority")
		}
		authority.DispatchPaused = paused != 0
		authority.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		authority.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
		oldTask.CleanupPending = append(oldTask.CleanupPending[:index], oldTask.CleanupPending[index+1:]...)
		oldTask.Version++
		oldTask.UpdatedAt = nowAt
		newPayload, err := json.Marshal(oldTask)
		if err != nil {
			return err
		}
		updated, err := tx.tx.ExecContext(ctx, `UPDATE tasks SET version=?,payload=? WHERE project_identity=? AND task_id=? AND status='discarded' AND version=?`, oldTask.Version, newPayload, c.Project, taskID, expectedVersion)
		if err = requireIdentityWrite(updated, err, "stale_version: discarded task changed during cleanup receipt"); err != nil {
			return err
		}
		operation := OperationRecord{ProjectIdentity: c.Project, OperationID: requestID, IdempotencyKey: requestID, Kind: "native_cleanup", Status: "confirmed", PayloadDigest: digestCanonical(request), Request: request, Receipt: append([]byte(nil), receipt...), CreatedAt: nowAt, UpdatedAt: nowAt}
		if _, made, err := tx.PutOperation(operation); err != nil {
			return err
		} else if !made {
			return nativeErr("idempotency_conflict", "cleanup request ID is already in use")
		}
		command := CommandRecord{ProjectIdentity: c.Project, CommandID: requestID, Kind: "record_task_cleanup", TargetID: taskID, ExpectedVersion: expectedVersion, IdempotencyKey: requestID, RequestedBy: requestedBy, Payload: request, Result: newPayload, Status: "accepted", CreatedAt: nowAt, UpdatedAt: nowAt}
		if _, made, err := tx.EnqueueCommand(command); err != nil {
			return nativeErr("idempotency_conflict", err.Error())
		} else if !made {
			return nativeErr("idempotency_conflict", "cleanup command request ID is already in use")
		}
		if _, err := tx.AdvanceTaskGraphRevision(c.Project, authority.AuthorityVersion, authority.GraphRevision); err != nil {
			return err
		}
		body, _ := json.Marshal(map[string]any{"task_id": taskID, "cleanup_item": cleanupItem, "request_id": requestID, "receipt": receipt})
		if _, err := tx.AppendEvent(DomainEvent{ProjectIdentity: c.Project, TaskID: taskID, ExternalSource: "native-task-cleanup", ExternalID: requestID, Type: "task.cleanup.confirmed", ReceivedAt: nowAt, Payload: body}); err != nil {
			return err
		}
		result = oldTask
		return nil
	})
	var replay taskBatchReplay
	if errors.As(err, &replay) {
		return result, nil
	}
	return result, err
}

func commitNativePRCleanupReceipt(ctx context.Context, tx *sqliteTx, project string, task TaskRecord, cleanupItem string, receipt json.RawMessage) error {
	var evidence NativePRCleanupReceipt
	if err := decode(receipt, &evidence); err != nil || evidence.PRURL == "" || cleanupItem != "pull_request_close:"+evidence.PRURL || evidence.WorkflowRunID == "" || evidence.ExecutionID == "" || evidence.AttemptID == "" || evidence.State != "closed_unmerged" || evidence.Merged || evidence.ObservedAt.IsZero() || !validCommit(evidence.HeadRevision) {
		return nativeErr("contract_mismatch", "native PR cleanup receipt does not prove the exact unmerged PR was closed")
	}
	if evidence.Repository == "" || evidence.BaseBranch == "" {
		return nativeErr("contract_mismatch", "native PR cleanup receipt lacks repository and branch identity")
	}
	var workflowPayload, executionPayload, attemptPayload []byte
	if err := tx.tx.QueryRowContext(ctx, `SELECT payload FROM workflow_runs WHERE project_identity=? AND workflow_run_id=? AND task_id=?`, project, evidence.WorkflowRunID, task.TaskID).Scan(&workflowPayload); err != nil {
		return nativeErr("contract_mismatch", "native PR cleanup workflow does not belong to the discarded task")
	}
	if err := tx.tx.QueryRowContext(ctx, `SELECT payload FROM executions WHERE project_identity=? AND execution_id=? AND workflow_run_id=?`, project, evidence.ExecutionID, evidence.WorkflowRunID).Scan(&executionPayload); err != nil {
		return nativeErr("contract_mismatch", "native PR cleanup execution is not bound to the fixed workflow")
	}
	if err := tx.tx.QueryRowContext(ctx, `SELECT payload FROM step_attempts WHERE project_identity=? AND attempt_id=? AND execution_id=? AND step_id='worker'`, project, evidence.AttemptID, evidence.ExecutionID).Scan(&attemptPayload); err != nil {
		return nativeErr("contract_mismatch", "native PR cleanup attempt is not the fixed worker attempt")
	}
	var workflow WorkflowRunRecord
	var execution ExecutionRecord
	var attempt StepAttemptRecord
	if err := json.Unmarshal(workflowPayload, &workflow); err != nil {
		return err
	}
	if err := json.Unmarshal(executionPayload, &execution); err != nil {
		return err
	}
	if err := json.Unmarshal(attemptPayload, &attempt); err != nil {
		return err
	}
	if workflow.WorkflowVersion != WorkerFlowVersion || workflow.TaskID != task.TaskID || workflow.Repository != evidence.Repository || workflow.Branch != evidence.BaseBranch || workflow.SelectedResult != evidence.ExecutionID || workflow.PRURL != evidence.PRURL || execution.ExecutionID != evidence.ExecutionID || execution.WorkflowRunID != evidence.WorkflowRunID || execution.SessionID == "" || execution.Workspace.Branch == "" || attempt.StepID != "worker" || attempt.ExecutionID != evidence.ExecutionID || attempt.AttemptID != evidence.AttemptID {
		return nativeErr("authority_mismatch", "native PR cleanup receipt does not match the fixed worker flow and PR")
	}
	var authorityBranch string
	if err := tx.tx.QueryRowContext(ctx, `SELECT branch FROM project_task_authorities WHERE project_identity=? AND authority='native'`, project).Scan(&authorityBranch); err != nil || authorityBranch != evidence.BaseBranch {
		return nativeErr("authority_mismatch", "native PR cleanup branch differs from the registered project")
	}
	return nil
}

func commitNativeWorkspaceCleanupReceipt(ctx context.Context, tx *sqliteTx, project string, task TaskRecord, cleanupItem string, receipt json.RawMessage) error {
	var evidence NativeWorkspaceCleanupReceipt
	if err := decode(receipt, &evidence); err != nil || evidence.WorkflowRunID == "" || evidence.ExecutionID == "" || evidence.Repository == "" || evidence.Branch == "" || !filepath.IsAbs(evidence.Worktree) || !strings.HasPrefix(cleanupItem, "workspace_cleanup:") || canonical(strings.TrimPrefix(cleanupItem, "workspace_cleanup:")) != canonical(evidence.Worktree) || !validCommit(evidence.BaseCommit) || !evidence.WorktreeGone || !evidence.BranchGone || evidence.ObservedAt.IsZero() {
		return nativeErr("contract_mismatch", "native workspace cleanup receipt lacks the exact absent worktree and branch identities")
	}
	var workflowPayload, executionPayload []byte
	if err := tx.tx.QueryRowContext(ctx, `SELECT payload FROM workflow_runs WHERE project_identity=? AND workflow_run_id=? AND task_id=?`, project, evidence.WorkflowRunID, task.TaskID).Scan(&workflowPayload); err != nil {
		return nativeErr("contract_mismatch", "native workspace cleanup workflow does not belong to the discarded task")
	}
	if err := tx.tx.QueryRowContext(ctx, `SELECT payload FROM executions WHERE project_identity=? AND execution_id=? AND workflow_run_id=?`, project, evidence.ExecutionID, evidence.WorkflowRunID).Scan(&executionPayload); err != nil {
		return nativeErr("contract_mismatch", "native workspace cleanup execution is not bound to the fixed workflow")
	}
	var workflow WorkflowRunRecord
	var execution ExecutionRecord
	if err := json.Unmarshal(workflowPayload, &workflow); err != nil {
		return err
	}
	if err := json.Unmarshal(executionPayload, &execution); err != nil {
		return err
	}
	fixedWorktree := execution.Target.WorktreeID
	if fixedWorktree == "" {
		fixedWorktree = execution.Workspace.Worktree
	}
	if workflow.TaskID != task.TaskID || execution.WorkflowRunID != evidence.WorkflowRunID || execution.Backend != string(executionapi.BackendLocal) || execution.Target.ID != evidence.Repository || fixedWorktree == "" || canonical(fixedWorktree) != canonical(evidence.Worktree) || execution.Workspace.Worktree != "" && canonical(execution.Workspace.Worktree) != canonical(evidence.Worktree) || execution.Target.Branch != evidence.Branch || execution.Target.BaseRevision != evidence.BaseCommit {
		return nativeErr("authority_mismatch", "native workspace cleanup receipt differs from the fixed local execution")
	}
	if _, err := os.Lstat(evidence.Worktree); !os.IsNotExist(err) {
		return nativeErr("outcome_unknown", "native workspace cleanup cannot confirm the fixed worktree is absent")
	}
	return nil
}

func commitNativeExecutionStopReceipt(ctx context.Context, tx *sqliteTx, project string, task TaskRecord, cleanupItem string, receipt json.RawMessage) error {
	var evidence NativeStopCleanupReceipt
	if err := decode(receipt, &evidence); err != nil || evidence.ExecutionID == "" || cleanupItem != "execution_stop_manual:"+evidence.ExecutionID || evidence.WorkflowRunID == "" || evidence.AttemptID == "" {
		return nativeErr("contract_mismatch", "native stop receipt does not match the pending execution identity")
	}
	op := evidence.Operation
	if op.Status != executionapi.OperationConfirmed || op.Action != "stop" || op.RunID != evidence.WorkflowRunID || op.AttemptID != evidence.AttemptID || op.Ref == nil || op.Ref.RunID != op.RunID || op.Ref.AttemptID != op.AttemptID || op.Ref.External.Kind != "os" {
		return nativeErr("contract_mismatch", "native stop receipt lacks a confirmed fixed local worker identity")
	}
	if evidence.Observation.Liveness != "exited" || evidence.Observation.ObservedAt.IsZero() || evidence.Observation.Source == "" {
		return nativeErr("contract_mismatch", "native stop receipt has no confirmed process-absent observation")
	}
	var workflowPayload, executionPayload []byte
	if err := tx.tx.QueryRowContext(ctx, `SELECT payload FROM workflow_runs WHERE project_identity=? AND workflow_run_id=? AND task_id=?`, project, evidence.WorkflowRunID, task.TaskID).Scan(&workflowPayload); err != nil {
		return nativeErr("contract_mismatch", "native stop receipt workflow does not belong to the discarded task")
	}
	if err := tx.tx.QueryRowContext(ctx, `SELECT payload FROM executions WHERE project_identity=? AND execution_id=? AND workflow_run_id=?`, project, evidence.ExecutionID, evidence.WorkflowRunID).Scan(&executionPayload); err != nil {
		return nativeErr("contract_mismatch", "native stop receipt execution is not bound to the discarded workflow")
	}
	var workflow WorkflowRunRecord
	var execution ExecutionRecord
	if err := json.Unmarshal(workflowPayload, &workflow); err != nil {
		return err
	}
	if err := json.Unmarshal(executionPayload, &execution); err != nil {
		return err
	}
	if workflow.TaskID != task.TaskID || execution.ExecutionID != evidence.ExecutionID || execution.WorkflowRunID != evidence.WorkflowRunID || execution.Process == nil || *execution.Process != op.Ref.External || execution.Target.ID != op.Ref.Target.ID || execution.Backend != string(executionapi.BackendLocal) || execution.Protocol != string(op.Protocol) || (op.Protocol != executionapi.ProtocolLocalProcess && op.Protocol != executionapi.ProtocolLocalCodexAppServer) {
		return nativeErr("authority_mismatch", "native stop receipt does not match the center's fixed execution identity")
	}
	var attemptPayload []byte
	if err := tx.tx.QueryRowContext(ctx, `SELECT payload FROM step_attempts WHERE project_identity=? AND attempt_id=? AND execution_id=?`, project, evidence.AttemptID, evidence.ExecutionID).Scan(&attemptPayload); err != nil {
		return nativeErr("contract_mismatch", "native stop receipt attempt is not bound to the fixed execution")
	}
	var attempt StepAttemptRecord
	if err := json.Unmarshal(attemptPayload, &attempt); err != nil {
		return err
	}
	if attempt.Status == "succeeded" || attempt.Status == "stopped" || attempt.Status == "failed" || attempt.Status == "rejected" {
		return nativeErr("stale_version", "native stop receipt targets a non-active step attempt")
	}
	attempt.Status, attempt.UpdatedAt = "stopped", now()
	if err := tx.PutStepAttempt(attempt); err != nil {
		return err
	}
	execution.Status, execution.UpdatedAt = "stopped", now()
	if err := tx.PutExecution(execution); err != nil {
		return err
	}
	workflow.Phase, workflow.UpdatedAt = "blocked", now()
	return tx.PutWorkflowRun(workflow)
}

func replayNativeCleanupReceipt(old CommandRecord, request []byte, project, taskID, requestedBy string) (TaskRecord, error) {
	if old.ProjectIdentity != project || old.Kind != "record_task_cleanup" || old.TargetID != taskID || old.RequestedBy != requestedBy || old.Status != "accepted" || !bytes.Equal(old.Payload, request) || len(old.Result) == 0 {
		return TaskRecord{}, nativeErr("idempotency_conflict", "cleanup request ID was used for different task evidence")
	}
	var task TaskRecord
	if err := decode(old.Result, &task); err != nil || task.TaskID != taskID || task.Status != "discarded" {
		return TaskRecord{}, nativeErr("unsupported_schema", "saved native cleanup receipt is invalid")
	}
	return task, nil
}
