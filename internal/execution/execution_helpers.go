package execution

import (
	"context"
	"encoding/json"
	"errors"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/execution/workspace"
)

func (s *Service) workflowRecords(run *reconstructiontypes.RunView) (*executionmodel.WorkflowRunRecord, *executionmodel.ExecutionRecord) {
	if run == nil {
		return nil, nil
	}
	var workflow *executionmodel.WorkflowRunRecord
	for i := range s.domain.WorkflowRuns {
		if s.domain.WorkflowRuns[i].WorkflowRunID == run.RunID {
			workflow = &s.domain.WorkflowRuns[i]
			break
		}
	}
	var execution *executionmodel.ExecutionRecord
	for i := range s.domain.Executions {
		if s.domain.Executions[i].ExecutionID == run.ExecutionID && s.domain.Executions[i].WorkflowRunID == run.RunID {
			execution = &s.domain.Executions[i]
			break
		}
	}
	return workflow, execution
}

func (s *Service) workflowAttempt(run *reconstructiontypes.RunView, attemptID string) *executionmodel.StepAttemptRecord {
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

func (s *Service) inspectWorkflowWorkspace(ctx context.Context, run *reconstructiontypes.RunView) (executionmodel.WorkspaceEvidence, error) {
	if run == nil || run.WorktreePath == nil || run.WorktreeBranch == nil || run.BaseCommit == nil || run.WorkspaceIntent == nil {
		return executionmodel.WorkspaceEvidence{}, errors.New("worker execution has no recorded workspace")
	}
	port, fixed, err := s.lifecycleForInspection(run)
	if err != nil {
		return executionmodel.WorkspaceEvidence{}, err
	}
	snapshot, err := s.lifecycleSnapshot(run, fixed.workflow, fixed.execution, fixed.attempt, nil)
	if err != nil {
		return executionmodel.WorkspaceEvidence{}, err
	}
	if snapshot.Ref.WorkspacePath == "" {
		// A lost or not-yet-committed resource receipt is inspected against the
		// original persisted intent. This read does not publish the candidate.
		operation := s.operationByKey("worker-workspace:" + run.ExecutionID)
		var saved struct {
			WorkflowRunID string                           `json:"workflow_run_id"`
			ExecutionID   string                           `json:"execution_id"`
			Workspace     executionbackend.WorkspaceIntent `json:"workspace"`
		}
		if operation == nil || operation.Kind != "prepare_workspace" || operation.LifecycleIdentity == nil || operation.LifecycleIdentity.ExecutionID != run.ExecutionID || operation.LifecycleIdentity.Ref.AttemptID != run.AttemptID ||
			json.Unmarshal(operation.Request, &saved) != nil || saved.WorkflowRunID != run.RunID || saved.ExecutionID != run.ExecutionID || saved.Workspace != *run.WorkspaceIntent || operationDigest(operation.Request) != operation.PayloadDigest {
			return executionmodel.WorkspaceEvidence{}, errors.New("workspace inspection has no matching committed resource or preparation intent")
		}
		snapshot.Ref.WorkspacePath, snapshot.Ref.WorkspaceBranch, snapshot.Ref.BaseCommit = saved.Workspace.Path, saved.Workspace.Branch, saved.Workspace.BaseCommit
		snapshot.Workspace = saved.Workspace
	}
	inspection, err := port.InspectLifecycleWorkspace(ctx, snapshot)
	if err != nil {
		return executionmodel.WorkspaceEvidence{}, err
	}
	if inspection.Worktree != snapshot.Ref.WorkspacePath || inspection.Branch != snapshot.Ref.WorkspaceBranch || inspection.Base != snapshot.Ref.BaseCommit || !revisionPattern.MatchString(inspection.Head) {
		return executionmodel.WorkspaceEvidence{}, nativeErr("contract_mismatch", "workspace inspection differs from its committed identity")
	}
	return executionmodel.WorkspaceEvidence{InspectionScope: inspection.InspectionScope, Repository: inspection.Repository, Worktree: inspection.Worktree, Branch: inspection.Branch, Base: inspection.Base, Head: inspection.Head, Dirty: inspection.Dirty, VerifiedAt: inspection.CheckedAt}, nil
}

func applyWorkflowWorkspaceEvidence(run *reconstructiontypes.RunView, execution *executionmodel.ExecutionRecord, workspace executionmodel.WorkspaceEvidence) {
	applyWorkflowWorkspaceTarget(run, execution, workspace)
	execution.Workspace = workspace
	execution.UpdatedAt = now()
	refreshExecutionView(run, execution)
}

func applyWorkflowWorkspaceTarget(run *reconstructiontypes.RunView, execution *executionmodel.ExecutionRecord, workspace executionmodel.WorkspaceEvidence) {
	if execution.Target.WorktreeID == "" {
		execution.Target.WorktreeID = workspace.Worktree
	}
	execution.Target.Branch = workspace.Branch
	execution.Target.BaseRevision = workspace.Base
}

func prepareWorktreeOnly(ctx context.Context, c Config, directory *ExecutionDirectory, processHost executionbackend.ManagedProcessHost, run *reconstructiontypes.RunView) error {
	_, err := prepareWorktree(ctx, c, directory, processHost, run)
	return err
}

func prepareWorktree(ctx context.Context, c Config, directory *ExecutionDirectory, processHost executionbackend.ManagedProcessHost, run *reconstructiontypes.RunView) (workspace.Preparation, error) {
	intent := run.WorkspaceIntent
	if intent == nil {
		return workspace.Preparation{Status: executionbackend.OperationRejected}, errors.New("missing durable workspace intent")
	}
	if err := c.PrepareWorktreeRoot(); err != nil {
		return workspace.Preparation{Status: executionbackend.OperationRejected}, err
	}
	repository, err := workspaceRepository(c, directory)
	if err != nil {
		return workspace.Preparation{Status: executionbackend.OperationRejected}, err
	}
	return workspace.Prepare(ctx, workspaceGit(c, processHost), repository, c.WorktreeRoot, intent)
}

func mustJSON(value any) []byte { data, _ := json.Marshal(value); return data }

func (s *Service) executionRecord(run *reconstructiontypes.RunView) *executionmodel.ExecutionRecord {
	_, execution := s.workflowRecords(run)
	return execution
}
