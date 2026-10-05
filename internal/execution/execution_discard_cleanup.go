package execution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
	"os"
	"path/filepath"
	"strings"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/execution/publication"
	"github.com/erranliu/SteerLine/internal/execution/workspace"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

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
			if run := s.findExecution(executionID); run != nil && s.workers[run.RunID] != nil {
				config, configErr := s.executionConfigForRun(run)
				if configErr != nil {
					return configErr
				}
				if config.hostName() != "local" {
					continue // Remote cancellation is not available; keep the exact manual item.
				}
				worker := s.workers[run.RunID]
				if _, err := s.requireBackendCapabilitiesFor(ctx, BackendFor(config), TargetFor(config), AgentFor(config), "stop", "observe"); err != nil {
					s.executionRecord(run).Error = ptr("discarded task worker remains active; verified local stop capability is unavailable: " + clip(err.Error(), 1000))
					s.refreshRun(run)
					if commitErr := s.commitWorker(run); commitErr != nil {
						return commitErr
					}
					continue
				}
				op, stopErr := s.stopRunExecution(ctx, run, worker)
				if errors.Is(stopErr, errLifecycleEffectPending) {
					continue
				}
				if stopErr != nil {
					s.executionRecord(run).Error = ptr("discarded task worker stop requires reconciliation: " + clip(stopErr.Error(), 1000))
					s.refreshRun(run)
					if commitErr := s.commitWorker(run); commitErr != nil {
						return commitErr
					}
					continue
				}
				if op.Status != executionbackend.OperationConfirmed {
					backend := BackendFor(config)
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

func (s *Service) reconcileNativePRCleanup(ctx context.Context, task taskmodel.TaskRecord, cleanupItem string) error {
	prURL := strings.TrimPrefix(cleanupItem, "pull_request_close:")
	var run *reconstructiontypes.RunView
	for _, candidate := range s.runs {
		if candidate == nil || candidate.TaskID != task.TaskID {
			continue
		}
		w := s.workflowRecord(candidate)
		if w.PRURL == prURL || contains(w.PRHistory, prURL) {
			run = candidate
			break
		}
	}
	if run == nil {
		return nativeErr("inconsistent_delivery", "discarded PR cleanup has no reconstructed workflow run")
	}
	workflow, execution := s.workflowRecords(run)
	attempt := s.workflowAttempt(run, run.AttemptID)
	sessionID, _ := executionSessionFacts(execution)
	if workflow == nil || execution == nil || attempt == nil || workflow.WorkflowVersion != executionmodel.Version || attempt.StepID != "worker" || workflow.PRURL != prURL && !contains(workflow.PRHistory, prURL) || sessionID == "" {
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
		if err := c.BindPublicationTarget(ctx); err != nil {
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
	_, err = (s.commandService(s.Config.identity)).RecordNativeCleanupReceipt(ctx, task.TaskID, requestID, task.Version, cleanupItem, s.Config.Claimant, data)
	return err
}

func (s *Service) reconcileNativeWorkspaceCleanup(ctx context.Context, task taskmodel.TaskRecord, cleanupItem string) error {
	path := strings.TrimPrefix(cleanupItem, "workspace_cleanup:")
	var execution *executionmodel.ExecutionRecord
	var workflow *executionmodel.WorkflowRunRecord
	for i := range s.domain.Executions {
		candidate := &s.domain.Executions[i]
		matches := candidate.Target.WorktreeID == path || candidate.Workspace.Worktree == path
		if !matches && filepath.IsAbs(path) && executionUsesLocalWorkspace(s.Config, executionbackend.BackendKind(candidate.Backend), executionbackend.ExecutionProtocol(candidate.Protocol), candidate.Agent) {
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
	if execution == nil || workflow == nil || execution.Target.Branch == "" || !taskmodel.ValidCommit(execution.Target.BaseRevision) {
		return nativeErr("contract_mismatch", "discarded workspace path lacks a fixed branch and base identity")
	}
	if !executionUsesLocalWorkspace(s.Config, executionbackend.BackendKind(execution.Backend), executionbackend.ExecutionProtocol(execution.Protocol), execution.Agent) {
		return executionbackend.UnsupportedAdapterOperation("WorkspaceCleanup", "remote native worktree cleanup is unavailable")
	}
	if !filepath.IsAbs(path) {
		return nativeErr("authority_mismatch", "discarded local workspace cleanup path is not absolute")
	}
	if err := s.Config.PrepareWorktreeRoot(); err != nil {
		return err
	}
	resolved, err := resolveProspectivePath(path)
	if err != nil || canonical(filepath.Dir(resolved)) != canonical(s.Config.WorktreeRoot) {
		return nativeErr("authority_mismatch", "discarded workspace cleanup path no longer matches worktree_root")
	}
	intent := &executionbackend.WorkspaceIntent{Path: resolved, Branch: execution.Target.Branch, BaseCommit: execution.Target.BaseRevision}
	repository, err := workspaceRepository(s.Config)
	if err != nil {
		return err
	}
	result, err := workspace.Cleanup(ctx, workspaceGit(s.Config), repository, s.Config.WorktreeRoot, intent)
	receipt := NativeWorkspaceCleanupReceipt{WorkflowRunID: workflow.WorkflowRunID, ExecutionID: execution.ExecutionID, Repository: execution.Target.ID, Worktree: resolved, Branch: intent.Branch, BaseCommit: intent.BaseCommit, WorktreeGone: result.WorktreeGone, BranchGone: result.BranchGone, ObservedAt: now()}
	data, _ := json.Marshal(receipt)
	requestID := digestValue(struct{ TaskID, Item string }{task.TaskID, cleanupItem})[:32]
	if err != nil {
		progressErr := (s.commandService(s.Config.identity)).RecordNativeWorkspaceCleanupProgress(ctx, task.TaskID, requestID, task.Version, cleanupItem, s.Config.Claimant, data)
		return errors.Join(err, progressErr)
	}
	_, err = (s.commandService(s.Config.identity)).RecordNativeCleanupReceipt(ctx, task.TaskID, requestID, task.Version, cleanupItem, s.Config.Claimant, data)
	return err
}

func (s *Service) recordNativeStopCleanup(ctx context.Context, taskID, cleanupItem string, op executionbackend.Operation) error {
	if op.Ref == nil {
		return &executionbackend.BackendError{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "native stop operation has no fixed execution reference"}
	}
	confirmed, err := s.confirmExecutionStop(op)
	if err != nil || !confirmed {
		return errors.Join(err, &executionbackend.BackendError{Code: executionbackend.BackendOutcomeUnknown, Diagnostic: "native stop operation is not confirmed"})
	}
	records := executionRecords{root: s.Config.RunDir, reader: s.Config.ExecutionRecords, resources: s.Config.Resources}
	persisted, err := records.loadOperation(op.RunID, op.AttemptID, op.ID)
	if err != nil || persisted.Status != executionbackend.OperationConfirmed {
		return errors.Join(err, nativeErr("outcome_unknown", "confirmed stop was not durably recorded"))
	}
	config, configErr := s.executionConfigForWorkflowID(op.RunID)
	if configErr != nil {
		return configErr
	}
	backend := BackendFor(config)
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
	_, err = (s.commandService(s.Config.identity)).RecordNativeCleanupReceipt(ctx, taskID, requestID, task.Version, cleanupItem, s.Config.Claimant, data)
	return err
}

func (s *Service) reconcileExitedNativeExecution(ctx context.Context, taskID, cleanupItem, executionID string) error {
	var execution *executionmodel.ExecutionRecord
	for i := range s.domain.Executions {
		if s.domain.Executions[i].ExecutionID == executionID {
			execution = &s.domain.Executions[i]
			break
		}
	}
	if execution == nil || execution.Process == nil {
		return nativeErr("outcome_unknown", "discarded task has no fixed local process identity")
	}
	if !executionUsesLocalWorkspace(s.Config, executionbackend.BackendKind(execution.Backend), executionbackend.ExecutionProtocol(execution.Protocol), execution.Agent) {
		return executionbackend.UnsupportedAdapterOperation("StopExecution", "remote native worker cancellation is unavailable")
	}
	workflowRunID := execution.WorkflowRunID
	config, configErr := s.executionConfigForWorkflowID(workflowRunID)
	if configErr != nil {
		return configErr
	}
	var attempt *executionmodel.StepAttemptRecord
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
	ref, err := (executionRecords{root: s.Config.RunDir, reader: s.Config.ExecutionRecords, resources: s.Config.Resources}).loadBinding(workflowRunID, attempt.AttemptID)
	if err != nil {
		return err
	}
	backend := BackendFor(config)
	observation, err := backend.Observe(ctx, ref)
	if err != nil || observation.Liveness != "exited" {
		return errors.Join(err, nativeErr("outcome_unknown", "fixed local process is live or its state is unknown; it cannot be stopped without the original handle"))
	}
	opID := "stop-" + attempt.AttemptID
	payload := struct {
		Policy string
		Grace  string
	}{"fixed_local_execution_stop", s.Config.ShutdownGrace}
	op := executionbackend.Operation{Version: executionbackend.ExecutionRecordVersion, ID: opID, RunID: workflowRunID, AttemptID: attempt.AttemptID, Backend: executionbackend.BackendLocal, Protocol: ref.Protocol, Target: ref.Target, Action: "stop", PayloadDigest: digestValue(payload), Status: executionbackend.OperationConfirmed, StartedAt: now(), UpdatedAt: now(), Diagnostic: observation.Diagnostic, Ref: &ref}
	records := executionRecords{root: s.Config.RunDir, reader: s.Config.ExecutionRecords, resources: s.Config.Resources}
	expectedOperation := ""
	if old, loadErr := records.loadOperation(workflowRunID, attempt.AttemptID, opID); loadErr == nil {
		expectedOperation = recordFingerprint(old)
		if old.PayloadDigest != op.PayloadDigest || old.Ref == nil || digestValue(*old.Ref) != digestValue(ref) {
			return nativeErr("authority_mismatch", "saved local stop operation differs from the fixed execution identity")
		}
		if old.Status != executionbackend.OperationConfirmed {
			old.Status, old.UpdatedAt, old.Diagnostic = executionbackend.OperationConfirmed, now(), observation.Diagnostic
			op = old
		}
	} else if !os.IsNotExist(loadErr) {
		return loadErr
	}
	if err := s.commitExecutionOperation(s.find(workflowRunID), op, expectedOperation); err != nil {
		return err
	}
	return s.recordNativeStopCleanup(ctx, taskID, cleanupItem, op)
}

func nativeExecutionIDForAttempt(attempts []executionmodel.StepAttemptRecord, attemptID string) string {
	for _, attempt := range attempts {
		if attempt.AttemptID == attemptID {
			return attempt.ExecutionID
		}
	}
	return ""
}
