package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/erranliu/SteerLine/internal/steerline/publication"
	"os"
	"path/filepath"
	"strings"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/workspace"
)

func (s *Service) refreshNativeSnapshot(ctx context.Context) error {
	if s.domainStore == nil {
		return errors.New("native service store is required")
	}
	snapshot, err := s.domainStore.Query(ctx, s.Config.identity)
	if err != nil {
		return err
	}
	for _, task := range snapshot.Tasks {
		if task.Source != "native" || task.ExternalProvider != "" || task.ExternalProject != "" || task.ExternalID != "" || task.IdentityMapDigest != "" {
			return nativeErr("authority_mismatch", "non-native task identity")
		}
		if task.Status == "in_progress" && latestNativeWorkflow(snapshot.WorkflowRuns, task.TaskID) == nil {
			return nativeErr("inconsistent_delivery", "claimed task has no workflow")
		}
	}
	for _, workflow := range snapshot.WorkflowRuns {
		if workflow.WorkflowVersion != WorkerFlowVersion {
			return nativeErr("unsupported_schema", "unsupported workflow version")
		}
	}
	authority, err := s.domainStore.nativeAuthority(ctx, s.Config.identity)
	if err != nil {
		return err
	}
	s.domain = snapshot
	s.workerDecisionWrites = nil
	s.pendingOperations = nil
	var copyErr error
	s.committed, copyErr = copyDomainSnapshot(snapshot)
	if copyErr != nil {
		return copyErr
	}
	s.State.Control.DispatchMode, s.State.Control.DispatchPaused, s.State.Control.Revision = authority.DispatchMode, authority.DispatchPaused, authority.AuthorityVersion
	s.State.SchemaVersion = WorkerFlowRuntimeSchema
	return s.projectState()
}

func latestNativeWorkflow(workflows []WorkflowRunRecord, taskID string) *WorkflowRunRecord {
	var latest *WorkflowRunRecord
	for i := range workflows {
		workflow := &workflows[i]
		if workflow.TaskID != taskID {
			continue
		}
		if latest == nil || workflow.CreatedAt.After(latest.CreatedAt) || workflow.CreatedAt.Equal(latest.CreatedAt) && workflow.WorkflowRunID > latest.WorkflowRunID {
			latest = workflow
		}
	}
	return latest
}

func latestWorkflowExecution(executions []ExecutionRecord, workflowRunID string) *ExecutionRecord {
	var latest *ExecutionRecord
	for i := range executions {
		execution := &executions[i]
		if execution.WorkflowRunID != workflowRunID {
			continue
		}
		if latest == nil || execution.CreatedAt.After(latest.CreatedAt) || execution.CreatedAt.Equal(latest.CreatedAt) && execution.ExecutionID > latest.ExecutionID {
			latest = execution
		}
	}
	return latest
}

func latestCurrentNativeAttempt(attempts []StepAttemptRecord, execution *ExecutionRecord) *StepAttemptRecord {
	if execution == nil || execution.CurrentStep == "" {
		return nil
	}
	var latest *StepAttemptRecord
	for i := range attempts {
		attempt := &attempts[i]
		if attempt.ExecutionID != execution.ExecutionID || attempt.StepID != execution.CurrentStep {
			continue
		}
		if latest == nil || attempt.AttemptNumber > latest.AttemptNumber {
			latest = attempt
		}
	}
	return latest
}

func latestNativeExecutionAttempt(attempts []StepAttemptRecord, executionID string) *StepAttemptRecord {
	var latest *StepAttemptRecord
	for i := range attempts {
		attempt := &attempts[i]
		if attempt.ExecutionID != executionID {
			continue
		}
		if latest == nil || attempt.CreatedAt.After(latest.CreatedAt) || attempt.CreatedAt.Equal(latest.CreatedAt) && attempt.AttemptID > latest.AttemptID {
			latest = attempt
		}
	}
	return latest
}

func (s *Service) reconcileNativeCleanup(ctx context.Context) error {
	var cleanupErr error
	for i := range s.domain.Tasks {
		task := s.domain.Tasks[i]
		if task.Status != "discarded" || len(task.CleanupPending) == 0 {
			continue
		}
		for _, item := range append([]string(nil), task.CleanupPending...) {
			if strings.HasPrefix(item, "pull_request_close:") {
				if err := s.reconcileNativePRCleanup(ctx, task, item); err != nil {
					if run := s.find(task.TaskID); run != nil {
						s.executionRecord(run).Error = ptr("discarded PR cleanup requires reconciliation: " + clip(err.Error(), 1000))
						s.refreshRun(run)
						if commitErr := s.commitWorker(run); commitErr != nil {
							return commitErr
						}
					}
					cleanupErr = errors.Join(cleanupErr, err)
					continue
				}
				if err := s.refreshNativeSnapshot(ctx); err != nil {
					return err
				}
				return nil
			}
			if strings.HasPrefix(item, "workspace_cleanup:") {
				stopPending := false
				for _, pending := range task.CleanupPending {
					if strings.HasPrefix(pending, "execution_stop_manual:") {
						stopPending = true
						break
					}
				}
				if stopPending {
					continue
				}
				if err := s.reconcileNativeWorkspaceCleanup(ctx, task, item); err == nil {
					if err := s.refreshNativeSnapshot(ctx); err != nil {
						return err
					}
					return nil
				} else {
					if run := s.find(task.TaskID); run != nil {
						s.executionRecord(run).Error = ptr("discarded workspace cleanup requires reconciliation: " + clip(err.Error(), 1000))
						s.refreshRun(run)
						if commitErr := s.commitWorker(run); commitErr != nil {
							return commitErr
						}
					}
					cleanupErr = errors.Join(cleanupErr, err)
					continue
				}
			}
			if !strings.HasPrefix(item, "execution_stop_manual:") {
				continue
			}
			executionID := strings.TrimPrefix(item, "execution_stop_manual:")
			if run := s.find(task.TaskID); run != nil && run.ExecutionID == executionID && s.workers[run.RunID] != nil {
				config, configErr := s.executionConfigForRun(run)
				if configErr != nil {
					return configErr
				}
				if config.hostName() != "local" {
					continue // Remote cancellation is not available; keep the exact manual item.
				}
				worker := s.workers[run.RunID]
				if _, err := s.requireBackendCapabilitiesFor(ctx, backendFor(config), targetFor(config), agentFor(config), "stop", "observe"); err != nil {
					s.executionRecord(run).Error = ptr("discarded task worker remains active; verified local stop capability is unavailable: " + clip(err.Error(), 1000))
					s.refreshRun(run)
					if commitErr := s.commitWorker(run); commitErr != nil {
						return commitErr
					}
					continue
				}
				op, stopErr := s.stopRunExecution(ctx, run, worker)
				if stopErr != nil {
					s.executionRecord(run).Error = ptr("discarded task worker stop requires reconciliation: " + clip(stopErr.Error(), 1000))
					s.refreshRun(run)
					if commitErr := s.commitWorker(run); commitErr != nil {
						return commitErr
					}
					continue
				}
				if op.Status != executionapi.OperationConfirmed {
					backend := backendFor(config)
					observation, observeErr := backend.Observe(ctx, *op.Ref)
					if observeErr != nil || observation.Liveness != "exited" {
						s.executionRecord(run).Error = ptr("discarded task worker stop is awaiting confirmed process absence")
						s.refreshRun(run)
						if commitErr := s.commitWorker(run); commitErr != nil {
							return commitErr
						}
						continue
					}
					confirmed, confirmErr := s.confirmExecutionStop(op)
					if !confirmed {
						s.executionRecord(run).Error = ptr("discarded task worker stop is not confirmed: " + clip(fmt.Sprint(confirmErr), 1000))
						s.refreshRun(run)
						if commitErr := s.commitWorker(run); commitErr != nil {
							return commitErr
						}
						continue
					}
				}
				if err := s.recordNativeStopCleanup(ctx, task.TaskID, item, op); err != nil {
					s.executionRecord(run).Error = ptr("discarded task stop receipt could not be committed: " + clip(err.Error(), 1000))
					s.refreshRun(run)
					if commitErr := s.commitWorker(run); commitErr != nil {
						return commitErr
					}
					continue
				}
				if err := s.refreshNativeSnapshot(ctx); err != nil {
					return err
				}
				if s.workers[run.RunID] == worker {
					delete(s.workers, run.RunID)
					s.State.CurrentTask = nil
					s.State.SlotOccupied = false
					if err := s.publishCheckpoint(); err != nil {
						return err
					}
				}
				return nil
			}
			// After a restart there is no local Job/process-group handle to stop.
			// Observe the exact saved identity and settle only if it is already absent.
			if err := s.reconcileExitedNativeExecution(ctx, task.TaskID, item, executionID); err != nil {
				continue
			}
			return nil
		}
	}
	return cleanupErr
}

func (s *Service) reconcileNativePRCleanup(ctx context.Context, task TaskRecord, cleanupItem string) error {
	prURL := strings.TrimPrefix(cleanupItem, "pull_request_close:")
	run := s.find(task.TaskID)
	if run == nil {
		return nativeErr("inconsistent_delivery", "discarded PR cleanup has no reconstructed workflow run")
	}
	workflow, execution := s.workflowRecords(run)
	attempt := s.workflowAttempt(run, run.AttemptID)
	if workflow == nil || execution == nil || attempt == nil || workflow.WorkflowVersion != WorkerFlowVersion || attempt.StepID != "worker" || workflow.PRURL != prURL || execution.SessionID == "" {
		return nativeErr("authority_mismatch", "discarded PR cleanup lacks its fixed worker flow, session and PR URL")
	}
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return err
	}
	info, err := publicationClient(fixed.config).Query(ctx, prURL)
	if err != nil || info.State == "unknown" {
		return errors.Join(err, nativeErr("merge_state_unknown", "discarded PR state could not be verified"))
	}
	if err := publication.ValidateWorkerPRIdentity(info, prURL, fixed.config.PR.Repository, workflow.Branch, execution.Workspace.Branch); err != nil {
		return err
	}
	if info.Merged || info.State == "merged" {
		return nativeErr("task_discarded", "discarded task PR is already merged; preserve delivery evidence for manual recovery")
	}
	if info.State == "open" {
		c := fixed.config
		if err := c.bindPublicationTarget(ctx); err != nil {
			return err
		}
		info, err = publicationClient(c).Close(ctx, prURL)
		if err != nil {
			var providerErr *publication.Error
			if errors.As(err, &providerErr) {
				return nativeErr(providerErr.Code, providerErr.Message)
			}
			return err
		}
	}
	if err := publication.ValidateWorkerPRIdentity(info, prURL, fixed.config.PR.Repository, workflow.Branch, execution.Workspace.Branch); err != nil {
		return err
	}
	if info.State != "closed_unmerged" || info.Merged {
		return nativeErr("merge_state_unknown", "discarded PR closure did not produce a verified closed_unmerged receipt")
	}
	evidence := NativePRCleanupReceipt{WorkflowRunID: workflow.WorkflowRunID, ExecutionID: execution.ExecutionID, AttemptID: attempt.AttemptID, PRURL: info.URL, Repository: info.Repository, BaseBranch: info.BaseBranch, HeadRevision: info.HeadRevision, State: info.State, Merged: info.Merged, ObservedAt: now()}
	data, _ := json.Marshal(evidence)
	requestID := digestValue(struct{ TaskID, Item string }{task.TaskID, cleanupItem})[:32]
	_, err = (CommandService{Store: s.domainStore, Project: s.Config.identity}).RecordNativeCleanupReceipt(ctx, task.TaskID, requestID, task.Version, cleanupItem, s.Config.Claimant, data)
	return err
}
func (s *Service) reconcileNativeWorkspaceCleanup(ctx context.Context, task TaskRecord, cleanupItem string) error {
	path := strings.TrimPrefix(cleanupItem, "workspace_cleanup:")
	var execution *ExecutionRecord
	var workflow *WorkflowRunRecord
	for i := range s.domain.Executions {
		candidate := &s.domain.Executions[i]
		matches := candidate.Target.WorktreeID == path || candidate.Workspace.Worktree == path
		if !matches && filepath.IsAbs(path) && hostForExecution(candidate.Backend, candidate.Protocol) == "local" {
			matches = canonical(candidate.Target.WorktreeID) == canonical(path) || canonical(candidate.Workspace.Worktree) == canonical(path)
		}
		if !matches {
			continue
		}
		for j := range s.domain.WorkflowRuns {
			if s.domain.WorkflowRuns[j].WorkflowRunID == candidate.WorkflowRunID && s.domain.WorkflowRuns[j].TaskID == task.TaskID {
				execution, workflow = candidate, &s.domain.WorkflowRuns[j]
				break
			}
		}
	}
	if execution == nil || workflow == nil || execution.Target.Branch == "" || !validCommit(execution.Target.BaseRevision) {
		return nativeErr("contract_mismatch", "discarded workspace path lacks a fixed branch and base identity")
	}
	if hostForExecution(execution.Backend, execution.Protocol) != "local" {
		return executionapi.UnsupportedAdapterOperation("WorkspaceCleanup", "remote native worktree cleanup is unavailable")
	}
	if !filepath.IsAbs(path) {
		return nativeErr("authority_mismatch", "discarded local workspace cleanup path is not absolute")
	}
	if err := s.Config.prepareWorktreeRoot(); err != nil {
		return err
	}
	resolved, err := resolveProspectivePath(path)
	if err != nil || canonical(filepath.Dir(resolved)) != canonical(s.Config.WorktreeRoot) {
		return nativeErr("authority_mismatch", "discarded workspace cleanup path no longer matches worktree_root")
	}
	intent := &executionapi.WorkspaceIntent{Path: resolved, Branch: execution.Target.Branch, BaseCommit: execution.Target.BaseRevision}
	repository, err := workspaceRepository(s.Config)
	if err != nil {
		return err
	}
	result, err := workspace.Cleanup(ctx, workspaceGit(s.Config), repository, s.Config.WorktreeRoot, intent)
	receipt := NativeWorkspaceCleanupReceipt{WorkflowRunID: workflow.WorkflowRunID, ExecutionID: execution.ExecutionID, Repository: execution.Target.ID, Worktree: resolved, Branch: intent.Branch, BaseCommit: intent.BaseCommit, WorktreeGone: result.WorktreeGone, BranchGone: result.BranchGone, ObservedAt: now()}
	data, _ := json.Marshal(receipt)
	requestID := digestValue(struct{ TaskID, Item string }{task.TaskID, cleanupItem})[:32]
	if err != nil {
		progressErr := (CommandService{Store: s.domainStore, Project: s.Config.identity}).RecordNativeWorkspaceCleanupProgress(ctx, task.TaskID, requestID, task.Version, cleanupItem, s.Config.Claimant, data)
		return errors.Join(err, progressErr)
	}
	_, err = (CommandService{Store: s.domainStore, Project: s.Config.identity}).RecordNativeCleanupReceipt(ctx, task.TaskID, requestID, task.Version, cleanupItem, s.Config.Claimant, data)
	return err
}

func (s *Service) recordNativeStopCleanup(ctx context.Context, taskID, cleanupItem string, op executionapi.Operation) error {
	if op.Ref == nil {
		return &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "native stop operation has no fixed execution reference"}
	}
	confirmed, err := s.confirmExecutionStop(op)
	if err != nil || !confirmed {
		return errors.Join(err, &executionapi.BackendError{Code: executionapi.BackendOutcomeUnknown, Diagnostic: "native stop operation is not confirmed"})
	}
	records := executionRecords{root: s.Config.RunDir}
	persisted, err := records.loadOperation(op.RunID, op.AttemptID, op.ID)
	if err != nil || persisted.Status != executionapi.OperationConfirmed {
		return errors.Join(err, nativeErr("outcome_unknown", "confirmed stop was not durably recorded"))
	}
	config, configErr := s.executionConfigForWorkflowID(op.RunID)
	if configErr != nil {
		return configErr
	}
	backend := backendFor(config)
	observation, err := backend.Observe(ctx, *op.Ref)
	if err != nil || observation.Liveness != "exited" {
		return errors.Join(err, nativeErr("outcome_unknown", "fixed local execution is not confirmed absent"))
	}
	evidence := NativeStopCleanupReceipt{WorkflowRunID: op.RunID, ExecutionID: nativeExecutionIDForAttempt(s.domain.Attempts, op.AttemptID), AttemptID: op.AttemptID, Operation: persisted, Observation: observation}
	if evidence.ExecutionID == "" {
		return nativeErr("contract_mismatch", "stop attempt is not bound to a native execution")
	}
	task := nativeTaskRecord(s.domain.Tasks, taskID)
	if task == nil {
		return nativeErr("task_not_found", "discarded native task disappeared during stop reconciliation")
	}
	data, _ := json.Marshal(evidence)
	requestID := digestValue(struct{ TaskID, Item, OperationID string }{taskID, cleanupItem, op.ID})[:32]
	_, err = (CommandService{Store: s.domainStore, Project: s.Config.identity}).RecordNativeCleanupReceipt(ctx, taskID, requestID, task.Version, cleanupItem, s.Config.Claimant, data)
	return err
}

func (s *Service) reconcileExitedNativeExecution(ctx context.Context, taskID, cleanupItem, executionID string) error {
	var execution *ExecutionRecord
	for i := range s.domain.Executions {
		if s.domain.Executions[i].ExecutionID == executionID {
			execution = &s.domain.Executions[i]
			break
		}
	}
	if execution == nil || execution.Process == nil {
		return nativeErr("outcome_unknown", "discarded task has no fixed local process identity")
	}
	if hostForExecution(execution.Backend, execution.Protocol) != "local" {
		return executionapi.UnsupportedAdapterOperation("StopExecution", "remote native worker cancellation is unavailable")
	}
	workflowRunID := execution.WorkflowRunID
	config, configErr := s.executionConfigForWorkflowID(workflowRunID)
	if configErr != nil {
		return configErr
	}
	var attempt *StepAttemptRecord
	for i := range s.domain.Attempts {
		candidate := &s.domain.Attempts[i]
		if candidate.ExecutionID == executionID && candidate.Status != "succeeded" && candidate.Status != "failed" && candidate.Status != "rejected" && candidate.Status != "stopped" {
			if attempt == nil || candidate.AttemptNumber > attempt.AttemptNumber {
				attempt = candidate
			}
		}
	}
	if attempt == nil {
		return nativeErr("outcome_unknown", "discarded task has no active local step attempt")
	}
	ref, err := (executionRecords{root: s.Config.RunDir}).loadBinding(workflowRunID, attempt.AttemptID)
	if err != nil {
		return err
	}
	backend := backendFor(config)
	observation, err := backend.Observe(ctx, ref)
	if err != nil || observation.Liveness != "exited" {
		return errors.Join(err, nativeErr("outcome_unknown", "fixed local process is live or its state is unknown; it cannot be stopped without the original handle"))
	}
	opID := "stop-" + attempt.AttemptID
	payload := struct {
		Policy string
		Grace  string
	}{"fixed_local_execution_stop", s.Config.ShutdownGrace}
	op := executionapi.Operation{Version: executionapi.ExecutionRecordVersion, ID: opID, RunID: workflowRunID, AttemptID: attempt.AttemptID, Backend: executionapi.BackendLocal, Protocol: ref.Protocol, Target: ref.Target, Action: "stop", PayloadDigest: digestValue(payload), Status: executionapi.OperationConfirmed, StartedAt: now(), UpdatedAt: now(), Diagnostic: observation.Diagnostic, Ref: &ref}
	records := executionRecords{root: s.Config.RunDir}
	expectedOperation := ""
	if old, loadErr := records.loadOperation(workflowRunID, attempt.AttemptID, opID); loadErr == nil {
		expectedOperation = recordFingerprint(old)
		if old.PayloadDigest != op.PayloadDigest || old.Ref == nil || digestValue(*old.Ref) != digestValue(ref) {
			return nativeErr("authority_mismatch", "saved local stop operation differs from the fixed execution identity")
		}
		if old.Status != executionapi.OperationConfirmed {
			old.Status, old.UpdatedAt, old.Diagnostic = executionapi.OperationConfirmed, now(), observation.Diagnostic
			op = old
		}
	} else if !os.IsNotExist(loadErr) {
		return loadErr
	}
	if err := s.commitExecutionOperation(s.localWorkerRun(workflowRunID), op, expectedOperation); err != nil {
		return err
	}
	return s.recordNativeStopCleanup(ctx, taskID, cleanupItem, op)
}

func nativeExecutionIDForAttempt(attempts []StepAttemptRecord, attemptID string) string {
	for _, attempt := range attempts {
		if attempt.AttemptID == attemptID {
			return attempt.ExecutionID
		}
	}
	return ""
}

func nativeRuntimeFinishStatus(task TaskRecord, workflow WorkflowRunRecord) string {
	switch {
	case task.Status == "closed" && workflow.Phase == "blocked":
		return "blocked"
	case task.Status == "closed" || workflow.Phase == "completed":
		return "completed"
	case workflow.Phase == "waiting_pr_merge":
		return "waiting_pr_merge"
	case workflow.Phase == "waiting_decision":
		return "waiting_decision"
	case workflow.Phase == "blocked" || task.Status == "discarded":
		return "blocked"
	default:
		return "pending"
	}
}

func (s *Service) dispatchNative(ctx context.Context) error {
	if !s.explicitDispatch && len(s.workers) != 0 {
		return nil
	}
	if err := s.refreshNativeSnapshot(ctx); err != nil {
		return err
	}
	if !s.explicitDispatch && (s.State.Control.DispatchMode != "auto" || s.State.Control.DispatchPaused) {
		return nil
	}
	if !s.explicitDispatch && s.State.SlotOccupied && s.activeControlRequestID() == "" {
		return nil
	}
	if !s.explicitDispatch {
		if reason := s.slotReason(s.activeControlRequestID()); reason != "" {
			return errors.New(reason)
		}
	}
	if !s.explicitDispatch {
		if err := s.selectDefaultDispatchProfile(); err != nil {
			return err
		}
	} else if s.dispatchProfile == nil {
		if err := s.selectDispatchProfile(ProjectExecutionProfileID, ""); err != nil {
			return err
		}
	}
	if s.dispatchConfig == nil {
		return errors.New("native dispatch has no selected execution profile")
	}
	c := *s.dispatchConfig
	defer func() {
		s.dispatchConfig = nil
		s.dispatchProfile = nil
	}()
	ready, err := (NewQueryService(s.domainStore, c.identity)).ReadyNativeTasks(ctx)
	if err != nil {
		return err
	}
	if len(ready) > 0 {
		if c.WorkflowProtocol == WorkerFlowVersion {
			if err = requireWorkerFlowCapabilities(ctx, c); err != nil {
				return err
			}
		} else {
			required := []string{"start", "observe", "workspace_inspection", "output_reading"}
			if c.hostName() == "orca_supervised" {
				required = append(required, "message_inbox", "resource_settlement")
			} else {
				required = append(required, "stop")
			}
			if _, err = s.requireBackendCapabilitiesFor(ctx, backendFor(c), targetFor(c), agentFor(c), required...); err != nil {
				return err
			}
		}
	}
	for _, task := range ready {
		if len(c.ExecutableTypes) > 0 && !contains(c.ExecutableTypes, task.Type) {
			continue
		}
		if s.targetTask != "" && s.targetTask != task.TaskID {
			continue
		}
		authority, authErr := s.domainStore.nativeAuthority(ctx, c.identity)
		if authErr != nil {
			return authErr
		}
		requestID := newID()
		if s.explicitDispatch && s.activeControlRequestID() != "" {
			requestID = s.activeControlRequestID()
		}
		workflowRunID := ""
		if s.reservedRunID != "" {
			workflowRunID = s.reservedRunID
		}
		receipt, claimErr := (CommandService{Store: s.domainStore, Project: c.identity}).ClaimNativeTask(ctx, c, TaskClaimOptions{TaskID: task.TaskID, RequestID: requestID, ExpectedTaskVersion: task.Version, ExpectedAuthorityVersion: authority.AuthorityVersion, RequestedBy: c.Claimant, Explicit: s.explicitDispatch, WorkflowRunID: workflowRunID, ExecutionProfile: s.dispatchProfile, ServiceInstanceID: s.State.Service.InstanceID})
		if claimErr != nil {
			if isNativeErrorCode(claimErr, "task_not_ready", "stale_version", "slot_busy", "dispatch_paused", "wrong_mode", "task_discarded") {
				if s.targetTask != "" {
					return claimErr
				}
				continue
			}
			return claimErr
		}
		if receipt.Status != "claim_reserved" || receipt.StartTaskApplied {
			return nativeErr("inconsistent_delivery", "native claim returned an invalid reservation status")
		}
		if err = s.refreshNativeSnapshot(ctx); err != nil {
			return err
		}
		run := s.find(task.TaskID)
		workflow, execution := s.workflowRecords(run)
		attempt := s.workflowAttempt(run, run.AttemptID)
		if run == nil || workflow == nil || execution == nil || attempt == nil {
			return nativeErr("inconsistent_delivery", "native claim reservation could not be reconstructed from center records")
		}
		intent := execution.WorkspaceIntent
		if intent == nil {
			return nativeErr("inconsistent_delivery", "claimed execution lacks its workspace intent")
		}
		if execution.Target.WorktreeID != "" || execution.Target.Branch != c.Branch {
			return nativeErr("authority_mismatch", "claimed execution already has a conflicting workspace identity")
		}
		if c.hostName() != "orca_supervised" {
			execution.Target.WorktreeID = intent.Path
			execution.Target.Branch = intent.Branch
		}
		execution.UpdatedAt = now()
		execution.OwnerServiceInstanceID = s.State.Service.InstanceID
		workflow.UpdatedAt = now()
		attempt.ExternalBinding = execution.Target
		attempt.UpdatedAt = now()
		execution.WorkspaceIntent = intent
		s.refreshRun(run)
		s.executionRecord(run).Status = "pending"
		s.refreshRun(run)
		s.State.SlotOccupied = true
		s.State.CurrentTask = ptr(task.TaskID)
		if err = s.commitWorker(run); err != nil {
			return err
		}
		s.lastDispatched = task.TaskID
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return s.prepareWorkerFlow(ctx, task, run)
	}
	if s.targetTask != "" {
		return fmt.Errorf("%w: %s is not in the current ready native task set", errTaskNotReady, s.targetTask)
	}
	return nil
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func isNativeErrorCode(err error, codes ...string) bool {
	var native *NativeTaskError
	if !errors.As(err, &native) {
		return false
	}
	for _, code := range codes {
		if native.Code == code {
			return true
		}
	}
	return false
}

func (s *Service) nativeCompleteMergedTask(ctx context.Context, run *Run) error {
	if run == nil || run.PR.URL == nil {
		return nativeErr("merge_state_unknown", "native task has no fixed PR URL")
	}
	task := nativeTaskRecord(s.domain.Tasks, run.DomainTaskID)
	workflow, _ := s.workflowRecords(run)
	if task == nil || workflow == nil {
		return nativeErr("inconsistent_delivery", "native delivery task or workflow record is missing")
	}
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return err
	}
	receipt, err := (CommandService{Store: s.domainStore, Project: fixed.config.identity}).CompleteNativeTask(ctx, fixed.config, task.TaskID, task.Version, workflow.WorkflowRunID, publicationClient(fixed.config))
	if err != nil {
		return err
	}
	if receipt.Status != "closed" || receipt.Delivery.TaskID != run.DomainTaskID {
		return nativeErr("inconsistent_delivery", "native delivery transaction returned an invalid task receipt")
	}
	if err = s.refreshNativeSnapshot(ctx); err != nil {
		return err
	}
	run = s.find(receipt.TaskID)
	if run == nil {
		return nativeErr("inconsistent_delivery", "closed native task disappeared from the execution projection")
	}
	s.executionRecord(run).Error = nil
	s.refreshRun(run)
	s.note(run, "steer", "delivery", "completed", "native task delivered after verified PR merge", receipt.Delivery.PRURL, map[string]any{"merge_commit": receipt.Delivery.MergeCommit, "graph_revision": receipt.GraphRevision})
	s.closeActivity(run)
	if err := s.commitWorker(run); err != nil {
		return err
	}
	if run.Protocol == string(executionapi.ProtocolOrcaTerminalV2) || run.Protocol == string(executionapi.ProtocolOrcaSupervised) {
		var cleanupErr error
		if run.Protocol == string(executionapi.ProtocolOrcaSupervised) {
			cleanupErr = s.finishRemoteOrcaCleanup(ctx, run)
		} else {
			cleanupErr = s.finishNativeOrcaCleanup(ctx, run)
		}
		if err := cleanupErr; err != nil {
			s.executionRecord(run).LastEvent = ptr("Orca delivery completed; owned terminal cleanup needs reconciliation: " + err.Error())
			s.refreshRun(run)
			return s.commitWorker(run)
		}
	}
	return nil
}

func (s *Service) pollNativePRs(ctx context.Context, onlyRunID string) error {
	var operationErr error
	for _, run := range s.runs {
		if run == nil || onlyRunID != "" && run.RunID != onlyRunID {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		task := nativeTaskRecord(s.domain.Tasks, run.DomainTaskID)
		if task == nil {
			continue
		}
		workflow, _ := s.workflowRecords(run)
		if workflow == nil {
			continue
		}
		if workflow.WorkflowVersion != WorkerFlowVersion {
			return nativeErr("unsupported_schema", "PR polling found an unsupported workflow")
		}
		if err := s.pollWorkerFlowPR(ctx, run, task, workflow); err != nil {
			operationErr = errors.Join(operationErr, err)
		}
	}
	return operationErr
}

func nativeTaskRecord(tasks []TaskRecord, taskID string) *TaskRecord {
	for i := range tasks {
		if tasks[i].TaskID == taskID {
			return &tasks[i]
		}
	}
	return nil
}
