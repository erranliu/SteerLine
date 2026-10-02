package steerline

import (
	"context"
	"encoding/json"
	"errors"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/workspace"
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
	port, fixed, err := s.lifecycleForRun(run)
	if err != nil {
		return WorkspaceEvidence{}, err
	}
	snapshot, err := s.lifecycleSnapshot(run, fixed.workflow, fixed.execution, fixed.attempt, nil)
	if err != nil {
		return WorkspaceEvidence{}, err
	}
	inspection, err := port.InspectLifecycleWorkspace(ctx, snapshot)
	if err != nil {
		return WorkspaceEvidence{}, err
	}
	if canonical(inspection.Worktree) != canonical(snapshot.Ref.WorkspacePath) || inspection.Branch != snapshot.Ref.WorkspaceBranch || inspection.Base != snapshot.Ref.BaseCommit || !revisionPattern.MatchString(inspection.Head) {
		return WorkspaceEvidence{}, nativeErr("contract_mismatch", "workspace inspection differs from its committed identity")
	}
	return WorkspaceEvidence{Repository: inspection.Repository, Worktree: inspection.Worktree, Branch: inspection.Branch, Base: inspection.Base, Head: inspection.Head, Dirty: inspection.Dirty, VerifiedAt: inspection.CheckedAt}, nil
}

func applyWorkflowWorkspaceEvidence(run *Run, execution *ExecutionRecord, workspace WorkspaceEvidence) {
	applyWorkflowWorkspaceTarget(run, execution, workspace)
	execution.Workspace = workspace
	execution.UpdatedAt = now()
	refreshExecutionView(run, execution)
}

func applyWorkflowWorkspaceTarget(run *Run, execution *ExecutionRecord, workspace WorkspaceEvidence) {
	if execution.Target.WorktreeID == "" {
		execution.Target.WorktreeID = workspace.Worktree
	}
	execution.Target.Branch = workspace.Branch
	execution.Target.BaseRevision = workspace.Base
}

func prepareWorktreeOnly(ctx context.Context, c Config, run *Run) error {
	_, err := prepareWorktree(ctx, c, run)
	return err
}

func prepareWorktree(ctx context.Context, c Config, run *Run) (workspace.Preparation, error) {
	intent := run.WorkspaceIntent
	if intent == nil {
		return workspace.Preparation{Status: executionapi.OperationRejected}, errors.New("missing durable workspace intent")
	}
	if err := c.prepareWorktreeRoot(); err != nil {
		return workspace.Preparation{Status: executionapi.OperationRejected}, err
	}
	repository, err := workspaceRepository(c)
	if err != nil {
		return workspace.Preparation{Status: executionapi.OperationRejected}, err
	}
	return workspace.Prepare(ctx, workspaceGit(c), repository, c.WorktreeRoot, intent)
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
