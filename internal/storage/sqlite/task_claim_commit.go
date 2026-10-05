package sqlite

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func (s *SQLiteStore) CommitTaskClaim(ctx context.Context, commit storageports.ClaimCommit) (executionmodel.TaskClaimReceipt, error) {
	if commit.Command.ProjectIdentity != commit.Before.ProjectIdentity || commit.Operation.ProjectIdentity != commit.Before.ProjectIdentity || commit.Command.IdempotencyKey != commit.Command.CommandID || commit.Operation.IdempotencyKey != commit.Command.CommandID || commit.Operation.Kind != commit.Command.Kind || commit.Operation.PayloadDigest != operationDigest(commit.Operation.Request) || commit.Candidate && (commit.Command.Kind != "start_candidate" || !commit.Explicit || commit.Before.Status != "in_progress") || !commit.Candidate && (commit.Command.Kind != "claim_task" || commit.Before.Status != "open") {
		return executionmodel.TaskClaimReceipt{}, nativeErr("contract_mismatch", "claim must preserve the original request, project and reservation kind")
	}
	var receipt executionmodel.TaskClaimReceipt
	err := s.transactSQLite(ctx, func(tx *sqliteTx) error {
		if old, found, e := tx.LookupCommand(ctx, commit.Before.ProjectIdentity, commit.Command.CommandID); e != nil {
			return e
		} else if found {
			r, e := replayClaimCommit(old, commit.Command)
			if e != nil {
				return e
			}
			receipt = r
			return errAtomicReplay
		}
		current, e := tx.TaskGraphSnapshot(ctx, commit.Before.ProjectIdentity)
		if e != nil {
			return e
		}
		currentTask, ok := current.ByID[commit.Before.TaskID]
		if !ok {
			return nativeErr("task_not_found", "native task was not found")
		}
		if current.Authority.AuthorityVersion != commit.AuthorityVersion || current.Authority.GraphRevision != commit.GraphRevision || currentTask.Version != commit.Before.Version || currentTask.Status != commit.Before.Status {
			return nativeErr("stale_version", "task or project authority changed during claim preflight")
		}
		if taskmodel.DeliveryInconsistent(currentTask.Status, current.Deliveries[currentTask.TaskID].TaskID != "") {
			return nativeErr("inconsistent_delivery", "task state conflicts with its delivery record")
		}
		if current.Authority.DispatchPaused {
			return nativeErr("dispatch_paused", "dispatch was paused before claim")
		}
		if !commit.Candidate && (commit.Explicit && current.Authority.DispatchMode != "manual" || !commit.Explicit && current.Authority.DispatchMode != "auto") {
			return nativeErr("wrong_mode", "project dispatch mode changed before claim")
		}
		if commit.Candidate {
			stored, e := tx.ReadTaskCandidateSnapshot(ctx, commit.Before.ProjectIdentity, commit.Before.TaskID)
			if e != nil {
				return e
			}
			if executionmodel.CandidatePRBindings(stored.WorkflowRuns, commit.Before.TaskID) != commit.ExpectedPRBindings {
				return nativeErr("stale_version", "known PR bindings changed during start verification")
			}
			storedBase, err := executionmodel.CandidateCommonBase(stored.WorkflowRuns, stored.Executions, commit.Before.TaskID)
			if err != nil {
				return err
			}
			if storedBase != commit.Execution.Workspace.Base {
				return nativeErr("contract_mismatch", "experiment common base changed during start verification")
			}
		}
		currentEligibility := currentTask
		if commit.Candidate {
			currentEligibility.Status = "open"
		}
		ready, blockers := taskmodel.DependencyStateForTask(currentEligibility, current.TaskGraph())
		if !ready {
			return taskmodel.DependencyClaimError(blockers)
		}
		for pred, merge := range commit.PredecessorMerges {
			if current.Deliveries[pred].MergeCommit != merge {
				return nativeErr("base_missing_delivery", "predecessor delivery identity changed during claim preflight")
			}
		}
		if !sameTaskRecord(currentTask, commit.Before) {
			return nativeErr("stale_version", "claim task before value changed")
		}
		contract, e := tx.TaskContract(ctx, commit.Before.ProjectIdentity, currentTask)
		if e != nil {
			return e
		}
		if digestValue(contract) != digestValue(commit.Contract) || commit.Attempt.ContractDigest != contract.Digest {
			return nativeErr("contract_mismatch", "claim fixed contract changed")
		}
		if commit.Candidate {
			if !sameTaskRecord(commit.Before, commit.After) {
				return nativeErr("contract_mismatch", "candidate cannot reclaim a task")
			}
		} else {
			expected, e := taskmodel.Claim(commit.Before, commit.After.Claimant, commit.After.UpdatedAt)
			if e != nil {
				return e
			}
			if !sameTaskRecord(expected, commit.After) {
				return nativeErr("contract_mismatch", "claim after value is not the declared task claim")
			}
		}
		if commit.Workflow.ProjectIdentity != commit.Before.ProjectIdentity || commit.Workflow.TaskID != commit.Before.TaskID || commit.Workflow.SelectedResult != commit.Execution.ExecutionID || commit.Execution.ProjectIdentity != commit.Before.ProjectIdentity || commit.Execution.WorkflowRunID != commit.Workflow.WorkflowRunID || commit.Execution.CurrentAttemptID != commit.Attempt.AttemptID || commit.Attempt.ProjectIdentity != commit.Before.ProjectIdentity || commit.Attempt.ExecutionID != commit.Execution.ExecutionID || commit.Command.TargetID != commit.Before.TaskID || commit.Command.ExpectedVersion != commit.Before.Version || commit.Operation.OperationID != commit.Command.CommandID {
			return nativeErr("contract_mismatch", "claim declarations have inconsistent identity")
		}
		occupied, e := tx.OccupiedExecutions(ctx, commit.Before.ProjectIdentity)
		if e != nil {
			return e
		}
		if occupied > 0 && !commit.Explicit {
			return nativeErr("slot_busy", "native execution slot is occupied or uncertain")
		}
		command := commit.Command
		var made bool
		command, made, e = tx.EnqueueCommand(command)
		if e != nil {
			return nativeErr("idempotency_conflict", e.Error())
		}
		if !made {
			return nativeErr("idempotency_conflict", "claim request ID is already in use")
		}
		operation := commit.Operation
		if _, made, e = tx.PutOperation(operation); e != nil {
			return e
		} else if !made {
			return nativeErr("idempotency_conflict", "claim operation request ID is already in use")
		}
		if !commit.Candidate {
			if e = tx.UpdateNativeTaskCAS(commit.After, commit.Before.Version); e != nil {
				return e
			}
		}
		if e = tx.PutWorkflowRun(commit.Workflow); e != nil {
			return e
		}
		if e = tx.PutExecution(commit.Execution); e != nil {
			return e
		}
		if e = tx.PutStepAttempt(commit.Attempt); e != nil {
			return e
		}
		receipt = executionmodel.TaskClaimReceipt{RequestID: commit.Command.CommandID, TaskID: commit.Before.TaskID, TaskVersion: commit.After.Version, WorkflowRunID: commit.Workflow.WorkflowRunID, ExecutionID: commit.Execution.ExecutionID, AttemptID: commit.Attempt.AttemptID, BaseCommit: commit.Execution.Workspace.Base, Status: "claim_reserved", StartTaskApplied: false}
		command.Result, _ = json.Marshal(receipt)
		command.Status = "accepted"
		if e = tx.UpdateCommand(command); e != nil {
			return e
		}
		body, _ := json.Marshal(receipt)
		eventType := "task.claim.reserved"
		if commit.Candidate {
			eventType = "candidate.start.reserved"
		}
		_, e = tx.AppendEvent(storageports.DomainEvent{ProjectIdentity: commit.Before.ProjectIdentity, TaskID: commit.Before.TaskID, WorkflowRunID: commit.Workflow.WorkflowRunID, ExecutionID: commit.Execution.ExecutionID, AttemptID: commit.Attempt.AttemptID, ExternalSource: "native-task-command", ExternalID: commit.Command.CommandID, Type: eventType, ReceivedAt: commit.Workflow.CreatedAt, Payload: body})
		return e
	})
	if err != nil && !errors.Is(err, errAtomicReplay) {
		return executionmodel.TaskClaimReceipt{}, err
	}
	return receipt, nil
}

func replayClaimCommit(old, request storageports.CommandRecord) (executionmodel.TaskClaimReceipt, error) {
	if old.Kind != request.Kind || old.TargetID != request.TargetID || old.RequestedBy != request.RequestedBy || !bytes.Equal(old.Payload, request.Payload) || old.Status != "accepted" || len(old.Result) == 0 {
		return executionmodel.TaskClaimReceipt{}, nativeErr("idempotency_conflict", "claim request content differs or has no committed receipt")
	}
	var receipt executionmodel.TaskClaimReceipt
	if err := json.Unmarshal(old.Result, &receipt); err != nil {
		return receipt, nativeErr("unsupported_schema", "claim receipt is unreadable")
	}
	return receipt, nil
}
