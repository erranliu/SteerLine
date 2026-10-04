package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/steerline/persistence"
	"github.com/erranliu/SteerLine/internal/steerline/publication"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

type TaskDeliveryVerifier interface {
	Query(context.Context, string) (executionmodel.PRInfo, error)
}

// CompleteNativeTask verifies the exact selected PR outside SQLite, then
// rechecks the task, selected workflow result and fixed gate evidence in one
// transaction before writing delivery, closed state and graph revision.
func (c CommandService) CompleteNativeTask(ctx context.Context, config Config, taskID string, expectedTaskVersion uint64, workflowRunID string, verifier TaskDeliveryVerifier) (executionmodel.TaskDeliveryReceipt, error) {
	if err := validateWorkflowConfig(&config, nil); err != nil {
		return executionmodel.TaskDeliveryReceipt{}, err
	}
	sqlite := c.Store
	if sqlite == nil || c.Project == "" || config.Identity() != c.Project {
		return executionmodel.TaskDeliveryReceipt{}, nativeErr("authority_mismatch", "delivery requires the configured native project")
	}
	if err := taskmodel.ValidateID(taskID); err != nil {
		return executionmodel.TaskDeliveryReceipt{}, err
	}
	if expectedTaskVersion == 0 || workflowRunID == "" {
		return executionmodel.TaskDeliveryReceipt{}, nativeErr("immutable_task", "delivery requires task version and workflow identity")
	}
	graph, err := sqlite.TaskGraphSnapshot(ctx, c.Project)
	if err != nil {
		return executionmodel.TaskDeliveryReceipt{}, err
	}
	task, exists := graph.ByID[taskID]
	if !exists {
		return executionmodel.TaskDeliveryReceipt{}, nativeErr("task_not_found", "native task was not found")
	}
	if existing, ok := graph.Deliveries[taskID]; ok {
		if task.Status != "closed" {
			return executionmodel.TaskDeliveryReceipt{}, nativeErr("inconsistent_delivery", "task has a delivery without closed state")
		}
		if existing.SelectedWorkflowRunID != workflowRunID {
			return executionmodel.TaskDeliveryReceipt{}, nativeErr("inconsistent_delivery", "task is already delivered by a different workflow")
		}
		saved, found, err := sqlite.TaskResult(ctx, c.Project, workflowRunID)
		if err != nil {
			return executionmodel.TaskDeliveryReceipt{}, err
		}
		if !found {
			return executionmodel.TaskDeliveryReceipt{}, nativeErr("inconsistent_delivery", "delivery has no normalized task result")
		}
		if expectedTaskVersion != saved.Input.ExpectedTaskVersion {
			return executionmodel.TaskDeliveryReceipt{}, nativeErr("stale_version", "delivery retry changed the original task version")
		}
		return executionmodel.TaskDeliveryReceipt{TaskID: taskID, TaskVersion: saved.TaskVersion, WorkflowRunID: workflowRunID, GraphRevision: saved.GraphRevision, Status: saved.Status, Delivery: existing}, nil
	}
	snapshot, err := sqlite.Query(ctx, c.Project)
	if err != nil {
		return executionmodel.TaskDeliveryReceipt{}, err
	}
	found := false
	for _, w := range snapshot.WorkflowRuns {
		if w.WorkflowRunID == workflowRunID && w.TaskID == taskID {
			config, err = fixedWorkflowPublicationConfig(config, w)
			found = true
			break
		}
	}
	if err != nil {
		return executionmodel.TaskDeliveryReceipt{}, err
	}
	if !found {
		return executionmodel.TaskDeliveryReceipt{}, nativeErr("inconsistent_delivery", "original publication workflow is missing")
	}
	if client, ok := verifier.(*publication.Client); ok && !publication.ProviderIdentity(config.PR).Matches(client.Config) {
		return executionmodel.TaskDeliveryReceipt{}, nativeErr("authority_mismatch", "PR verifier differs from the original publication provider")
	}
	if verifier == nil {
		return executionmodel.TaskDeliveryReceipt{}, nativeErr("merge_state_unknown", "PR verifier is required")
	}
	if err = validateNativeConfiguredTarget(ctx, config, graph.Authority); err != nil {
		return executionmodel.TaskDeliveryReceipt{}, err
	}
	if task.Status == "closed" {
		return executionmodel.TaskDeliveryReceipt{}, nativeErr("inconsistent_delivery", "closed task has no delivery record")
	}
	if task.Status == "discarded" {
		return executionmodel.TaskDeliveryReceipt{}, c.recordDiscardedMergeConflict(ctx, sqlite, config, task, workflowRunID, verifier, graph.Authority)
	}
	if task.Status != "in_progress" || task.Version != expectedTaskVersion {
		return executionmodel.TaskDeliveryReceipt{}, nativeErr("stale_version", fmt.Sprintf("task is %s at version %d", task.Status, task.Version))
	}
	workflow, execution, err := selectedNativeDeliveryState(snapshot, taskID, workflowRunID)
	if err != nil {
		return executionmodel.TaskDeliveryReceipt{}, err
	}
	contract, err := sqlite.TaskContract(ctx, c.Project, task)
	if err != nil {
		return executionmodel.TaskDeliveryReceipt{}, err
	}
	prURL, err := validateNativeDeliveryGates(task, contract, workflow, execution)
	if err != nil {
		return executionmodel.TaskDeliveryReceipt{}, err
	}
	known, verificationErr := verifyKnownCandidatePRs(ctx, config, snapshot, taskID, verifier, workflowRunID)
	if err := verificationErr; err != nil {
		if isNativeErrorCode(err, "inconsistent_delivery") {
			_ = c.recordCandidateMergeConflict(ctx, snapshot, taskID, workflowRunID, err.Error(), known...)
		}
		return executionmodel.TaskDeliveryReceipt{}, err
	}
	var info executionmodel.PRInfo
	for _, pr := range known {
		if pr.WorkflowID == workflowRunID && pr.URL == prURL {
			info = pr.Info
			break
		}
	}
	if info.State == "unknown" {
		return executionmodel.TaskDeliveryReceipt{}, nativeErr("merge_state_unknown", "PR provider returned an unknown merge state")
	} else if !info.Merged || info.State != "merged" {
		return executionmodel.TaskDeliveryReceipt{}, nativeErr("task_not_ready", "selected PR does not have complete verified merged evidence")
	} else if info.MergedAt == nil || info.MergedAt.IsZero() || info.MergeCommit == nil || !taskmodel.ValidCommit(*info.MergeCommit) {
		return executionmodel.TaskDeliveryReceipt{}, nativeErr("merge_state_unknown", "merged PR evidence is incomplete")
	} else {
		target, targetErr := configuredRepositoryTarget(ctx, config)
		if targetErr != nil || !sameRepositoryTarget(target, graph.Authority.Repository) || config.PR.ValidateRemote(target) != nil {
			return executionmodel.TaskDeliveryReceipt{}, nativeErr("authority_mismatch", "PR configuration does not match the fixed native repository")
		}
		if err := publication.ValidateWorkerPRIdentity(info, prURL, config.PR.Repository, graph.Authority.Branch, execution.Workspace.Branch); err != nil {
			return executionmodel.TaskDeliveryReceipt{}, nativeErr("immutable_task", "merged PR does not belong to the selected worker execution branch")
		}
		number, err := config.PR.PRNumber(info.URL)
		if err != nil {
			return executionmodel.TaskDeliveryReceipt{}, nativeErr("immutable_task", "verified PR URL does not match configured platform identity")
		}
		evidence := executionmodel.TaskDeliveryEvidence{PRInstance: strings.TrimSuffix(config.PR.BaseURL, "/"), URL: info.URL, Repository: info.Repository, PRNumber: int64(number), BaseBranch: info.BaseBranch, State: info.State, Merged: true, MergedAt: *info.MergedAt, MergeCommit: *info.MergeCommit}
		evidenceBytes, _ := json.Marshal(evidence)
		record := executionmodel.TaskDeliveryRecord{ProjectIdentity: c.Project, TaskID: taskID, SelectedWorkflowRunID: workflowRunID, PRInstance: evidence.PRInstance, PRRepository: evidence.Repository, PRNumber: evidence.PRNumber, PRURL: evidence.URL, MergeCommit: evidence.MergeCommit, MergedAt: evidence.MergedAt, RecordedAt: time.Now().UTC(), Evidence: evidenceBytes}
		return c.commitNativeDelivery(ctx, sqlite, graph.Authority.AuthorityVersion, graph.Authority.GraphRevision, task, workflow, execution, contract, prURL, record, executionmodel.CandidateBindings(snapshot.WorkflowRuns, snapshot.Executions, taskID))
	}
}

// recordDiscardedMergeConflict keeps a late platform merge as audit evidence
// without converting a discarded task into a normal delivery.
func (c CommandService) recordDiscardedMergeConflict(ctx context.Context, s persistence.CommandStore, config Config, task taskmodel.TaskRecord, workflowRunID string, verifier TaskDeliveryVerifier, authority executionmodel.ProjectTaskAuthority) error {
	snapshot, err := s.Query(ctx, c.Project)
	if err != nil {
		return err
	}
	workflow, execution, err := selectedNativeDeliveryState(snapshot, task.TaskID, workflowRunID)
	if err != nil {
		return nativeErr("task_discarded", "discarded task cannot be completed by an unrelated workflow")
	}
	urls := uniqueSorted(append(append(append([]string(nil), workflow.PRHistory...), workflow.PRURL), workflow.PRCandidateURL))
	merged := false
	for _, url := range urls {
		info, err := verifier.Query(ctx, url)
		if err != nil || info.State == "unknown" {
			return nativeErr("merge_state_unknown", "discarded candidate PR merge state could not be verified")
		}
		if err := publication.ValidateWorkerPRIdentity(info, url, config.PR.Repository, authority.Branch, execution.Workspace.Branch); err != nil {
			return nativeErr("merge_state_unknown", "discarded candidate PR identity differs from its original worker")
		}
		if !info.Merged || info.State != "merged" {
			continue
		}
		if info.MergedAt == nil || info.MergedAt.IsZero() || info.MergeCommit == nil || !taskmodel.ValidCommit(*info.MergeCommit) {
			return nativeErr("merge_state_unknown", "discarded candidate merge evidence is incomplete")
		}
		err = s.TransactDeliveryAudit(ctx, func(tx persistence.DeliveryAuditTx) error {
			graph, err := tx.TaskGraphSnapshot(ctx, c.Project)
			if err != nil {
				return err
			}
			current, ok := graph.ByID[task.TaskID]
			if !ok || current.Status != "discarded" || current.Version != task.Version || graph.Authority.AuthorityVersion != authority.AuthorityVersion {
				return nativeErr("stale_version", "discarded task changed during merge reconciliation")
			}
			currentSnapshot, err := tx.Query(ctx, c.Project)
			if err != nil {
				return err
			}
			if executionmodel.CandidateBindings(currentSnapshot.WorkflowRuns, currentSnapshot.Executions, task.TaskID) != executionmodel.CandidateBindings(snapshot.WorkflowRuns, snapshot.Executions, task.TaskID) {
				return nativeErr("stale_version", "candidate PR set changed during merge reconciliation")
			}
			_, err = tx.AppendEvent(storageports.DomainEvent{ProjectIdentity: c.Project, TaskID: task.TaskID, WorkflowRunID: workflowRunID, ExecutionID: execution.ExecutionID, ExternalSource: "native-task-delivery", ExternalID: task.TaskID + ":late-merge:" + url + ":" + *info.MergeCommit, Type: "task.delivery.conflict", ReceivedAt: time.Now().UTC(), Payload: mustJSON(map[string]any{"task_id": task.TaskID, "workflow_run_id": workflowRunID, "pr_url": info.URL, "pr_head": info.HeadRevision, "merge_commit": *info.MergeCommit, "merged_at": *info.MergedAt, "pr": info, "discard_request_id": task.DiscardRequestID, "discarded_at": task.DiscardedAt})})
			return err
		})
		if err != nil {
			return err
		}
		merged = true
	}
	if merged {
		return nativeErr("inconsistent_delivery", "discarded candidate has a merged PR; manual verification is required")
	}
	return nativeErr("task_discarded", "discarded candidate PR is not verified merged")
}

func selectedNativeDeliveryState(snapshot DomainSnapshot, taskID, workflowRunID string) (executionmodel.WorkflowRunRecord, executionmodel.ExecutionRecord, error) {
	var workflow *executionmodel.WorkflowRunRecord
	for i := range snapshot.WorkflowRuns {
		if snapshot.WorkflowRuns[i].WorkflowRunID == workflowRunID && snapshot.WorkflowRuns[i].TaskID == taskID {
			workflow = &snapshot.WorkflowRuns[i]
			break
		}
	}
	if workflow == nil {
		return executionmodel.WorkflowRunRecord{}, executionmodel.ExecutionRecord{}, nativeErr("immutable_task", "selected workflow run does not belong to this task")
	}
	var execution *executionmodel.ExecutionRecord
	for i := range snapshot.Executions {
		if snapshot.Executions[i].WorkflowRunID == workflowRunID && snapshot.Executions[i].ExecutionID == workflow.SelectedResult {
			execution = &snapshot.Executions[i]
			break
		}
	}
	if execution == nil {
		return *workflow, executionmodel.ExecutionRecord{}, nativeErr("inconsistent_delivery", "selected workflow result is missing")
	}
	return *workflow, *execution, nil
}

func validateNativeDeliveryGates(record taskmodel.TaskRecord, contract taskmodel.TaskContract, workflow executionmodel.WorkflowRunRecord, execution executionmodel.ExecutionRecord) (string, error) {
	sessionID, _ := executionSessionFacts(&execution)
	url, err := executionmodel.DeliveryPR(record.TaskID, record.ContractID, contract.ContractID, contract.Digest, executionmodel.DeliveryGate{
		WorkflowVersion:     workflow.WorkflowVersion,
		WorkflowTaskID:      workflow.TaskID,
		SelectedExecutionID: workflow.SelectedResult,
		ExecutionID:         execution.ExecutionID,
		ExecutionWorkflowID: execution.WorkflowRunID,
		WorkflowRunID:       workflow.WorkflowRunID,
		PRURL:               workflow.PRURL,
		SessionID:           sessionID,
		WorkspaceBranch:     execution.Workspace.Branch,
		Phase:               workflow.Phase,
	}, executionmodel.Version)
	if err != nil {
		var e *executionmodel.ResultError
		if errors.As(err, &e) {
			return "", nativeErr(e.Code, e.Message)
		}
	}
	return url, err
}

func (c CommandService) commitNativeDelivery(ctx context.Context, s persistence.CommandStore, expectedAuthorityVersion, expectedGraphRevision uint64, task taskmodel.TaskRecord, workflow executionmodel.WorkflowRunRecord, execution executionmodel.ExecutionRecord, contract taskmodel.TaskContract, prURL string, delivery executionmodel.TaskDeliveryRecord, expectedBindings string) (executionmodel.TaskDeliveryReceipt, error) {
	if c.taskResults == nil {
		return executionmodel.TaskDeliveryReceipt{}, nativeErr("authority_mismatch", "task result port is required")
	}
	input := taskmodel.Result{ProjectIdentity: c.Project, TaskID: task.TaskID, ContractID: contract.ContractID, ContractDigest: contract.Digest, Claimant: task.Claimant, ExpectedTaskVersion: task.Version, ExpectedGraphRevision: expectedGraphRevision, ExpectedSetVersion: expectedBindings, ResultID: workflow.WorkflowRunID, RequestID: "delivery:" + workflow.WorkflowRunID, Conclusion: "delivered"}
	declaration, err := c.taskResults.Declare(task, contract, input, delivery.RecordedAt)
	if err != nil {
		return executionmodel.TaskDeliveryReceipt{}, err
	}
	result, err := s.CommitTaskResult(ctx, storageports.ResultCommit{Task: declaration, AuthorityVersion: expectedAuthorityVersion, Workflow: workflow, Execution: execution, Delivery: delivery})
	if err != nil {
		return executionmodel.TaskDeliveryReceipt{}, err
	}
	return executionmodel.TaskDeliveryReceipt{TaskID: task.TaskID, TaskVersion: result.Task.TaskVersion, WorkflowRunID: workflow.WorkflowRunID, GraphRevision: result.Task.GraphRevision, Status: result.Task.Status, Delivery: result.Delivery}, nil
}
