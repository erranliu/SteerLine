package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

func (s *Service) workflowRecords(run *Run) (*WorkflowRunRecord, *ExecutionRecord) {
	if run == nil {
		return nil, nil
	}
	var workflow *WorkflowRunRecord
	for i := range s.domain.WorkflowRuns {
		if s.domain.WorkflowRuns[i].WorkflowRunID == run.RunID {
			workflow = &s.domain.WorkflowRuns[i]
			break
		}
	}
	var execution *ExecutionRecord
	for i := range s.domain.Executions {
		if s.domain.Executions[i].ExecutionID == run.ExecutionID && s.domain.Executions[i].WorkflowRunID == run.RunID {
			execution = &s.domain.Executions[i]
			break
		}
	}
	return workflow, execution
}

func (s *Service) workflowAttempt(run *Run, attemptID string) *StepAttemptRecord {
	if run == nil {
		return nil
	}
	for i := range s.domain.Attempts {
		if s.domain.Attempts[i].AttemptID == attemptID && s.domain.Attempts[i].ExecutionID == run.ExecutionID {
			return &s.domain.Attempts[i]
		}
	}
	return nil
}

func (s *Service) inspectWorkflowWorkspace(ctx context.Context, run *Run) (WorkspaceEvidence, error) {
	if run == nil || run.WorktreePath == nil || run.WorktreeBranch == nil || run.BaseCommit == nil || run.WorkspaceIntent == nil {
		return WorkspaceEvidence{}, errors.New("worker execution has no recorded workspace")
	}
	c, configErr := s.executionConfigForRun(run)
	if configErr != nil {
		return WorkspaceEvidence{}, configErr
	}
	if run.Backend == string(executionapi.BackendOrca) && run.Protocol == string(executionapi.ProtocolOrcaSupervised) {
		execution := s.executionRecord(run)
		if execution == nil || execution.Process == nil {
			return WorkspaceEvidence{}, errors.New("supervised workspace has no execution binding")
		}
		target := execution.Target
		fixed := targetFor(c)
		if target.ID != fixed.ID || target.ConnectionID != fixed.ConnectionID || target.ExternalRuntime != fixed.ExternalRuntime {
			return WorkspaceEvidence{}, nativeErr("authority_mismatch", "supervised target differs from its fixed execution")
		}
		ref := executionapi.ExecutionRef{Version: executionapi.ExecutionRecordVersion, Backend: executionapi.BackendOrca, Protocol: executionapi.ProtocolOrcaSupervised, Target: target, RunID: run.RunID, AttemptID: run.AttemptID, External: *execution.Process, WorkspacePath: execution.Workspace.Worktree, WorkspaceBranch: execution.Workspace.Branch, BaseCommit: execution.Workspace.Base}
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
		return WorkspaceEvidence{}, nativeErr("authority_mismatch", "worker workspace differs from its fixed durable intent")
	}
	backend := backendFor(c)
	kind, protocol := backend.Identity()
	if kind == executionapi.BackendLocal && run.Protocol == string(executionapi.ProtocolLocalCodexAppServer) {
		protocol = executionapi.ProtocolLocalCodexAppServer
	}
	if string(kind) != run.Backend || string(protocol) != run.Protocol || run.Target.ID != targetFor(c).ID {
		return WorkspaceEvidence{}, executionapi.UnsupportedAdapterOperation("InspectWorkspace", "configured target differs from the native execution identity")
	}
	inspection, err := backend.InspectWorkspace(ctx, executionapi.ExecutionRef{Version: executionapi.ExecutionRecordVersion, Backend: kind, Protocol: protocol, Target: targetFor(c), RunID: run.RunID, AttemptID: run.AttemptID, WorkspacePath: worktree, WorkspaceBranch: *run.WorktreeBranch, BaseCommit: *run.BaseCommit})
	if err != nil {
		return WorkspaceEvidence{}, err
	}
	if canonical(inspection.Worktree) != canonical(worktree) || inspection.Branch != *run.WorktreeBranch || inspection.Base != *run.BaseCommit || !revisionPattern.MatchString(inspection.Head) {
		return WorkspaceEvidence{}, nativeErr("contract_mismatch", "native target workspace inspection differs from its fixed identity")
	}
	return WorkspaceEvidence{Repository: c.identity, Worktree: canonical(worktree), Branch: inspection.Branch, Base: inspection.Base, Head: inspection.Head, Dirty: inspection.Dirty, VerifiedAt: inspection.CheckedAt}, nil
}

func applyWorkflowWorkspaceEvidence(run *Run, execution *ExecutionRecord, workspace WorkspaceEvidence) {
	applyWorkflowWorkspaceTarget(run, execution, workspace)
	execution.Workspace = workspace
	execution.UpdatedAt = now()
	refreshExecutionView(run, execution)
}

func applyWorkflowWorkspaceTarget(run *Run, execution *ExecutionRecord, workspace WorkspaceEvidence) {
	if execution.Protocol != string(executionapi.ProtocolOrcaSupervised) {
		execution.Target.WorktreeID = workspace.Worktree
	}
	execution.Target.Branch = workspace.Branch
	execution.Target.BaseRevision = workspace.Base
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
