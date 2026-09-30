package steerline

import (
	"context"
	"errors"
	"fmt"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
)

type workerOrcaWorktree = orcaworker.Worktree

func workerOrcaWorkspaceError(err error) error {
	if errors.Is(err, orcaworker.ErrWorktreeIdentity) {
		return &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: err.Error()}
	}
	return err
}

func workerOrcaBackendError(err error) error {
	var platform *orcaworker.PlatformError
	if errors.As(err, &platform) {
		return &executionapi.BackendError{Code: executionapi.BackendErrorCode(platform.Code), Diagnostic: platform.Diagnostic, Cause: platform.Cause}
	}
	return err
}

func orcaWorkerCaller(c Config) orcaworker.Call         { return nativeOrcaClient(c).Call }
func orcaWorkerMetaCaller(c Config) orcaworker.MetaCall { return nativeOrcaClient(c).MetaCall }

func workerOrcaWorktreeName(taskID, runID string) (string, error) {
	if !canonicalDecimalID(taskID) || !stableExecutionID(runID) {
		return "", errors.New("Orca worktree requires a canonical task number and stable workflow identity")
	}
	return "sl-" + taskID + "-" + runID, nil
}

func findWorkerOrcaWorktree(ctx context.Context, c Config, repoID, name, base string) (workerOrcaWorktree, bool, error) {
	worktree, found, err := orcaworker.FindWorktree(ctx, orcaWorkerCaller(c), repoID, name, base)
	return worktree, found, workerOrcaWorkspaceError(err)
}

func applyWorkerOrcaWorkspace(c Config, run *Run, execution *ExecutionRecord, worktree workerOrcaWorktree, base string) error {
	if run == nil || execution == nil || run.ExecutionID != execution.ExecutionID || !commitHash.MatchString(base) {
		return errors.New("remote worker worktree lacks its fixed execution or base")
	}
	fact, err := orcaworker.WorkspaceBinding(worktree, c.identity, base)
	if err != nil {
		return err
	}
	execution.WorkspaceIntent = &executionapi.WorkspaceIntent{Path: fact.Worktree, Branch: fact.Branch, BaseCommit: fact.Base}
	execution.Target.WorktreeID, execution.Target.Branch = worktree.ID, fact.Branch
	execution.Workspace = WorkspaceEvidence{Repository: fact.Repository, Worktree: fact.Worktree, Branch: fact.Branch, Base: fact.Base, Head: fact.Head, VerifiedAt: fact.CheckedAt}
	refreshExecutionView(run, execution)
	return nil
}

func (s *Service) prepareRemoteWorkerWorkspace(ctx context.Context, run *Run, execution *ExecutionRecord, operationID string) error {
	fixed, configErr := s.workerFlowContext(run)
	if configErr != nil {
		return configErr
	}
	fixedConfig := fixed.config
	if fixedConfig.hostName() != "orca_supervised" || run == nil || execution == nil || run.WorkspaceIntent == nil {
		return errors.New("remote worker workspace has no fixed preparation intent")
	}
	base := run.WorkspaceIntent.BaseCommit
	name, err := workerOrcaWorktreeName(run.TaskID, run.RunID)
	if err != nil {
		return err
	}
	pair, err := probeSupervisedOrcaTarget(ctx, fixedConfig)
	if err != nil {
		return err
	}
	if execution.OrcaNative == nil {
		execution.OrcaNative = &OrcaNativeExecution{RemoteRuntimeID: pair.RemoteRuntime}
	}
	if execution.OrcaNative.RemoteRuntimeID != pair.RemoteRuntime {
		return &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "original remote workspace runtime changed"}
	}
	if err := s.commitWorker(run); err != nil {
		return err
	}
	worktree, err := remoteWorkerClient(fixedConfig).CreateWorktree(ctx, pair, name, base)
	if err != nil {
		return err
	}
	if err := applyWorkerOrcaWorkspace(fixedConfig, run, execution, worktree, base); err != nil {
		return err
	}
	if operationID != "" {
		if err := s.changeWorkerOperation(run, operationID, "confirmed", "", mustJSON(worktree), ""); err != nil {
			return fmt.Errorf("remote worktree created but receipt persistence failed: %w", err)
		}
	}
	return nil
}

func (s *Service) recoverRemoteWorkerWorkspace(ctx context.Context, run *Run, execution *ExecutionRecord, operation *OperationRecord) error {
	fixed, configErr := s.workerFlowContext(run)
	if configErr != nil {
		return configErr
	}
	fixedConfig := fixed.config
	if fixedConfig.hostName() != "orca_supervised" || run == nil || execution == nil || operation == nil || operation.Kind != "prepare_workspace" || run.WorkspaceIntent == nil {
		return errors.New("remote workspace recovery has no original preparation intent")
	}
	name, err := workerOrcaWorktreeName(run.TaskID, run.RunID)
	if err != nil {
		return err
	}
	pair, err := probeSupervisedOrcaTarget(ctx, fixedConfig)
	if err != nil {
		return err
	}
	if execution.OrcaNative == nil || execution.OrcaNative.RemoteRuntimeID != pair.RemoteRuntime {
		return &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "original remote workspace runtime is unverified"}
	}
	worktree, found, err := remoteWorkerClient(fixedConfig).FindWorktree(ctx, pair, name, run.WorkspaceIntent.BaseCommit)
	if err != nil {
		return err
	}
	if !found {
		return errors.New("saved Orca worktree creation has no matching remote worktree; original request will not be repeated")
	}
	if err := applyWorkerOrcaWorkspace(fixedConfig, run, execution, worktree, run.WorkspaceIntent.BaseCommit); err != nil {
		return err
	}
	if operation.Status != "confirmed" {
		return s.changeWorkerOperation(run, operation.OperationID, "confirmed", operation.PlatformRequestID, mustJSON(worktree), "")
	}
	return s.commitWorker(run)
}
