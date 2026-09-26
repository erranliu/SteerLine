package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (s *Service) refreshNativeSnapshot(ctx context.Context, reconstruct bool) error {
	if !s.native || s.domainStore == nil {
		return errors.New("native snapshot requires a native service store")
	}
	snapshot, err := s.domainStore.Query(ctx, s.Config.identity)
	if err != nil {
		return err
	}
	authority, err := s.domainStore.nativeAuthority(ctx, s.Config.identity)
	if err != nil {
		return err
	}
	s.State.Tasks = snapshot.Tasks
	s.State.WorkflowRuns = snapshot.WorkflowRuns
	s.State.Executions = snapshot.Executions
	s.State.Attempts = snapshot.Attempts
	s.State.Decisions = snapshot.Decisions
	s.State.Control.DispatchMode = authority.DispatchMode
	s.State.Control.DispatchPaused = authority.DispatchPaused
	s.State.Control.Revision = authority.AuthorityVersion
	for _, task := range snapshot.Tasks {
		if task.Status == "open" {
			continue
		}
		workflow := latestNativeWorkflow(snapshot.WorkflowRuns, task.TaskID)
		if workflow == nil {
			if task.Status == "in_progress" {
				return nativeErr("inconsistent_delivery", "claimed native task has no persisted WorkflowRun")
			}
			continue
		}
		execution := latestWorkflowExecution(snapshot.Executions, workflow.WorkflowRunID)
		if execution == nil {
			return nativeErr("inconsistent_delivery", "native WorkflowRun has no persisted Execution")
		}
		attempt := latestCurrentNativeAttempt(snapshot.Attempts, execution)
		run := s.find(task.TaskID)
		if run == nil {
			run = &Run{TaskID: task.TaskID, DomainTaskID: task.TaskID, Title: task.Title, RunID: workflow.WorkflowRunID, Claimant: task.Claimant, Host: s.Config.hostName(), Backend: execution.Backend, Protocol: execution.Protocol, Agent: execution.Agent, Target: execution.Target, WorkflowVersion: workflow.WorkflowVersion, WorkflowConfigDigest: workflow.ConfigDigest, ExecutionID: execution.ExecutionID, PR: PRState{State: "unknown"}}
			s.State.Runs = append(s.State.Runs, run)
		}
		run.TaskID, run.DomainTaskID, run.Title = task.TaskID, task.TaskID, task.Title
		run.RunID, run.ExecutionID = workflow.WorkflowRunID, execution.ExecutionID
		run.Claimant, run.Host = task.Claimant, s.Config.hostName()
		run.TaskStatus = task.Status
		run.Backend, run.Protocol, run.Agent = execution.Backend, execution.Protocol, execution.Agent
		run.Target = execution.Target
		run.WorkflowVersion, run.WorkflowConfigDigest = workflow.WorkflowVersion, workflow.ConfigDigest
		run.BeadsContext, run.Completion = nil, nil
		run.BeadsStatus = ""
		if attempt != nil {
			run.AttemptID, run.DomainAttemptNumber = attempt.AttemptID, attempt.AttemptNumber
		} else {
			if last := latestNativeExecutionAttempt(snapshot.Attempts, execution.ExecutionID); last != nil {
				run.AttemptID, run.DomainAttemptNumber = last.AttemptID, last.AttemptNumber
			} else {
				run.AttemptID = ""
			}
		}
		base := execution.Workspace.Base
		if base == "" {
			base = execution.Target.BaseRevision
		}
		path := execution.Workspace.Worktree
		if path == "" {
			path = execution.Target.WorktreeID
		}
		branch := execution.Workspace.Branch
		if branch == "" {
			branch = execution.Target.Branch
		}
		if path != "" && branch != "" && base != "" {
			intent := WorkspaceIntent{Path: path, Branch: branch, BaseCommit: base}
			run.WorkspaceIntent = &intent
			run.WorktreePath, run.WorktreeBranch, run.BaseCommit = ptr(path), ptr(branch), ptr(base)
		} else if base != "" && execution.Backend == string(BackendLocal) {
			intent, planErr := planWorkspace(s.Config, task.TaskID, workflow.WorkflowRunID, base)
			if planErr != nil {
				return planErr
			}
			run.WorkspaceIntent = intent
			run.WorktreePath, run.WorktreeBranch, run.BaseCommit = ptr(intent.Path), ptr(intent.Branch), ptr(intent.BaseCommit)
		}
		workerActive := s.worker != nil && s.active == run
		if execution.Process != nil {
			run.Process = execution.Process
		} else if !workerActive {
			run.Process = nil
		}
		if workerActive && task.Status != "discarded" {
			run.ExecutionStatus, run.FinishTaskStatus = "running", "pending"
		} else {
			run.ExecutionStatus = nativeRuntimeExecutionStatus(task, *workflow, *execution, attempt)
			run.FinishTaskStatus = nativeRuntimeFinishStatus(task, *workflow)
		}
		run.Error = nil
		if run.FinishTaskStatus == "blocked" && task.Status == "discarded" {
			run.Error = ptr("native task was discarded; cleanup is handled by its fixed external identities")
		}
		if run.FinishTaskStatus == "blocked" && run.ExecutionStatus == "unknown" && run.Error == nil {
			run.Error = ptr("native worker outcome is uncertain; original attempt and resource identity are retained")
		}
		if attempt != nil && attempt.StepID == "publish_pr" && len(attempt.Result) > 0 {
			var result StepGateResult
			if json.Unmarshal(attempt.Result, &result) == nil && result.PRURL != "" {
				run.PR.URL = ptr(result.PRURL)
			}
		}
		if run.PR.URL == nil {
			if publication, ok := latestNativePublication(snapshot.Attempts, execution.ExecutionID); ok {
				run.PR.URL = ptr(publication.PRURL)
			}
		}
		if task.Status == "closed" {
			page, queryErr := (QueryService{Store: s.domainStore, Project: s.Config.identity}).ShowNativeTask(ctx, task.TaskID)
			if queryErr != nil {
				return queryErr
			}
			if len(page.Tasks) != 1 || page.Tasks[0].Delivery == nil {
				return nativeErr("inconsistent_delivery", "closed native task has no delivery evidence")
			}
			delivery := page.Tasks[0].Delivery
			run.PR.URL = ptr(delivery.PRURL)
			run.PR.State, run.PR.MergedAt, run.PR.MergeCommit = "merged", ptr(delivery.MergedAt), ptr(delivery.MergeCommit)
		}
	}
	s.State.SchemaVersion = CurrentRuntimeSchema
	s.refreshSlotOccupancy()
	return nil
}

func latestNativePublication(attempts []StepAttemptRecord, executionID string) (StepGateResult, bool) {
	var selected StepAttemptRecord
	var result StepGateResult
	found := false
	for _, attempt := range attempts {
		if attempt.ExecutionID != executionID || attempt.StepID != "publish_pr" || attempt.Status != "succeeded" || len(attempt.Result) == 0 {
			continue
		}
		var candidate StepGateResult
		if json.Unmarshal(attempt.Result, &candidate) != nil || candidate.StepID != "publish_pr" || candidate.PRURL == "" || candidate.ResultID != attempt.ResultID {
			continue
		}
		if !found || attempt.AttemptNumber > selected.AttemptNumber {
			selected, result, found = attempt, candidate, true
		}
	}
	return result, found
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

func nativeRuntimeExecutionStatus(task TaskRecord, workflow WorkflowRunRecord, execution ExecutionRecord, attempt *StepAttemptRecord) string {
	if task.Status == "closed" {
		return "succeeded"
	}
	switch execution.Status {
	case "failed":
		return "failed"
	case "blocked", "waiting_decision":
		return "unknown"
	case "succeeded":
		return "succeeded"
	case "stopped":
		return "failed"
	}
	if workflow.Phase == "waiting_pr_merge" {
		return "succeeded"
	}
	if attempt != nil {
		switch attempt.Status {
		case "planned", "rejected", "failed":
			if attempt.Status == "failed" || attempt.Status == "rejected" {
				return "failed"
			}
			return "pending"
		case "running", "dispatched", "dispatching", "outcome_unknown", "waiting_decision":
			return "unknown"
		}
	}
	if execution.Status == "pending" || execution.Status == "active" {
		return "pending"
	}
	return "unknown"
}

func (s *Service) reconcileNativeCleanup(ctx context.Context) error {
	if !s.native {
		return nil
	}
	var cleanupErr error
	for i := range s.State.Tasks {
		task := s.State.Tasks[i]
		if task.Status != "discarded" || len(task.CleanupPending) == 0 {
			continue
		}
		for _, item := range append([]string(nil), task.CleanupPending...) {
			if strings.HasPrefix(item, "pull_request_close:") {
				if err := s.reconcileNativePRCleanup(ctx, task, item); err != nil {
					if run := s.find(task.TaskID); run != nil {
						run.Error = ptr("discarded PR cleanup requires reconciliation: " + clip(err.Error(), 1000))
					}
					cleanupErr = errors.Join(cleanupErr, err)
					continue
				}
				if err := s.refreshNativeSnapshot(ctx, true); err != nil {
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
					if err := s.refreshNativeSnapshot(ctx, true); err != nil {
						return err
					}
					return nil
				} else {
					if run := s.find(task.TaskID); run != nil {
						run.Error = ptr("discarded workspace cleanup requires reconciliation: " + clip(err.Error(), 1000))
					}
					cleanupErr = errors.Join(cleanupErr, err)
					continue
				}
			}
			if !strings.HasPrefix(item, "execution_stop_manual:") {
				continue
			}
			executionID := strings.TrimPrefix(item, "execution_stop_manual:")
			if run := s.active; run != nil && run.DomainTaskID == task.TaskID && run.ExecutionID == executionID && s.worker != nil {
				if s.Config.hostName() != "local" {
					continue // Remote cancellation is not available; keep the exact manual item.
				}
				if _, err := s.requireBackendCapabilities(ctx, backendFor(s.Config, BeadsContext{}), "stop", "observe"); err != nil {
					run.Error = ptr("discarded task worker remains active; verified local stop capability is unavailable: " + clip(err.Error(), 1000))
					continue
				}
				op, stopErr := s.stopActiveExecution(ctx)
				if stopErr != nil {
					run.Error = ptr("discarded task worker stop requires reconciliation: " + clip(stopErr.Error(), 1000))
					continue
				}
				if op.Status != OperationConfirmed {
					backend := backendFor(s.Config, BeadsContext{})
					observation, observeErr := backend.Observe(ctx, *op.Ref)
					if observeErr != nil || observation.Liveness != "exited" {
						run.Error = ptr("discarded task worker stop is awaiting confirmed process absence")
						continue
					}
					confirmed, confirmErr := s.confirmExecutionStop(op)
					if !confirmed {
						run.Error = ptr("discarded task worker stop is not confirmed: " + clip(fmt.Sprint(confirmErr), 1000))
						continue
					}
				}
				if err := s.recordNativeStopCleanup(ctx, task.TaskID, item, op); err != nil {
					run.Error = ptr("discarded task stop receipt could not be committed: " + clip(err.Error(), 1000))
					continue
				}
				if err := s.refreshNativeSnapshot(ctx, true); err != nil {
					return err
				}
				if s.active != nil && s.active.RunID == op.RunID && s.active.AttemptID == op.AttemptID {
					s.worker, s.active = nil, nil
					s.State.CurrentTask = nil
					s.State.SlotOccupied = false
					if err := s.save(); err != nil {
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
	workflow, execution := s.minimalRecords(run)
	if workflow == nil || execution == nil {
		return nativeErr("inconsistent_delivery", "discarded PR cleanup lacks its fixed workflow and execution")
	}
	publication, ok := latestNativePublication(s.State.Attempts, execution.ExecutionID)
	if !ok || publication.PRURL != prURL || publication.PublishedHead != execution.CandidateRevision {
		return nativeErr("authority_mismatch", "discarded PR cleanup URL differs from the fixed publish_pr result")
	}
	info, err := s.prs.Query(ctx, prURL)
	if err != nil || info.State == "unknown" {
		return errors.Join(err, nativeErr("merge_state_unknown", "discarded PR state could not be verified"))
	}
	if info.Merged || info.State == "merged" {
		_, conflictErr := (CommandService{Store: s.domainStore, Project: s.Config.identity}).CompleteNativeTask(ctx, s.Config, task.TaskID, task.Version, workflow.WorkflowRunID, s.prs)
		if conflictErr == nil {
			return nativeErr("inconsistent_delivery", "discarded task unexpectedly received a normal delivery")
		}
		return conflictErr
	}
	if info.URL != prURL || info.Repository != s.Config.PR.Repository || info.BaseBranch != s.Config.Branch || info.HeadRevision != execution.CandidateRevision {
		return nativeErr("authority_mismatch", "discarded PR provider returned a different URL, repository, branch or head")
	}
	if info.State == "open" {
		info, err = s.prs.Close(ctx, prURL)
		if err != nil {
			return err
		}
	}
	if info.State != "closed_unmerged" || info.Merged {
		return nativeErr("merge_state_unknown", "discarded PR closure did not produce a verified closed_unmerged receipt")
	}
	evidence := NativePRCleanupReceipt{WorkflowRunID: workflow.WorkflowRunID, ExecutionID: execution.ExecutionID, AttemptID: latestNativePublicationAttemptID(s.State.Attempts, execution.ExecutionID), PRURL: info.URL, Repository: info.Repository, BaseBranch: info.BaseBranch, HeadRevision: info.HeadRevision, State: info.State, Merged: info.Merged, ObservedAt: now()}
	if evidence.AttemptID == "" {
		return nativeErr("inconsistent_delivery", "discarded PR cleanup cannot identify its publication attempt")
	}
	data, _ := json.Marshal(evidence)
	requestID := digestValue(struct{ TaskID, Item string }{task.TaskID, cleanupItem})[:32]
	_, err = (CommandService{Store: s.domainStore, Project: s.Config.identity}).RecordNativeCleanupReceipt(ctx, task.TaskID, requestID, task.Version, cleanupItem, s.Config.Claimant, data)
	return err
}

func latestNativePublicationAttemptID(attempts []StepAttemptRecord, executionID string) string {
	var selected StepAttemptRecord
	found := false
	for _, attempt := range attempts {
		if attempt.ExecutionID == executionID && attempt.StepID == "publish_pr" && attempt.Status == "succeeded" && (!found || attempt.AttemptNumber > selected.AttemptNumber) {
			selected, found = attempt, true
		}
	}
	if found {
		return selected.AttemptID
	}
	return ""
}

func (s *Service) reconcileNativeWorkspaceCleanup(ctx context.Context, task TaskRecord, cleanupItem string) error {
	if s.Config.hostName() != "local" {
		return UnsupportedAdapterOperation("WorkspaceCleanup", "remote native worktree cleanup is unavailable")
	}
	path := strings.TrimPrefix(cleanupItem, "workspace_cleanup:")
	root, err := filepath.EvalSymlinks(s.Config.WorktreeRoot)
	if err != nil {
		return err
	}
	resolved, err := resolveProspectivePath(path)
	if err != nil || canonical(filepath.Dir(resolved)) != canonical(root) {
		return nativeErr("authority_mismatch", "discarded workspace cleanup path no longer matches worktree_root")
	}
	var execution *ExecutionRecord
	var workflow *WorkflowRunRecord
	for i := range s.State.Executions {
		candidate := &s.State.Executions[i]
		if canonical(candidate.Target.WorktreeID) != canonical(resolved) && canonical(candidate.Workspace.Worktree) != canonical(resolved) {
			continue
		}
		for j := range s.State.WorkflowRuns {
			if s.State.WorkflowRuns[j].WorkflowRunID == candidate.WorkflowRunID && s.State.WorkflowRuns[j].TaskID == task.TaskID {
				execution, workflow = candidate, &s.State.WorkflowRuns[j]
				break
			}
		}
	}
	if execution == nil || workflow == nil || execution.Target.Branch == "" || !validCommit(execution.Target.BaseRevision) {
		return nativeErr("contract_mismatch", "discarded workspace path lacks a fixed branch and base identity")
	}
	branch := execution.Target.Branch
	if _, err = (Runner{Dir: s.Config.Repository, Timeout: duration(s.Config.CommandTimeout)}).Output(ctx, s.Config.Git, "check-ref-format", "refs/heads/"+branch); err != nil {
		return nativeErr("authority_mismatch", "discarded workspace branch identity is invalid")
	}
	git := gitRunner(s.Config, s.Config.Repository)
	worktrees, err := git.Output(ctx, s.Config.Git, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return err
	}
	registered := false
	for _, record := range strings.Split(string(worktrees), "\x00") {
		if strings.HasPrefix(record, "worktree ") && canonical(strings.TrimPrefix(record, "worktree ")) == canonical(resolved) {
			registered = true
			break
		}
	}
	if registered {
		intent := WorkspaceIntent{Path: resolved, Branch: branch, BaseCommit: execution.Target.BaseRevision}
		if err = verifyWorkspace(ctx, s.Config, &intent, false); err != nil {
			return fmt.Errorf("discarded workspace identity could not be verified: %w", err)
		}
		if _, err = git.Output(ctx, s.Config.Git, "worktree", "remove", "--force", "--", resolved); err != nil {
			return err
		}
	} else if _, statErr := os.Lstat(resolved); statErr == nil {
		return nativeErr("authority_mismatch", "discarded workspace path exists but is not the fixed Git worktree")
	} else if !os.IsNotExist(statErr) {
		return statErr
	}
	refs, err := git.Output(ctx, s.Config.Git, "for-each-ref", "--format=%(refname)", "refs/heads/"+branch)
	if err != nil {
		return err
	}
	branchExists := strings.TrimSpace(string(refs)) != ""
	if branchExists {
		if _, err = git.Output(ctx, s.Config.Git, "branch", "-D", "--", branch); err != nil {
			return err
		}
	}
	worktrees, err = git.Output(ctx, s.Config.Git, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return err
	}
	for _, record := range strings.Split(string(worktrees), "\x00") {
		if strings.HasPrefix(record, "worktree ") && canonical(strings.TrimPrefix(record, "worktree ")) == canonical(resolved) {
			return nativeErr("outcome_unknown", "discarded worktree remains registered after cleanup")
		}
	}
	refs, err = git.Output(ctx, s.Config.Git, "for-each-ref", "--format=%(refname)", "refs/heads/"+branch)
	if err != nil || strings.TrimSpace(string(refs)) != "" {
		return nativeErr("outcome_unknown", "discarded workspace branch remains after cleanup")
	}
	receipt := NativeWorkspaceCleanupReceipt{WorkflowRunID: workflow.WorkflowRunID, ExecutionID: execution.ExecutionID, Repository: execution.Target.ID, Worktree: resolved, Branch: branch, BaseCommit: execution.Target.BaseRevision, WorktreeGone: true, BranchGone: true, ObservedAt: now()}
	data, _ := json.Marshal(receipt)
	requestID := digestValue(struct{ TaskID, Item string }{task.TaskID, cleanupItem})[:32]
	_, err = (CommandService{Store: s.domainStore, Project: s.Config.identity}).RecordNativeCleanupReceipt(ctx, task.TaskID, requestID, task.Version, cleanupItem, s.Config.Claimant, data)
	return err
}

func (s *Service) recordNativeStopCleanup(ctx context.Context, taskID, cleanupItem string, op Operation) error {
	if op.Ref == nil {
		return &BackendError{Code: BackendIdentityMismatch, Diagnostic: "native stop operation has no fixed execution reference"}
	}
	confirmed, err := s.confirmExecutionStop(op)
	if err != nil || !confirmed {
		return errors.Join(err, &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "native stop operation is not confirmed"})
	}
	records := executionRecords{root: s.Config.RunDir}
	persisted, err := records.loadOperation(op.RunID, op.AttemptID, op.ID)
	if err != nil || persisted.Status != OperationConfirmed {
		return errors.Join(err, nativeErr("outcome_unknown", "confirmed stop was not durably recorded"))
	}
	backend := backendFor(s.Config, BeadsContext{})
	observation, err := backend.Observe(ctx, *op.Ref)
	if err != nil || observation.Liveness != "exited" {
		return errors.Join(err, nativeErr("outcome_unknown", "fixed local execution is not confirmed absent"))
	}
	evidence := NativeStopCleanupReceipt{WorkflowRunID: op.RunID, ExecutionID: nativeExecutionIDForAttempt(s.State.Attempts, op.AttemptID), AttemptID: op.AttemptID, Operation: persisted, Observation: observation}
	if evidence.ExecutionID == "" {
		return nativeErr("contract_mismatch", "stop attempt is not bound to a native execution")
	}
	task := nativeTaskRecord(s.State.Tasks, taskID)
	if task == nil {
		return nativeErr("task_not_found", "discarded native task disappeared during stop reconciliation")
	}
	data, _ := json.Marshal(evidence)
	requestID := digestValue(struct{ TaskID, Item, OperationID string }{taskID, cleanupItem, op.ID})[:32]
	_, err = (CommandService{Store: s.domainStore, Project: s.Config.identity}).RecordNativeCleanupReceipt(ctx, taskID, requestID, task.Version, cleanupItem, s.Config.Claimant, data)
	return err
}

func (s *Service) reconcileExitedNativeExecution(ctx context.Context, taskID, cleanupItem, executionID string) error {
	if s.Config.hostName() != "local" {
		return UnsupportedAdapterOperation("StopExecution", "remote native worker cancellation is unavailable")
	}
	var execution *ExecutionRecord
	for i := range s.State.Executions {
		if s.State.Executions[i].ExecutionID == executionID {
			execution = &s.State.Executions[i]
			break
		}
	}
	if execution == nil || execution.Process == nil {
		return nativeErr("outcome_unknown", "discarded task has no fixed local process identity")
	}
	workflowRunID := execution.WorkflowRunID
	var attempt *StepAttemptRecord
	for i := range s.State.Attempts {
		candidate := &s.State.Attempts[i]
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
	backend := backendFor(s.Config, BeadsContext{})
	observation, err := backend.Observe(ctx, ref)
	if err != nil || observation.Liveness != "exited" {
		return errors.Join(err, nativeErr("outcome_unknown", "fixed local process is live or its state is unknown; it cannot be stopped without the original handle"))
	}
	opID := "stop-" + attempt.AttemptID
	payload := struct {
		Policy string
		Grace  string
	}{"fixed_local_execution_stop", s.Config.ShutdownGrace}
	op := Operation{Version: ExecutionRecordVersion, ID: opID, RunID: workflowRunID, AttemptID: attempt.AttemptID, Backend: BackendLocal, Protocol: ProtocolLocalProcess, Target: ref.Target, Action: "stop", PayloadDigest: digestValue(payload), Status: OperationConfirmed, StartedAt: now(), UpdatedAt: now(), Diagnostic: observation.Diagnostic, Ref: &ref}
	records := executionRecords{root: s.Config.RunDir}
	if old, loadErr := records.loadOperation(workflowRunID, attempt.AttemptID, opID); loadErr == nil {
		if old.PayloadDigest != op.PayloadDigest || old.Ref == nil || digestValue(*old.Ref) != digestValue(ref) {
			return nativeErr("authority_mismatch", "saved local stop operation differs from the fixed execution identity")
		}
		if old.Status != OperationConfirmed {
			old.Status, old.UpdatedAt, old.Diagnostic = OperationConfirmed, now(), observation.Diagnostic
			op = old
		}
	} else if !os.IsNotExist(loadErr) {
		return loadErr
	}
	if err := records.saveOperation(op); err != nil {
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

func nativeTaskModel(task TaskRecord) Task {
	priority := task.Priority
	return Task{ID: task.TaskID, Title: task.Title, Status: task.Status, Type: task.Type, Assignee: task.Claimant, Priority: &priority}
}

func (s *Service) dispatchNative(ctx context.Context) error {
	if s.worker != nil {
		return nil
	}
	if err := s.refreshNativeSnapshot(ctx, true); err != nil {
		return err
	}
	if !s.explicitDispatch && (s.State.Control.DispatchMode != "auto" || s.State.Control.DispatchPaused) {
		return nil
	}
	if s.worker != nil || s.State.SlotOccupied && s.activeControlRequestID() == "" {
		return nil
	}
	if reason := s.slotReason(s.activeControlRequestID()); reason != "" {
		return errors.New(reason)
	}
	ready, err := (QueryService{Store: s.domainStore, Project: s.Config.identity}).ReadyNativeTasks(ctx)
	if err != nil {
		return err
	}
	if len(ready) > 0 {
		required := []string{"start", "observe", "workspace_inspection", "output_reading"}
		if s.Config.hostName() == "orca_supervised" {
			required = append(required, "message_inbox", "reply_continue", "resource_settlement")
		} else {
			required = append(required, "stop")
		}
		if _, err = s.requireBackendCapabilities(ctx, backendFor(s.Config, BeadsContext{}), required...); err != nil {
			return err
		}
	}
	for _, task := range ready {
		if len(s.Config.ExecutableTypes) > 0 && !contains(s.Config.ExecutableTypes, task.Type) {
			continue
		}
		if s.targetTask != "" && s.targetTask != task.TaskID {
			continue
		}
		authority, authErr := s.domainStore.nativeAuthority(ctx, s.Config.identity)
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
		receipt, claimErr := (CommandService{Store: s.domainStore, Project: s.Config.identity}).ClaimNativeTask(ctx, s.Config, TaskClaimOptions{TaskID: task.TaskID, RequestID: requestID, ExpectedTaskVersion: task.Version, ExpectedAuthorityVersion: authority.AuthorityVersion, RequestedBy: s.Config.Claimant, Explicit: s.explicitDispatch, WorkflowRunID: workflowRunID})
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
		if err = s.refreshNativeSnapshot(ctx, true); err != nil {
			return err
		}
		run := s.find(task.TaskID)
		workflow, execution := s.minimalRecords(run)
		attempt := s.minimalAttempt(run, run.AttemptID)
		if run == nil || workflow == nil || execution == nil || attempt == nil {
			return nativeErr("inconsistent_delivery", "native claim reservation could not be reconstructed from center records")
		}
		intent, planErr := planWorkspace(s.Config, task.TaskID, workflow.WorkflowRunID, receipt.BaseCommit)
		if planErr != nil {
			return planErr
		}
		if execution.Target.WorktreeID != "" || execution.Target.Branch != s.Config.Branch {
			return nativeErr("authority_mismatch", "claimed execution already has a conflicting workspace identity")
		}
		if s.Config.hostName() != "orca_supervised" {
			execution.Target.WorktreeID = intent.Path
			execution.Target.Branch = intent.Branch
		}
		execution.Status = "active"
		if workflow.Phase == "queued" {
			workflow.Phase = "executing"
		}
		execution.UpdatedAt = now()
		workflow.UpdatedAt = now()
		attempt.ExternalBinding = execution.Target
		attempt.UpdatedAt = now()
		run.WorkspaceIntent = intent
		if s.Config.hostName() == "orca_supervised" {
			run.WorktreePath, run.WorktreeBranch = nil, nil
			run.BaseCommit = ptr(intent.BaseCommit)
		} else {
			run.WorktreePath, run.WorktreeBranch, run.BaseCommit = ptr(intent.Path), ptr(intent.Branch), ptr(intent.BaseCommit)
		}
		run.ExecutionStatus = "pending"
		run.FinishTaskStatus = "pending"
		s.State.SlotOccupied = true
		s.State.CurrentTask = ptr(task.TaskID)
		if err = s.save(); err != nil {
			return err
		}
		s.lastDispatched = task.TaskID
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return s.prepareMinimalExecution(ctx, nativeTaskModel(task), run, BeadsContext{})
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
	task := nativeTaskRecord(s.State.Tasks, run.DomainTaskID)
	workflow, _ := s.minimalRecords(run)
	if task == nil || workflow == nil {
		return nativeErr("inconsistent_delivery", "native delivery task or workflow record is missing")
	}
	receipt, err := (CommandService{Store: s.domainStore, Project: s.Config.identity}).CompleteNativeTask(ctx, s.Config, task.TaskID, task.Version, workflow.WorkflowRunID, s.prs)
	if err != nil {
		return err
	}
	if receipt.Status != "closed" || receipt.Delivery.TaskID != run.DomainTaskID {
		return nativeErr("inconsistent_delivery", "native delivery transaction returned an invalid task receipt")
	}
	if err = s.refreshNativeSnapshot(ctx, false); err != nil {
		return err
	}
	run = s.find(receipt.TaskID)
	if run == nil {
		return nativeErr("inconsistent_delivery", "closed native task disappeared from the execution projection")
	}
	run.ExecutionStatus = "succeeded"
	run.FinishTaskStatus = "completed"
	run.PR.State, run.PR.MergedAt, run.PR.MergeCommit = "merged", ptr(receipt.Delivery.MergedAt), ptr(receipt.Delivery.MergeCommit)
	run.Error = nil
	s.note(run, "steer", "delivery", "completed", "native task delivered after verified PR merge", receipt.Delivery.PRURL, map[string]any{"merge_commit": receipt.Delivery.MergeCommit, "graph_revision": receipt.GraphRevision})
	s.closeActivity(run)
	return s.save()
}

func (s *Service) pollNativePRs(ctx context.Context, onlyRunID string) error {
	var operationErr error
	for _, run := range s.State.Runs {
		if run == nil || onlyRunID != "" && run.RunID != onlyRunID {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		task := nativeTaskRecord(s.State.Tasks, run.DomainTaskID)
		if task == nil {
			continue
		}
		workflow, _ := s.minimalRecords(run)
		if workflow == nil {
			continue
		}
		if task.Status == "discarded" {
			if run.PR.URL == nil {
				continue
			}
			_, err := (CommandService{Store: s.domainStore, Project: s.Config.identity}).CompleteNativeTask(ctx, s.Config, task.TaskID, task.Version, workflow.WorkflowRunID, s.prs)
			if err != nil && !isNativeErrorCode(err, "task_discarded") {
				operationErr = errors.Join(operationErr, err)
			}
			continue
		}
		if task.Status != "in_progress" || run.FinishTaskStatus != "waiting_pr_merge" || run.PR.URL == nil {
			continue
		}
		info, err := s.prs.Query(ctx, *run.PR.URL)
		queried := now()
		run.PR.QueriedAt = &queried
		if err != nil {
			run.PR.State = "unknown"
			run.PR.QueryError = ptr(clip(err.Error(), 1000))
			if saveErr := s.save(); saveErr != nil {
				return saveErr
			}
			operationErr = errors.Join(operationErr, nativeErr("merge_state_unknown", "PR merge state could not be verified: "+clip(err.Error(), 1000)))
			continue
		}
		run.PR.QueryError = nil
		run.PR.State, run.PR.MergedAt, run.PR.MergeCommit = info.State, info.MergedAt, info.MergeCommit
		if info.BaseBranch != s.Config.Branch {
			run.FinishTaskStatus = "blocked"
			run.Error = ptr("PR target branch differs from the fixed native project branch")
			if err = s.save(); err != nil {
				return err
			}
			operationErr = errors.Join(operationErr, nativeErr("immutable_task", *run.Error))
			continue
		}
		if err = s.save(); err != nil {
			return err
		}
		switch info.State {
		case "merged":
			if err = s.nativeCompleteMergedTask(ctx, run); err != nil {
				run.Error = ptr("PR merged; center delivery transaction will retry: " + clip(err.Error(), 1000))
				operationErr = errors.Join(operationErr, err)
				if saveErr := s.save(); saveErr != nil {
					return saveErr
				}
			}
		case "closed_unmerged":
			run.FinishTaskStatus = "blocked"
			run.Error = ptr("PR closed without merge; native task remains in progress for manual recovery")
			if err = s.save(); err != nil {
				return err
			}
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
