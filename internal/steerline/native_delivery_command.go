package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/persistence"
	"github.com/erranliu/SteerLine/internal/steerline/publication"
	taskdomain "github.com/erranliu/SteerLine/internal/steerline/task"
)

// TaskClaimReceipt records reservation, not worker start. The existing

type TaskDeliveryVerifier interface {
	Query(context.Context, string) (PRInfo, error)
}

type TaskDeliveryReceipt = taskdomain.TaskDeliveryReceipt

// CompleteNativeTask verifies the exact selected PR outside SQLite, then
// rechecks the task, selected workflow result and fixed gate evidence in one
// transaction before writing delivery, closed state and graph revision.
func (c CommandService) CompleteNativeTask(ctx context.Context, config Config, taskID string, expectedTaskVersion uint64, workflowRunID string, verifier TaskDeliveryVerifier) (TaskDeliveryReceipt, error) {
	if err := validateWorkflowConfig(&config, nil); err != nil {
		return TaskDeliveryReceipt{}, err
	}
	sqlite := c.Store
	if sqlite == nil || c.Project == "" || config.Identity() != c.Project {
		return TaskDeliveryReceipt{}, nativeErr("authority_mismatch", "delivery requires the configured native project")
	}
	if err := ValidateNativeTaskID(taskID); err != nil {
		return TaskDeliveryReceipt{}, err
	}
	if expectedTaskVersion == 0 || workflowRunID == "" {
		return TaskDeliveryReceipt{}, nativeErr("immutable_task", "delivery requires task version and workflow identity")
	}
	graph, err := sqlite.TaskGraphSnapshot(ctx, c.Project)
	if err != nil {
		return TaskDeliveryReceipt{}, err
	}
	task, exists := graph.ByID[taskID]
	if !exists {
		return TaskDeliveryReceipt{}, nativeErr("task_not_found", "native task was not found")
	}
	if existing, ok := graph.Deliveries[taskID]; ok {
		if task.Status != "closed" {
			return TaskDeliveryReceipt{}, nativeErr("inconsistent_delivery", "task has a delivery without closed state")
		}
		if existing.SelectedWorkflowRunID != workflowRunID {
			return TaskDeliveryReceipt{}, nativeErr("inconsistent_delivery", "task is already delivered by a different workflow")
		}
		return TaskDeliveryReceipt{TaskID: taskID, TaskVersion: task.Version, WorkflowRunID: workflowRunID, GraphRevision: graph.Authority.GraphRevision, Status: "closed", Delivery: existing}, nil
	}
	snapshot, err := sqlite.Query(ctx, c.Project)
	if err != nil {
		return TaskDeliveryReceipt{}, err
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
		return TaskDeliveryReceipt{}, err
	}
	if !found {
		return TaskDeliveryReceipt{}, nativeErr("inconsistent_delivery", "original publication workflow is missing")
	}
	if client, ok := verifier.(*publication.Client); ok && !publication.ProviderIdentity(config.PR).Matches(client.Config) {
		return TaskDeliveryReceipt{}, nativeErr("authority_mismatch", "PR verifier differs from the original publication provider")
	}
	if verifier == nil {
		return TaskDeliveryReceipt{}, nativeErr("merge_state_unknown", "PR verifier is required")
	}
	if err = validateNativeConfiguredTarget(ctx, config, graph.Authority); err != nil {
		return TaskDeliveryReceipt{}, err
	}
	if task.Status == "closed" {
		return TaskDeliveryReceipt{}, nativeErr("inconsistent_delivery", "closed task has no delivery record")
	}
	if task.Status == "discarded" {
		return TaskDeliveryReceipt{}, c.recordDiscardedMergeConflict(ctx, sqlite, config, task, workflowRunID, verifier, graph.Authority)
	}
	if task.Status != "in_progress" || task.Version != expectedTaskVersion {
		return TaskDeliveryReceipt{}, nativeErr("stale_version", fmt.Sprintf("task is %s at version %d", task.Status, task.Version))
	}
	workflow, execution, err := selectedNativeDeliveryState(snapshot, taskID, workflowRunID)
	if err != nil {
		return TaskDeliveryReceipt{}, err
	}
	contract, err := sqlite.TaskContract(ctx, c.Project, task)
	if err != nil {
		return TaskDeliveryReceipt{}, err
	}
	prURL, err := validateNativeDeliveryGates(task, contract, workflow, execution)
	if err != nil {
		return TaskDeliveryReceipt{}, err
	}
	discardPlan, _, err := taskDiscardPlanFromSnapshot(snapshot, taskID)
	if err != nil {
		return TaskDeliveryReceipt{}, err
	}
	if discardPlan.UncertainPR || discardPlan.PRURL != prURL {
		return TaskDeliveryReceipt{}, nativeErr("merge_state_unknown", "task has another unresolved or mismatched PR publication")
	}
	if info, err := verifier.Query(ctx, prURL); err != nil {
		return TaskDeliveryReceipt{}, nativeErr("merge_state_unknown", "PR merge state could not be verified: "+clip(err.Error(), 1000))
	} else if info.State == "unknown" {
		return TaskDeliveryReceipt{}, nativeErr("merge_state_unknown", "PR provider returned an unknown merge state")
	} else if !info.Merged || info.State != "merged" {
		return TaskDeliveryReceipt{}, nativeErr("task_not_ready", "selected PR does not have complete verified merged evidence")
	} else if info.MergedAt == nil || info.MergedAt.IsZero() || info.MergeCommit == nil || !validCommit(*info.MergeCommit) {
		return TaskDeliveryReceipt{}, nativeErr("merge_state_unknown", "merged PR evidence is incomplete")
	} else {
		target, targetErr := configuredRepositoryTarget(ctx, config)
		if targetErr != nil || !sameRepositoryTarget(target, graph.Authority.Repository) || config.PR.ValidateRemote(target) != nil {
			return TaskDeliveryReceipt{}, nativeErr("authority_mismatch", "PR configuration does not match the fixed native repository")
		}
		if err := publication.ValidateWorkerPRIdentity(info, prURL, config.PR.Repository, graph.Authority.Branch, execution.Workspace.Branch); err != nil {
			return TaskDeliveryReceipt{}, nativeErr("immutable_task", "merged PR does not belong to the selected worker execution branch")
		}
		number, err := config.PR.PRNumber(info.URL)
		if err != nil {
			return TaskDeliveryReceipt{}, nativeErr("immutable_task", "verified PR URL does not match configured platform identity")
		}
		evidence := TaskDeliveryEvidence{PRInstance: strings.TrimSuffix(config.PR.BaseURL, "/"), URL: info.URL, Repository: info.Repository, PRNumber: int64(number), BaseBranch: info.BaseBranch, State: info.State, Merged: true, MergedAt: *info.MergedAt, MergeCommit: *info.MergeCommit}
		evidenceBytes, _ := json.Marshal(evidence)
		record := TaskDeliveryRecord{ProjectIdentity: c.Project, TaskID: taskID, SelectedWorkflowRunID: workflowRunID, PRInstance: evidence.PRInstance, PRRepository: evidence.Repository, PRNumber: evidence.PRNumber, PRURL: evidence.URL, MergeCommit: evidence.MergeCommit, MergedAt: evidence.MergedAt, RecordedAt: time.Now().UTC(), Evidence: evidenceBytes}
		return c.commitNativeDelivery(ctx, sqlite, graph.Authority.AuthorityVersion, task, workflow, execution, contract, prURL, record)
	}
}

// recordDiscardedMergeConflict keeps a late platform merge as audit evidence
// without converting a discarded task into a normal delivery.
func (c CommandService) recordDiscardedMergeConflict(ctx context.Context, s persistence.CommandStore, config Config, task TaskRecord, workflowRunID string, verifier TaskDeliveryVerifier, authority ProjectTaskAuthority) error {
	owned, err := s.WorkflowOwned(ctx, c.Project, workflowRunID, task.TaskID)
	if err != nil {
		return err
	}
	if !owned {
		return nativeErr("task_discarded", "discarded task cannot be completed by an unrelated workflow")
	}
	plan, current, err := taskDiscardPlanFromStore(ctx, s, c.Project, task.TaskID)
	if err != nil {
		return err
	}
	if current.Status != "discarded" || current.Version != task.Version || plan.PRURL == "" || plan.UncertainPR {
		return nativeErr("task_discarded", "discarded task has no conclusive PR binding to reconcile")
	}
	target, err := configuredRepositoryTarget(ctx, config)
	if err != nil || !sameRepositoryTarget(target, authority.Repository) || config.PR.ValidateRemote(target) != nil {
		return nativeErr("authority_mismatch", "PR configuration does not match the discarded task repository")
	}
	info, err := verifier.Query(ctx, plan.PRURL)
	if err != nil || info.State == "unknown" {
		return nativeErr("merge_state_unknown", "discarded task PR merge state could not be verified")
	}
	if !info.Merged || info.State != "merged" {
		return nativeErr("task_discarded", "discarded task PR is not verified merged")
	}
	if info.URL != plan.PRURL || info.Repository != config.PR.Repository || info.BaseBranch != authority.Branch || info.MergedAt == nil || info.MergedAt.IsZero() || info.MergeCommit == nil || !validCommit(*info.MergeCommit) {
		return nativeErr("merge_state_unknown", "discarded task PR merge identity or evidence is incomplete")
	}
	if _, err = config.PR.PRNumber(info.URL); err != nil {
		return nativeErr("merge_state_unknown", "discarded task PR URL does not match its fixed platform identity")
	}
	conflict := struct {
		TaskID           string     `json:"task_id"`
		WorkflowRunID    string     `json:"workflow_run_id"`
		DiscardRequestID string     `json:"discard_request_id"`
		DiscardedAt      *time.Time `json:"discarded_at,omitempty"`
		PRURL            string     `json:"pr_url"`
		PRHead           string     `json:"pr_head"`
		MergeCommit      string     `json:"merge_commit"`
		MergedAt         time.Time  `json:"merged_at"`
	}{task.TaskID, workflowRunID, task.DiscardRequestID, task.DiscardedAt, info.URL, info.HeadRevision, *info.MergeCommit, *info.MergedAt}
	body, err := json.Marshal(conflict)
	if err != nil {
		return err
	}
	err = s.TransactDelivery(ctx, func(tx persistence.DeliveryTx) error {
		graph, e := tx.TaskGraphSnapshot(ctx, c.Project)
		if e != nil {
			return e
		}
		stored, exists := graph.ByID[task.TaskID]
		if !exists || stored.Status != "discarded" || stored.Version != task.Version || graph.Authority.AuthorityVersion != authority.AuthorityVersion {
			return nativeErr("stale_version", "discarded task changed during PR merge reconciliation")
		}
		if _, delivered := graph.Deliveries[task.TaskID]; delivered {
			return nativeErr("inconsistent_delivery", "discarded task already has a normal delivery")
		}
		fixed, _, e := taskDiscardPlanTx(ctx, tx, c.Project, task.TaskID)
		if e != nil {
			return e
		}
		if fixed.PRURL != plan.PRURL || fixed.UncertainPR {
			return nativeErr("merge_state_unknown", "discarded task PR binding changed during reconciliation")
		}
		_, e = tx.AppendEvent(DomainEvent{ProjectIdentity: c.Project, TaskID: task.TaskID, WorkflowRunID: workflowRunID, ExternalSource: "native-task-delivery", ExternalID: task.TaskID + ":late-merge:" + *info.MergeCommit, Type: "task.delivery.conflict", ReceivedAt: time.Now().UTC(), Payload: body})
		return e
	})
	if err != nil {
		return err
	}
	return nativeErr("inconsistent_delivery", "discarded task has a merged PR; manual verification is required")
}

func selectedNativeDeliveryState(snapshot DomainSnapshot, taskID, workflowRunID string) (WorkflowRunRecord, ExecutionRecord, error) {
	var workflow *WorkflowRunRecord
	for i := range snapshot.WorkflowRuns {
		if snapshot.WorkflowRuns[i].WorkflowRunID == workflowRunID && snapshot.WorkflowRuns[i].TaskID == taskID {
			workflow = &snapshot.WorkflowRuns[i]
			break
		}
	}
	if workflow == nil {
		return WorkflowRunRecord{}, ExecutionRecord{}, nativeErr("immutable_task", "selected workflow run does not belong to this task")
	}
	var execution *ExecutionRecord
	for i := range snapshot.Executions {
		if snapshot.Executions[i].WorkflowRunID == workflowRunID && snapshot.Executions[i].ExecutionID == workflow.SelectedResult {
			execution = &snapshot.Executions[i]
			break
		}
	}
	if execution == nil {
		return *workflow, ExecutionRecord{}, nativeErr("inconsistent_delivery", "selected workflow result is missing")
	}
	return *workflow, *execution, nil
}

func validateNativeDeliveryGates(record TaskRecord, contract TaskContract, workflow WorkflowRunRecord, execution ExecutionRecord) (string, error) {
	sessionID, _ := executionSessionFacts(&execution)
	return taskdomain.DeliveryPR(record, contract, taskdomain.DeliveryGate{
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
	}, WorkerFlowVersion)
}

func (c CommandService) commitNativeDelivery(ctx context.Context, s persistence.CommandStore, expectedAuthorityVersion uint64, task TaskRecord, workflow WorkflowRunRecord, execution ExecutionRecord, contract TaskContract, prURL string, delivery TaskDeliveryRecord) (TaskDeliveryReceipt, error) {
	var receipt TaskDeliveryReceipt
	err := s.TransactDelivery(ctx, func(tx persistence.DeliveryTx) error {
		graph, err := tx.TaskGraphSnapshot(ctx, c.Project)
		if err != nil {
			return err
		}
		if graph.Authority.AuthorityVersion != expectedAuthorityVersion {
			return nativeErr("stale_version", "native project authority changed during PR verification")
		}
		current, ok := graph.ByID[task.TaskID]
		if !ok {
			return nativeErr("task_not_found", "native task was not found")
		}
		if record, found := graph.Deliveries[task.TaskID]; found {
			if current.Status != "closed" {
				return nativeErr("inconsistent_delivery", "task has a delivery without closed state")
			}
			if record.SelectedWorkflowRunID == workflow.WorkflowRunID && record.MergeCommit == delivery.MergeCommit {
				receipt = TaskDeliveryReceipt{TaskID: task.TaskID, TaskVersion: current.Version, WorkflowRunID: record.SelectedWorkflowRunID, GraphRevision: graph.Authority.GraphRevision, Status: "closed", Delivery: record}
				return taskDeliveryReplay{Receipt: receipt}
			}
			return nativeErr("inconsistent_delivery", "task already has a different authoritative delivery")
		}
		if current.Status == "closed" {
			return nativeErr("inconsistent_delivery", "closed task has no delivery record")
		}
		if current.Status == "discarded" {
			return nativeErr("task_discarded", "discarded task cannot be completed")
		}
		if current.Status != "in_progress" || current.Version != task.Version {
			return nativeErr("stale_version", "task state changed during PR verification")
		}
		currentWorkflow, err := tx.Workflow(ctx, c.Project, workflow.WorkflowRunID, task.TaskID)
		if err != nil {
			return nativeErr("immutable_task", "selected workflow run changed during PR verification")
		}
		currentExecution, err := tx.Execution(ctx, c.Project, execution.ExecutionID, workflow.WorkflowRunID)
		if err != nil {
			return nativeErr("inconsistent_delivery", "selected execution changed during PR verification")
		}
		currentContract, err := tx.TaskContract(ctx, c.Project, current)
		if err != nil {
			return err
		}
		if currentContract.Digest != contract.Digest {
			return nativeErr("immutable_task", "fixed task contract changed during PR verification")
		}
		currentPRURL, err := validateNativeDeliveryGates(current, currentContract, currentWorkflow, currentExecution)
		if err != nil {
			return err
		}
		if digestValue(currentWorkflow.ExecutionProfile) != digestValue(workflow.ExecutionProfile) || digestValue(currentExecution.PolicyResources) != digestValue(execution.PolicyResources) || currentWorkflow.WorkflowRunID != workflow.WorkflowRunID || currentWorkflow.SelectedResult != execution.ExecutionID || currentPRURL != prURL ||
			currentWorkflow.WorkflowVersion != workflow.WorkflowVersion || digestValue(currentExecution.ImplementationData) != digestValue(execution.ImplementationData) ||
			currentExecution.Workspace.Branch != execution.Workspace.Branch {
			return nativeErr("immutable_task", "selected result or PR binding changed during platform verification")
		}
		discardPlan, _, err := taskDiscardPlanTx(ctx, tx, c.Project, task.TaskID)
		if err != nil {
			return err
		}
		if discardPlan.UncertainPR || discardPlan.PRURL != prURL {
			return nativeErr("merge_state_unknown", "task publication became unresolved during PR verification")
		}
		reused, err := tx.PRBindingsOnOtherTasks(ctx, c.Project, task.TaskID, delivery.PRURL)
		if err != nil {
			return err
		}
		if reused > 0 {
			return nativeErr("immutable_task", "PR is already bound to another native task")
		}
		if err = tx.PutTaskDelivery(delivery); err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "unique constraint failed: task_deliveries.pr_instance") {
				return nativeErr("immutable_task", "PR delivery identity is already bound to another task")
			}
			return err
		}
		next, err := taskdomain.CompleteDelivery(current, delivery, graph.Authority.Branch, time.Now().UTC())
		if err != nil {
			return err
		}
		if err = tx.UpdateNativeTaskCAS(next, current.Version); err != nil {
			return err
		}
		receipt = TaskDeliveryReceipt{TaskID: task.TaskID, TaskVersion: next.Version, WorkflowRunID: workflow.WorkflowRunID, GraphRevision: graph.Authority.GraphRevision + 1, Status: "closed", Delivery: delivery}
		body, _ := json.Marshal(receipt)
		_, err = tx.AppendEvent(DomainEvent{ProjectIdentity: c.Project, TaskID: task.TaskID, WorkflowRunID: workflow.WorkflowRunID, ExecutionID: execution.ExecutionID, ExternalSource: "native-task-delivery", ExternalID: delivery.PRURL + ":" + delivery.MergeCommit, Type: "task.delivered", ReceivedAt: delivery.RecordedAt, Payload: body})
		return err
	})
	var replay taskDeliveryReplay
	if errors.As(err, &replay) {
		return replay.Receipt, nil
	}
	return receipt, err
}

type taskDeliveryReplay struct{ Receipt TaskDeliveryReceipt }

func (e taskDeliveryReplay) Error() string { return "task delivery already committed" }
