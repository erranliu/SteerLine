package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func (s *SQLiteStore) CommitTaskResult(ctx context.Context, commit storageports.ResultCommit) (storageports.ResultCommitReceipt, error) {
	input := commit.Task.Input
	if commit.Workflow.ProjectIdentity != input.ProjectIdentity || commit.Workflow.TaskID != input.TaskID || commit.Execution.ProjectIdentity != input.ProjectIdentity || commit.Execution.WorkflowRunID != commit.Workflow.WorkflowRunID || commit.Workflow.SelectedResult != commit.Execution.ExecutionID {
		return storageports.ResultCommitReceipt{}, nativeErr("contract_mismatch", "result must preserve its original execution and workflow identity")
	}
	if input.ExpectedSetVersion == "" {
		return storageports.ResultCommitReceipt{}, nativeErr("merge_state_unknown", "delivery requires its verified candidate and PR binding set")
	}
	var receipt taskmodel.ResultReceipt
	var delivery executionmodel.TaskDeliveryRecord
	err := s.transactSQLite(ctx, func(tx *sqliteTx) error {
		graph, err := tx.TaskGraphSnapshot(ctx, input.ProjectIdentity)
		if err != nil {
			return err
		}
		if err := validateNativeTaskDelivery(commit.Delivery, graph.Authority.Branch); err != nil {
			return err
		}
		if saved, found, err := tx.taskResult(ctx, input.ProjectIdentity, input.ResultID); err != nil {
			return err
		} else if found {
			if digestValue(saved.Input) != digestValue(input) {
				return nativeErr("idempotency_conflict", "result ID was reused with different content")
			}
			current, err := tx.Task(ctx, input.ProjectIdentity, input.TaskID)
			if err != nil {
				return err
			}
			if current.Status != "closed" || current.Version != saved.TaskVersion {
				return nativeErr("inconsistent_delivery", "saved result lost its closed task")
			}
			original, found := graph.Deliveries[input.TaskID]
			candidate := commit.Delivery
			candidate.RecordedAt = original.RecordedAt
			if !found || digestValue(original) != digestValue(candidate) {
				return nativeErr("idempotency_conflict", "result association differs from original delivery")
			}
			delivery = original
			receipt = saved
			return errAtomicReplay
		}
		if graph.Authority.AuthorityVersion != commit.AuthorityVersion || graph.Authority.GraphRevision != input.ExpectedGraphRevision {
			return nativeErr("stale_version", "project or graph changed during verification")
		}
		current, ok := graph.ByID[input.TaskID]
		if !ok {
			return nativeErr("task_not_found", "result task was not found")
		}
		if _, found := graph.Deliveries[input.TaskID]; found {
			return nativeErr("inconsistent_delivery", "task already has a different authoritative result")
		}
		if current.Status == "closed" {
			return nativeErr("inconsistent_delivery", "closed task has no delivery record")
		}
		if current.Status == "discarded" {
			return nativeErr("task_discarded", "discarded task cannot be completed")
		}
		if current.Status != "in_progress" || current.Version != input.ExpectedTaskVersion {
			return nativeErr("stale_version", "task state changed during PR verification")
		}
		currentWorkflow, err := tx.Workflow(ctx, input.ProjectIdentity, commit.Workflow.WorkflowRunID, input.TaskID)
		if err != nil {
			return nativeErr("immutable_task", "selected workflow run changed during PR verification")
		}
		if currentWorkflow.Abandonment != nil {
			return nativeErr("immutable_task", "abandoned candidate cannot deliver")
		}
		currentExecution, err := tx.Execution(ctx, input.ProjectIdentity, commit.Execution.ExecutionID, commit.Workflow.WorkflowRunID)
		if err != nil {
			return nativeErr("inconsistent_delivery", "selected execution changed during PR verification")
		}
		currentContract, err := tx.TaskContract(ctx, input.ProjectIdentity, current)
		if err != nil {
			return err
		}
		if currentContract.Digest != input.ContractDigest {
			return nativeErr("immutable_task", "fixed task contract changed during PR verification")
		}
		currentPRURL, err := validateResultGates(current, currentContract, currentWorkflow, currentExecution)
		if err != nil {
			return err
		}
		if currentExecution.Generation != commit.Execution.Generation || currentExecution.OwnerServiceInstanceID != commit.Execution.OwnerServiceInstanceID || currentExecution.ConfigurationDigest != commit.Execution.ConfigurationDigest || digestValue(currentExecution.Target) != digestValue(commit.Execution.Target) || currentExecution.Workspace.Worktree != commit.Execution.Workspace.Worktree || currentExecution.Workspace.Repository != commit.Execution.Workspace.Repository || currentExecution.Workspace.Base != commit.Execution.Workspace.Base || digestValue(currentWorkflow.ExecutionProfile) != digestValue(commit.Workflow.ExecutionProfile) || digestValue(currentExecution.PolicyResources) != digestValue(commit.Execution.PolicyResources) || currentWorkflow.WorkflowRunID != commit.Workflow.WorkflowRunID || currentWorkflow.SelectedResult != commit.Execution.ExecutionID || currentPRURL != commit.Delivery.PRURL ||
			currentWorkflow.WorkflowVersion != commit.Workflow.WorkflowVersion || digestValue(currentExecution.ImplementationData) != digestValue(commit.Execution.ImplementationData) ||
			currentExecution.Workspace.Branch != commit.Execution.Workspace.Branch {
			return nativeErr("immutable_task", "selected result or PR binding changed during platform verification")
		}
		currentSnapshot, err := tx.ReadTaskCandidateSnapshot(ctx, input.ProjectIdentity, input.TaskID)
		if err != nil {
			return err
		}
		if executionmodel.CandidateBindings(currentSnapshot.WorkflowRuns, currentSnapshot.Executions, input.TaskID) != input.ExpectedSetVersion {
			return nativeErr("stale_version", "candidate eligibility or known PR set changed during verification")
		}
		reused, err := tx.PRBindingsOnOtherTasks(ctx, input.ProjectIdentity, input.TaskID, commit.Delivery.PRURL)
		if err != nil {
			return err
		}
		if reused > 0 {
			return nativeErr("immutable_task", "PR is already bound to another native task")
		}
		if err = tx.PutTaskDelivery(commit.Delivery); err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "unique constraint failed: task_deliveries.pr_instance") {
				return nativeErr("immutable_task", "PR delivery identity is already bound to another task")
			}
			return err
		}
		declaration, err := taskmodel.DeclareResult(current, currentContract, input, commit.Task.After.UpdatedAt)
		if err != nil {
			return err
		}
		if !sameTaskRecord(commit.Task.Before, current) || !sameTaskRecord(declaration.After, commit.Task.After) {
			return nativeErr("stale_version", "task declaration differs from its original before/after")
		}
		if input.ResultID != commit.Workflow.WorkflowRunID || input.RequestID != "delivery:"+input.ResultID || commit.Delivery.ProjectIdentity != input.ProjectIdentity || commit.Delivery.TaskID != input.TaskID || commit.Delivery.SelectedWorkflowRunID != input.ResultID {
			return nativeErr("contract_mismatch", "task conclusion must reference the unique verified execution delivery")
		}
		next := declaration.After
		if err = tx.UpdateNativeTaskCAS(next, current.Version); err != nil {
			return err
		}
		delivery = commit.Delivery
		receipt = taskmodel.ResultReceipt{Input: input, TaskVersion: next.Version, GraphRevision: graph.Authority.GraphRevision + 1, Status: "closed"}
		if _, err := tx.tx.ExecContext(ctx, `INSERT INTO task_results(project_identity,task_id,result_id,request_id,input,receipt) VALUES(?,?,?,?,?,?)`, input.ProjectIdentity, input.TaskID, input.ResultID, input.RequestID, mustResultJSON(input), mustResultJSON(receipt)); err != nil {
			return err
		}
		body, _ := json.Marshal(executionmodel.TaskDeliveryReceipt{TaskID: input.TaskID, TaskVersion: next.Version, WorkflowRunID: commit.Workflow.WorkflowRunID, GraphRevision: receipt.GraphRevision, Status: "closed", Delivery: commit.Delivery})
		_, err = tx.AppendEvent(storageports.DomainEvent{ProjectIdentity: input.ProjectIdentity, TaskID: input.TaskID, WorkflowRunID: commit.Workflow.WorkflowRunID, ExecutionID: commit.Execution.ExecutionID, ExternalSource: "native-task-delivery", ExternalID: commit.Delivery.PRURL + ":" + commit.Delivery.MergeCommit, Type: "task.delivered", ReceivedAt: commit.Delivery.RecordedAt, Payload: body})
		return err
	})
	if err != nil && !errors.Is(err, errAtomicReplay) {
		return storageports.ResultCommitReceipt{}, err
	}
	return storageports.ResultCommitReceipt{Task: receipt, Delivery: delivery}, nil
}

func (s *SQLiteStore) TaskResult(ctx context.Context, project, id string) (taskmodel.ResultReceipt, bool, error) {
	return readTaskResult(ctx, s.db, project, id)
}
func (tx *sqliteTx) taskResult(ctx context.Context, project, id string) (taskmodel.ResultReceipt, bool, error) {
	return readTaskResult(ctx, tx.tx, project, id)
}
func readTaskResult(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, project, id string) (taskmodel.ResultReceipt, bool, error) {
	var raw []byte
	var receipt taskmodel.ResultReceipt
	err := q.QueryRowContext(ctx, `SELECT receipt FROM task_results WHERE project_identity=? AND result_id=?`, project, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return receipt, false, nil
	}
	if err != nil {
		return receipt, false, err
	}
	err = json.Unmarshal(raw, &receipt)
	return receipt, err == nil, err
}
func mustResultJSON(v any) []byte { b, _ := json.Marshal(v); return b }
func validateResultGates(record taskmodel.TaskRecord, contract taskmodel.TaskContract, workflow executionmodel.WorkflowRunRecord, execution executionmodel.ExecutionRecord) (string, error) {
	session, _ := executionSessionFacts(&execution)
	url, err := executionmodel.DeliveryPR(record.TaskID, record.ContractID, contract.ContractID, contract.Digest, executionmodel.DeliveryGate{WorkflowVersion: workflow.WorkflowVersion, FlowRef: workflow.FlowRef, WorkflowTaskID: workflow.TaskID, SelectedExecutionID: workflow.SelectedResult, ExecutionID: execution.ExecutionID, ExecutionWorkflowID: execution.WorkflowRunID, WorkflowRunID: workflow.WorkflowRunID, PRURL: workflow.PRURL, SessionID: session, WorkspaceBranch: execution.Workspace.Branch, Status: workflow.Status, StepID: workflow.StepID}, executionmodel.Version)
	if err != nil {
		var e *executionmodel.ResultError
		if errors.As(err, &e) {
			return "", nativeErr(e.Code, e.Message)
		}
	}
	return url, err
}
