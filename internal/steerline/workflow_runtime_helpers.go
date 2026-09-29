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

func (s *Service) workflowRecords(run *Run) (*WorkflowRunRecord, *ExecutionRecord) {
	if run == nil {
		return nil, nil
	}
	var workflow *WorkflowRunRecord
	for i := range s.State.WorkflowRuns {
		if s.State.WorkflowRuns[i].WorkflowRunID == run.RunID {
			workflow = &s.State.WorkflowRuns[i]
			break
		}
	}
	var execution *ExecutionRecord
	for i := range s.State.Executions {
		if s.State.Executions[i].ExecutionID == run.ExecutionID && s.State.Executions[i].WorkflowRunID == run.RunID {
			execution = &s.State.Executions[i]
			break
		}
	}
	return workflow, execution
}

func (s *Service) workflowAttempt(run *Run, attemptID string) *StepAttemptRecord {
	if run == nil {
		return nil
	}
	for i := range s.State.Attempts {
		if s.State.Attempts[i].AttemptID == attemptID && s.State.Attempts[i].ExecutionID == run.ExecutionID {
			return &s.State.Attempts[i]
		}
	}
	return nil
}

func (s *Service) inspectWorkflowWorkspace(ctx context.Context, run *Run) (WorkspaceEvidence, error) {
	if run == nil || run.WorktreePath == nil || run.WorktreeBranch == nil || run.BaseCommit == nil || run.WorkspaceIntent == nil {
		return WorkspaceEvidence{}, errors.New("minimal execution has no recorded workspace")
	}
	c, configErr := s.executionConfigForRun(run)
	if configErr != nil {
		return WorkspaceEvidence{}, configErr
	}
	if run.Backend == string(BackendOrca) && run.Protocol == string(ProtocolOrcaSupervised) {
		if err := restoreSupervisedTarget(run, s.State.Attempts, targetFor(c), executionRecords{root: c.RunDir}); err != nil {
			return WorkspaceEvidence{}, err
		}
		records := executionRecords{root: c.RunDir}
		bindingAttemptID := run.AttemptID
		ref, err := records.loadBinding(run.RunID, bindingAttemptID)
		if os.IsNotExist(err) {
			// A newly planned step has no binding yet. Reuse the most recent
			// settled attempt's exact Dispatch only for read-only workspace facts.
			var latest StepAttemptRecord
			for _, candidate := range s.State.Attempts {
				if candidate.ExecutionID == run.ExecutionID && candidate.ExternalBinding.WorktreeID == run.Target.WorktreeID && candidate.ExternalBinding.Dispatch != "" && (latest.UpdatedAt.IsZero() || candidate.UpdatedAt.After(latest.UpdatedAt)) {
					latest = candidate
				}
			}
			if latest.AttemptID != "" {
				bindingAttemptID = latest.AttemptID
				ref, err = records.loadBinding(run.RunID, bindingAttemptID)
			}
		}
		if err != nil {
			return WorkspaceEvidence{}, err
		}
		inspection, err := (orcaSupervisedBackend{config: c}).InspectWorkspace(ctx, ref)
		if err != nil {
			return WorkspaceEvidence{}, err
		}
		if execution := s.executionRecord(run); execution != nil {
			inspection.Dirty = execution.Workspace.Dirty
		}
		return WorkspaceEvidence{Repository: inspection.Repository, Worktree: canonical(inspection.Worktree), Branch: inspection.Branch, Base: inspection.Base, Head: inspection.Head, Dirty: inspection.Dirty, VerifiedAt: inspection.CheckedAt}, nil
	}
	worktree := *run.WorktreePath
	if canonical(worktree) != canonical(run.WorkspaceIntent.Path) || *run.WorktreeBranch != run.WorkspaceIntent.Branch || *run.BaseCommit != run.WorkspaceIntent.BaseCommit {
		return WorkspaceEvidence{}, nativeErr("authority_mismatch", "minimal workspace differs from its fixed durable intent")
	}
	backend := backendFor(c)
	kind, protocol := backend.Identity()
	if kind == BackendLocal && run.Protocol == string(ProtocolLocalCodexAppServer) {
		protocol = ProtocolLocalCodexAppServer
	}
	if string(kind) != run.Backend || string(protocol) != run.Protocol || run.Target.ID != targetFor(c).ID {
		return WorkspaceEvidence{}, UnsupportedAdapterOperation("InspectWorkspace", "configured target differs from the native execution identity")
	}
	inspection, err := backend.InspectWorkspace(ctx, ExecutionRef{Version: ExecutionRecordVersion, Backend: kind, Protocol: protocol, Target: targetFor(c), RunID: run.RunID, AttemptID: run.AttemptID, WorkspacePath: worktree, WorkspaceBranch: *run.WorktreeBranch, BaseCommit: *run.BaseCommit})
	if err != nil {
		return WorkspaceEvidence{}, err
	}
	if canonical(inspection.Worktree) != canonical(worktree) || inspection.Branch != *run.WorktreeBranch || inspection.Base != *run.BaseCommit || !revisionPattern.MatchString(inspection.Head) {
		return WorkspaceEvidence{}, nativeErr("contract_mismatch", "native target workspace inspection differs from its fixed identity")
	}
	return WorkspaceEvidence{Repository: c.identity, Worktree: canonical(worktree), Branch: inspection.Branch, Base: inspection.Base, Head: inspection.Head, Dirty: inspection.Dirty, VerifiedAt: inspection.CheckedAt}, nil
}

func restoreSupervisedTarget(run *Run, attempts []StepAttemptRecord, fixedTarget ExecutionTarget, records executionRecords) error {
	if run.Target.ID != "" && run.Target.ID != fixedTarget.ID {
		return nativeErr("authority_mismatch", fmt.Sprintf("supervised execution target ID %q differs from configured target %q", run.Target.ID, fixedTarget.ID))
	}
	if run.Target.ConnectionID != "" && run.Target.ConnectionID != fixedTarget.ConnectionID || run.Target.ExternalRuntime != "" && run.Target.ExternalRuntime != fixedTarget.ExternalRuntime {
		return nativeErr("authority_mismatch", "runtime target connection differs from the configured supervised target")
	}
	var recovered *ExecutionTarget
	for i := range attempts {
		attempt := &attempts[i]
		binding := attempt.ExternalBinding
		if attempt.ExecutionID != run.ExecutionID || attempt.StepID != "prepare" {
			continue
		}
		if binding.WorktreeID == "" || binding.OrchestrationRun == "" || binding.Dispatch == "" {
			ref, err := records.loadBinding(run.RunID, attempt.AttemptID)
			if err != nil {
				continue
			}
			binding = ref.Target
		}
		if binding.ID == "" || binding.WorktreeID == "" || binding.OrchestrationRun == "" || binding.Dispatch == "" {
			continue
		}
		if binding.ID != fixedTarget.ID || binding.ConnectionID != fixedTarget.ConnectionID || binding.ExternalRuntime != fixedTarget.ExternalRuntime {
			return nativeErr("authority_mismatch", "persisted prepare binding differs from the configured supervised target")
		}
		if recovered != nil && (recovered.WorktreeID != binding.WorktreeID || recovered.OrchestrationRun != binding.OrchestrationRun || recovered.Dispatch != binding.Dispatch) {
			return nativeErr("authority_mismatch", "persisted prepare bindings disagree on the fixed supervised worktree")
		}
		copy := binding
		recovered = &copy
	}
	if recovered == nil {
		return UnsupportedAdapterOperation("InspectWorkspace", "supervised execution lacks its exact fixed prepare binding")
	}
	if run.Target.WorktreeID != "" && run.Target.WorktreeID != recovered.WorktreeID ||
		run.Target.OrchestrationRun != "" && run.Target.OrchestrationRun != recovered.OrchestrationRun {
		return nativeErr("authority_mismatch", "runtime target conflicts with its fixed prepare binding")
	}
	if run.Target.WorktreeID != "" && run.Target.OrchestrationRun != "" {
		return nil
	}
	run.Target = *recovered
	return nil
}

func applyWorkflowWorkspaceEvidence(run *Run, execution *ExecutionRecord, workspace WorkspaceEvidence) {
	applyWorkflowWorkspaceTarget(run, execution, workspace)
	execution.Workspace = workspace
	execution.UpdatedAt = now()
}

func applyWorkflowWorkspaceTarget(run *Run, execution *ExecutionRecord, workspace WorkspaceEvidence) {
	if run.Protocol == string(ProtocolOrcaSupervised) {
		execution.Target = run.Target
	} else {
		execution.Target.WorktreeID = workspace.Worktree
	}
	execution.Target.Branch = workspace.Branch
	execution.Target.BaseRevision = workspace.Base
	run.Target = execution.Target
}

func prepareWorktreeOnly(ctx context.Context, c Config, run *Run) error {
	intent := run.WorkspaceIntent
	if intent == nil {
		return errors.New("missing durable workspace intent")
	}
	if err := c.prepareWorktreeRoot(); err != nil {
		return err
	}
	if canonical(filepath.Dir(intent.Path)) != canonical(c.WorktreeRoot) {
		return errors.New("workspace intent path no longer matches worktree_root")
	}
	if _, err := os.Lstat(intent.Path); !os.IsNotExist(err) {
		return fmt.Errorf("worktree path already exists or is inaccessible: %s", intent.Path)
	}
	if err := os.MkdirAll(c.WorktreeRoot, 0700); err != nil {
		return err
	}
	git := gitRunner(c, c.Repository)
	if _, err := git.Output(ctx, c.Git, "check-ref-format", "refs/heads/"+intent.Branch); err != nil {
		return err
	}
	refs, err := git.Output(ctx, c.Git, "for-each-ref", "--format=%(refname)", "refs/heads/"+intent.Branch)
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(refs)) != "" {
		return errors.New("worktree branch already exists")
	}
	if err = os.Mkdir(intent.Path, 0700); err != nil {
		return err
	}
	if _, err = git.Output(ctx, c.Git, "worktree", "add", "--no-track", "-b", intent.Branch, "--", intent.Path, intent.BaseCommit); err != nil {
		return err
	}
	return verifyWorkspace(ctx, c, intent, true)
}

func mustJSON(value any) []byte { data, _ := json.Marshal(value); return data }

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (s *Service) executionRecord(run *Run) *ExecutionRecord {
	_, execution := s.workflowRecords(run)
	return execution
}
