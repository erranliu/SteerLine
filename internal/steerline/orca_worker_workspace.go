package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
)

type workerOrcaWorktree = orcaworker.Worktree

func workerOrcaWorkspaceError(err error) error {
	if errors.Is(err, orcaworker.ErrWorktreeIdentity) {
		return &BackendError{Code: BackendIdentityMismatch, Diagnostic: err.Error()}
	}
	if errors.Is(err, orcaworker.ErrWorktreeOutcomeUnknown) {
		return &BackendError{Code: BackendOutcomeUnknown, Diagnostic: orcaworker.ErrWorktreeOutcomeUnknown.Error(), Cause: err}
	}
	return err
}

func workerOrcaBackendError(err error) error {
	var platform *orcaworker.PlatformError
	if errors.As(err, &platform) {
		return &BackendError{Code: BackendErrorCode(platform.Code), Diagnostic: platform.Diagnostic, Cause: platform.Cause}
	}
	return err
}

func workerOrcaCall(ctx context.Context, c Config, args ...string) (orcaEnvelope, error) {
	if c.hostName() != "orca" && c.hostName() != "orca_supervised" {
		return orcaEnvelope{}, errors.New("worker Orca call has no Orca execution target")
	}
	if c.hostName() == "orca_supervised" && (len(args) == 0 || args[0] != "orchestration") {
		if c.Orca.Supervised == nil || c.Orca.Supervised.EnvironmentID == "" {
			return orcaEnvelope{}, errors.New("remote worker has no fixed Orca environment")
		}
		args = append(args, "--environment", c.Orca.Supervised.EnvironmentID)
	}
	out, callErr := (orcaClient{cfg: c}).call(ctx, positiveDuration(c.CommandTimeout, 30*time.Second), args...)
	envelope, parseErr := parseEnvelope(out)
	if callErr != nil || parseErr != nil {
		return envelope, errors.Join(callErr, parseErr)
	}
	return envelope, nil
}

func orcaWorkerCaller(c Config) orcaworker.Call {
	return func(ctx context.Context, args ...string) (json.RawMessage, error) {
		env, err := workerOrcaCall(ctx, c, args...)
		return env.Result, err
	}
}

func orcaWorkerMetaCaller(c Config) orcaworker.MetaCall {
	return func(ctx context.Context, args ...string) (orcaworker.Result, error) {
		env, err := workerOrcaCall(ctx, c, args...)
		result := orcaworker.Result{Payload: env.Result, RuntimeID: env.Meta.RuntimeID}
		if env.Error != nil {
			result.ErrorCode, result.ErrorData = env.Error.Code, env.Error.Data
		}
		return result, err
	}
}

func workerOrcaWorktreeName(taskID, runID string) (string, error) {
	if !canonicalDecimalID(taskID) || !stableExecutionID(runID) {
		return "", errors.New("Orca worktree requires a canonical task number and stable workflow identity")
	}
	return "sl-" + taskID + "-" + runID, nil
}

func validateWorkerOrcaWorktree(value workerOrcaWorktree, name, repoID, base string) error {
	return workerOrcaWorkspaceError(orcaworker.ValidateWorktree(value, name, repoID, base))
}
func findWorkerOrcaWorktree(ctx context.Context, c Config, repoID, name, base string) (workerOrcaWorktree, bool, error) {
	worktree, found, err := orcaworker.FindWorktree(ctx, orcaWorkerCaller(c), repoID, name, base)
	return worktree, found, workerOrcaWorkspaceError(err)
}
func createWorkerOrcaWorktree(ctx context.Context, c Config, repoID, name, base string) (workerOrcaWorktree, error) {
	worktree, err := orcaworker.CreateWorktree(ctx, orcaWorkerCaller(c), repoID, name, base)
	return worktree, workerOrcaWorkspaceError(err)
}

func applyWorkerOrcaWorkspace(c Config, run *Run, execution *ExecutionRecord, worktree workerOrcaWorktree, base string) error {
	if run == nil || execution == nil || run.ExecutionID != execution.ExecutionID || !commitHash.MatchString(base) {
		return errors.New("remote worker worktree lacks its fixed execution or base")
	}
	branch := strings.TrimPrefix(worktree.Branch, "refs/heads/")
	if branch == "" || worktree.Path == "" || worktree.Head != base {
		return errors.New("remote worker worktree identity is incomplete")
	}
	path := worktree.Path
	execution.WorkspaceIntent = &WorkspaceIntent{Path: path, Branch: branch, BaseCommit: base}
	execution.Target.WorktreeID, execution.Target.Branch = worktree.ID, branch
	execution.Workspace = WorkspaceEvidence{Repository: c.identity, Worktree: path, Branch: branch, Base: base, Head: base, VerifiedAt: time.Now().UTC()}
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
		return &BackendError{Code: BackendIdentityMismatch, Diagnostic: "original remote workspace runtime changed"}
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
		return &BackendError{Code: BackendIdentityMismatch, Diagnostic: "original remote workspace runtime is unverified"}
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
