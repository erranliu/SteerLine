package steerline

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type NativeStepRetryOptions struct {
	RequestID           string
	TaskID              string
	ExpectedTaskVersion uint64
	WorkflowRunID       string
	ExecutionID         string
	FailedAttemptID     string
	RequestedBy         string
}

type NativeStepRetryReceipt struct {
	RequestID       string `json:"request_id"`
	TaskID          string `json:"task_id"`
	WorkflowRunID   string `json:"workflow_run_id"`
	ExecutionID     string `json:"execution_id"`
	PreviousAttempt string `json:"previous_attempt_id"`
	AttemptID       string `json:"attempt_id"`
	StepID          string `json:"step_id"`
	AttemptNumber   int    `json:"attempt_number"`
	Status          string `json:"status"`
}

// RetryNativeStep admits one settled, failed step as a new attempt on the
// original workflow and execution. The old attempt remains immutable history.
func (c CommandService) RetryNativeStep(ctx context.Context, options NativeStepRetryOptions) (NativeStepRetryReceipt, error) {
	sqlite, ok := c.Store.(*SQLiteStore)
	if !ok || c.Project == "" {
		return NativeStepRetryReceipt{}, nativeErr("authority_mismatch", "native retry requires the center SQLite project")
	}
	if !controlIDPattern.MatchString(options.RequestID) || !canonicalDecimalID(options.TaskID) || options.ExpectedTaskVersion == 0 || !stableExecutionID(options.WorkflowRunID) || !stableExecutionID(options.ExecutionID) || !stableExecutionID(options.FailedAttemptID) {
		return NativeStepRetryReceipt{}, nativeErr("invalid_batch", "native retry requires request, task, workflow, execution and failed-attempt identities")
	}
	if options.RequestedBy == "" {
		options.RequestedBy = DefaultRequestedBy()
	}
	requestRaw, err := json.Marshal(struct {
		TaskID          string `json:"task_id"`
		WorkflowRunID   string `json:"workflow_run_id"`
		ExecutionID     string `json:"execution_id"`
		FailedAttemptID string `json:"failed_attempt_id"`
		RequestedBy     string `json:"requested_by"`
	}{options.TaskID, options.WorkflowRunID, options.ExecutionID, options.FailedAttemptID, options.RequestedBy})
	if err != nil {
		return NativeStepRetryReceipt{}, err
	}
	requestPayload, err := CanonicalJSON(requestRaw)
	if err != nil {
		return NativeStepRetryReceipt{}, err
	}
	newAttemptID := newID()
	created := time.Now().UTC()
	var receipt NativeStepRetryReceipt
	err = sqlite.Transact(ctx, func(domainTx DomainTx) error {
		tx, ok := domainTx.(*sqliteTx)
		if !ok {
			return nativeErr("authority_mismatch", "native retry requires a center SQLite transaction")
		}
		if old, found, lookupErr := lookupTaskBatchCommandTx(ctx, tx, c.Project, options.RequestID); lookupErr != nil {
			return lookupErr
		} else if found {
			var replayErr error
			receipt, replayErr = replayNativeStepRetry(old, requestPayload, c.Project, options.RequestedBy)
			if replayErr != nil {
				return replayErr
			}
			return taskBatchReplay{Receipt: TaskBatchReceipt{RequestID: options.RequestID, Status: "accepted"}}
		}
		graph, readErr := taskGraphSnapshotTx(ctx, tx, c.Project)
		if readErr != nil {
			return readErr
		}
		task, found := graph.ByID[options.TaskID]
		if !found {
			return nativeErr("task_not_found", "native retry task was not found")
		}
		if task.Source != "native" || task.Status != "in_progress" || task.Version != options.ExpectedTaskVersion {
			return nativeErr("stale_version", "native retry task status or version changed")
		}
		if nativeDeliveryInconsistent(task, graph) || graph.Deliveries[task.TaskID].TaskID != "" {
			return nativeErr("inconsistent_delivery", "a delivered or inconsistent task cannot be retried")
		}
		if graph.Authority.DispatchPaused {
			return nativeErr("dispatch_paused", "dispatch is paused")
		}
		var occupied int
		if err := tx.tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM executions WHERE project_identity=? AND status IN ('pending','active','waiting_decision','blocked') AND execution_id<>?`, c.Project, options.ExecutionID).Scan(&occupied); err != nil {
			return err
		}
		if occupied != 0 {
			return nativeErr("slot_busy", "another execution holds or may hold the project execution slot")
		}
		snapshot, err := querySnapshot(ctx, tx.tx, c.Project)
		if err != nil {
			return err
		}
		var workflow *WorkflowRunRecord
		for i := range snapshot.WorkflowRuns {
			if snapshot.WorkflowRuns[i].WorkflowRunID == options.WorkflowRunID && snapshot.WorkflowRuns[i].TaskID == task.TaskID {
				workflow = &snapshot.WorkflowRuns[i]
				break
			}
		}
		var execution *ExecutionRecord
		for i := range snapshot.Executions {
			if snapshot.Executions[i].ExecutionID == options.ExecutionID && snapshot.Executions[i].WorkflowRunID == options.WorkflowRunID {
				execution = &snapshot.Executions[i]
				break
			}
		}
		var failed *StepAttemptRecord
		for i := range snapshot.Attempts {
			if snapshot.Attempts[i].AttemptID == options.FailedAttemptID && snapshot.Attempts[i].ExecutionID == options.ExecutionID {
				failed = &snapshot.Attempts[i]
				break
			}
		}
		if workflow == nil || execution == nil || failed == nil || execution.CurrentStep != failed.StepID || execution.Status != "failed" || workflow.Phase != "blocked" || workflow.SelectedResult != "" || failed.Status != "failed" && failed.Status != "rejected" {
			return nativeErr("stale_version", "retry requires the current settled failed step of a blocked native execution")
		}
		contractBytes, marshalErr := json.Marshal(minimalStepContract(workflow.ProjectSteps, failed.StepID))
		if marshalErr != nil || workflow.WorkflowVersion != MinimalWorkflowVersion || workflow.Definition.ID != MinimalWorkflowVersion || workflow.Definition.Version != MinimalWorkflowVersion || workflow.ConfigDigest == "" || failed.ContractDigest != operationDigest(contractBytes) {
			return nativeErr("contract_mismatch", "failed step does not match its fixed native workflow contract")
		}
		if task.DiscardedAt != nil || len(task.CleanupPending) != 0 || execution.Target.WorktreeID == "" || execution.Target.Branch == "" || execution.Target.BaseRevision == "" {
			return nativeErr("task_discarded", "discarded or unbound native execution cannot be retried")
		}
		for _, candidate := range snapshot.Attempts {
			if candidate.ExecutionID == execution.ExecutionID && isActiveStepAttempt(candidate.Status) {
				return nativeErr("slot_busy", "native execution has an unsettled step attempt")
			}
			if candidate.ExecutionID == execution.ExecutionID && candidate.StepID == failed.StepID && candidate.AttemptNumber > failed.AttemptNumber {
				return nativeErr("stale_version", "a later attempt already superseded the selected failure")
			}
		}
		for _, candidate := range snapshot.WorkflowRuns {
			if candidate.TaskID == task.TaskID && candidate.WorkflowRunID != workflow.WorkflowRunID && candidate.Phase != "blocked" && candidate.Phase != "completed" {
				return nativeErr("inconsistent_delivery", "another workflow is still authoritative for the task")
			}
		}
		if _, err = nativeContractForTaskTx(ctx, tx, c.Project, task); err != nil {
			return err
		}
		number := nextStepAttemptNumber(snapshot.Attempts, execution.ExecutionID, failed.StepID)
		attempt := StepAttemptRecord{ProjectIdentity: c.Project, AttemptID: newAttemptID, ExecutionID: execution.ExecutionID, StepID: failed.StepID, AttemptNumber: number, Status: "planned", InputRevision: failed.InputRevision, ContractDigest: failed.ContractDigest, PrerequisiteIDs: append([]string(nil), failed.PrerequisiteIDs...), CreatedAt: created, UpdatedAt: created}
		execution.Status, execution.Process, execution.UpdatedAt = "pending", nil, created
		workflow.Phase, workflow.UpdatedAt = "executing", created
		command := CommandRecord{ProjectIdentity: c.Project, CommandID: options.RequestID, Kind: "retry_step", TargetID: task.TaskID, ExpectedVersion: options.ExpectedTaskVersion, IdempotencyKey: options.RequestID, RequestedBy: options.RequestedBy, Payload: requestPayload, Status: "in_flight", CreatedAt: created}
		if _, made, enqueueErr := tx.EnqueueCommand(command); enqueueErr != nil {
			return nativeErr("idempotency_conflict", enqueueErr.Error())
		} else if !made {
			return nativeErr("idempotency_conflict", "retry request ID is already in use")
		}
		operation := OperationRecord{ProjectIdentity: c.Project, OperationID: options.RequestID, IdempotencyKey: options.RequestID, Kind: "retry_step", Status: "confirmed", PayloadDigest: digestCanonical(requestPayload), Request: requestPayload, CreatedAt: created, UpdatedAt: created}
		if _, made, putErr := tx.PutOperation(operation); putErr != nil {
			return putErr
		} else if !made {
			return nativeErr("idempotency_conflict", "retry operation request ID is already in use")
		}
		if err = tx.PutWorkflowRun(*workflow); err != nil {
			return err
		}
		if err = tx.PutExecution(*execution); err != nil {
			return err
		}
		if err = tx.PutStepAttempt(attempt); err != nil {
			return err
		}
		receipt = NativeStepRetryReceipt{RequestID: options.RequestID, TaskID: task.TaskID, WorkflowRunID: workflow.WorkflowRunID, ExecutionID: execution.ExecutionID, PreviousAttempt: failed.AttemptID, AttemptID: attempt.AttemptID, StepID: attempt.StepID, AttemptNumber: attempt.AttemptNumber, Status: "planned"}
		command.Status = "accepted"
		command.Result, _ = json.Marshal(receipt)
		command.UpdatedAt = created
		if err = tx.UpdateCommand(command); err != nil {
			return err
		}
		body, _ := json.Marshal(receipt)
		_, err = tx.AppendEvent(DomainEvent{ProjectIdentity: c.Project, TaskID: task.TaskID, WorkflowRunID: workflow.WorkflowRunID, ExecutionID: execution.ExecutionID, AttemptID: attempt.AttemptID, ExternalSource: "native-task-command", ExternalID: options.RequestID, Type: "execution.step.retry.reserved", ReceivedAt: created, Payload: body})
		return err
	})
	var replay taskBatchReplay
	if errors.As(err, &replay) {
		return receipt, nil
	}
	return receipt, err
}

func replayNativeStepRetry(old CommandRecord, requestPayload []byte, project, requestedBy string) (NativeStepRetryReceipt, error) {
	if old.ProjectIdentity != project || old.Kind != "retry_step" || old.RequestedBy != requestedBy || !bytes.Equal(old.Payload, requestPayload) || old.Status != "accepted" || len(old.Result) == 0 {
		return NativeStepRetryReceipt{}, nativeErr("idempotency_conflict", "retry request ID was used for different command content")
	}
	var receipt NativeStepRetryReceipt
	if err := decode(old.Result, &receipt); err != nil || receipt.RequestID != old.CommandID || receipt.AttemptID == "" {
		return receipt, nativeErr("unsupported_schema", "saved native retry receipt is invalid")
	}
	return receipt, nil
}

func isActiveStepAttempt(status string) bool {
	switch status {
	case "planned", "dispatching", "dispatched", "running", "waiting_decision", "stopping", "outcome_unknown":
		return true
	default:
		return false
	}
}

func (s *Service) startNativeRetry(ctx context.Context, runID string) error {
	if !stableExecutionID(runID) {
		return errors.New("invalid native workflow run ID")
	}
	if s.worker != nil || s.State.SlotOccupied {
		return errors.New("execution slot occupied")
	}
	if reason := s.slotReason(""); reason != "" {
		return errors.New(reason)
	}
	var run *Run
	for _, candidate := range s.State.Runs {
		if candidate != nil && candidate.RunID == runID {
			run = candidate
			break
		}
	}
	if run == nil || run.WorkflowVersion != MinimalWorkflowVersion || run.ExecutionStatus != "failed" || run.FinishTaskStatus != "blocked" || run.PR.URL != nil {
		return nativeErr("stale_version", "retry requires a settled failed native workflow without a bound PR")
	}
	task := nativeTaskRecord(s.State.Tasks, run.DomainTaskID)
	workflow, execution := s.minimalRecords(run)
	failed := s.minimalAttempt(run, run.AttemptID)
	if task == nil || task.Status != "in_progress" || task.Claimant != run.Claimant || workflow == nil || execution == nil || failed == nil || execution.Status != "failed" || workflow.Phase != "blocked" || failed.StepID != execution.CurrentStep || failed.Status != "failed" && failed.Status != "rejected" {
		return nativeErr("stale_version", "current native task, workflow, execution or failed attempt changed")
	}
	workspace, err := s.inspectMinimalWorkspace(ctx, run)
	if err != nil {
		return fmt.Errorf("native retry workspace could not be verified: %w", err)
	}
	if workspace.Dirty || workspace.Head != failed.InputRevision {
		return nativeErr("contract_mismatch", "native retry requires a clean fixed workspace at the failed attempt input revision")
	}
	if _, err = s.requireBackendCapabilities(ctx, backendFor(s.Config, BeadsContext{}), "start", "observe", "stop", "workspace_inspection", "output_reading"); err != nil {
		return err
	}
	page, err := (QueryService{Store: s.domainStore, Project: s.Config.identity}).ShowNativeTask(ctx, task.TaskID)
	if err != nil {
		return err
	}
	if len(page.Tasks) != 1 || page.Tasks[0].Delivery != nil || page.Tasks[0].TaskRecord.Version != task.Version {
		return nativeErr("stale_version", "native task delivery or version changed before retry")
	}
	requestedBy := strings.TrimSpace(s.Config.Claimant)
	requestID := nativeRetryRequestID(task.TaskID, workflow.WorkflowRunID, execution.ExecutionID, failed.AttemptID)
	receipt, err := (CommandService{Store: s.domainStore, Project: s.Config.identity}).RetryNativeStep(ctx, NativeStepRetryOptions{RequestID: requestID, TaskID: task.TaskID, ExpectedTaskVersion: task.Version, WorkflowRunID: workflow.WorkflowRunID, ExecutionID: execution.ExecutionID, FailedAttemptID: failed.AttemptID, RequestedBy: requestedBy})
	if err != nil {
		return err
	}
	if receipt.Status != "planned" || receipt.PreviousAttempt != failed.AttemptID || receipt.StepID != failed.StepID || receipt.AttemptID == "" {
		return nativeErr("inconsistent_delivery", "native retry command returned an invalid attempt receipt")
	}
	if err = s.refreshNativeSnapshot(ctx, true); err != nil {
		return err
	}
	run = s.find(task.TaskID)
	if run == nil {
		return nativeErr("inconsistent_delivery", "retried native task disappeared from the center projection")
	}
	if run.Process != nil {
		return nativeErr("inconsistent_delivery", "planned native retry still carries the settled worker identity")
	}
	workflow, execution = s.minimalRecords(run)
	attempt := s.minimalAttempt(run, receipt.AttemptID)
	if workflow == nil || execution == nil || attempt == nil || attempt.Status != "planned" || execution.CurrentStep != receipt.StepID {
		return nativeErr("inconsistent_delivery", "planned native retry cannot be reconstructed from center records")
	}
	s.State.SlotOccupied = true
	s.State.CurrentTask = ptr(task.TaskID)
	if err = s.save(); err != nil {
		return err
	}
	workflow, execution = s.minimalRecords(run)
	attempt = s.minimalAttempt(run, receipt.AttemptID)
	if workflow == nil || execution == nil || attempt == nil || attempt.Status != "planned" {
		return nativeErr("inconsistent_delivery", "planned native retry changed during runtime projection publication")
	}
	return s.startMinimalStep(ctx, nativeTaskModel(*task), run, BeadsContext{}, attempt)
}

func nativeRetryRequestID(taskID, workflowRunID, executionID, failedAttemptID string) string {
	return digestValue(struct {
		Action, TaskID, WorkflowRunID, ExecutionID, FailedAttemptID string
	}{"retry_step", taskID, workflowRunID, executionID, failedAttemptID})[:32]
}
