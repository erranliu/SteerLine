package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

type workerOrcaWorktree struct {
	ID          string `json:"id"`
	RepoID      string `json:"repoId"`
	Path        string `json:"path"`
	Branch      string `json:"branch"`
	Head        string `json:"head"`
	DisplayName string `json:"displayName"`
	BaseRef     string `json:"baseRef"`
}

func workerOrcaCall(ctx context.Context, c Config, args ...string) (orcaEnvelope, error) {
	if c.hostName() != "orca" && c.hostName() != "orca_supervised" {
		return orcaEnvelope{}, errors.New("worker Orca call has no Orca execution target")
	}
	if c.hostName() == "orca_supervised" {
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

func workerOrcaWorktreeName(runID string) (string, error) {
	if !stableExecutionID(runID) {
		return "", errors.New("Orca worktree requires a stable workflow identity")
	}
	return "sl-" + runID, nil
}

func validateWorkerOrcaWorktree(value workerOrcaWorktree, name, repoID, base string) error {
	branch := strings.TrimPrefix(value.Branch, "refs/heads/")
	if value.DisplayName != name || value.RepoID != repoID || value.ID == "" ||
		!filepath.IsAbs(filepath.FromSlash(value.Path)) || branch == "" ||
		value.Head != base || value.BaseRef != base {
		return &BackendError{Code: BackendIdentityMismatch, Diagnostic: "Orca worktree does not match the fixed name, repository, base and branch"}
	}
	return nil
}

func findWorkerOrcaWorktree(ctx context.Context, c Config, repoID, name, base string) (workerOrcaWorktree, bool, error) {
	env, err := workerOrcaCall(ctx, c, "worktree", "list", "--repo", "id:"+repoID, "--limit", "1000", "--json")
	if err != nil {
		return workerOrcaWorktree{}, false, err
	}
	var listing struct {
		Worktrees []workerOrcaWorktree `json:"worktrees"`
		Truncated bool                 `json:"truncated"`
	}
	if err := json.Unmarshal(env.Result, &listing); err != nil || listing.Truncated {
		return workerOrcaWorktree{}, false, errors.Join(err, errors.New("Orca worktree list is incomplete"))
	}
	var found *workerOrcaWorktree
	for i := range listing.Worktrees {
		candidate := &listing.Worktrees[i]
		if candidate.DisplayName != name {
			continue
		}
		if found != nil {
			return workerOrcaWorktree{}, false, errors.New("more than one Orca worktree has the fixed worker name")
		}
		found = candidate
	}
	if found == nil {
		return workerOrcaWorktree{}, false, nil
	}
	if err := validateWorkerOrcaWorktree(*found, name, repoID, base); err != nil {
		return workerOrcaWorktree{}, false, err
	}
	return *found, true, nil
}

func createWorkerOrcaWorktree(ctx context.Context, c Config, repoID, name, base string) (workerOrcaWorktree, error) {
	if found, exists, err := findWorkerOrcaWorktree(ctx, c, repoID, name, base); err != nil {
		return workerOrcaWorktree{}, err
	} else if exists {
		return found, nil
	}
	env, callErr := workerOrcaCall(ctx, c, "worktree", "create", "--repo", "id:"+repoID, "--name", name, "--base-branch", base, "--no-parent", "--setup", "skip", "--json")
	if callErr != nil {
		if found, exists, err := findWorkerOrcaWorktree(ctx, c, repoID, name, base); err == nil && exists {
			return found, nil
		}
		return workerOrcaWorktree{}, &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "remote worker worktree creation has no confirmed receipt", Cause: callErr}
	}
	var result struct {
		Worktree workerOrcaWorktree `json:"worktree"`
	}
	if err := json.Unmarshal(env.Result, &result); err != nil {
		return workerOrcaWorktree{}, err
	}
	if err := validateWorkerOrcaWorktree(result.Worktree, name, repoID, base); err != nil {
		return workerOrcaWorktree{}, err
	}
	return result.Worktree, nil
}

func applyWorkerOrcaWorkspace(c Config, run *Run, execution *ExecutionRecord, worktree workerOrcaWorktree, base string) error {
	if run == nil || execution == nil || run.ExecutionID != execution.ExecutionID || !commitHash.MatchString(base) {
		return errors.New("remote worker worktree lacks its fixed execution or base")
	}
	branch := strings.TrimPrefix(worktree.Branch, "refs/heads/")
	if branch == "" || worktree.Path == "" || worktree.Head != base {
		return errors.New("remote worker worktree identity is incomplete")
	}
	path := filepath.FromSlash(worktree.Path)
	run.WorkspaceIntent = &WorkspaceIntent{Path: path, Branch: branch, BaseCommit: base}
	run.WorktreePath, run.WorktreeBranch, run.BaseCommit = ptr(path), ptr(branch), ptr(base)
	run.Target.WorktreeID, run.Target.Branch = worktree.ID, branch
	execution.Target.WorktreeID, execution.Target.Branch = worktree.ID, branch
	execution.Workspace = WorkspaceEvidence{Repository: c.identity, Worktree: path, Branch: branch, Base: base, Head: base, VerifiedAt: time.Now().UTC()}
	return nil
}

func (s *Service) prepareRemoteWorkerWorkspace(ctx context.Context, run *Run, execution *ExecutionRecord, operationID string) error {
	if s.Config.hostName() != "orca_supervised" || run == nil || execution == nil || run.WorkspaceIntent == nil {
		return errors.New("remote worker workspace has no fixed preparation intent")
	}
	base := run.WorkspaceIntent.BaseCommit
	name, err := workerOrcaWorktreeName(run.RunID)
	if err != nil {
		return err
	}
	pair, err := probeSupervisedOrcaTarget(ctx, s.Config)
	if err != nil {
		return err
	}
	worktree, err := createWorkerOrcaWorktree(ctx, s.Config, pair.RepositoryID, name, base)
	if err != nil {
		return err
	}
	if err := applyWorkerOrcaWorkspace(s.Config, run, execution, worktree, base); err != nil {
		return err
	}
	if operationID != "" {
		if err := s.changeOperation(operationID, "confirmed", "", mustJSON(worktree), ""); err != nil {
			return fmt.Errorf("remote worktree created but receipt persistence failed: %w", err)
		}
	}
	return nil
}

func (s *Service) recoverRemoteWorkerWorkspace(ctx context.Context, run *Run, execution *ExecutionRecord, operation *OperationRecord) error {
	if s.Config.hostName() != "orca_supervised" || run == nil || execution == nil || operation == nil || operation.Kind != "prepare_workspace" || run.WorkspaceIntent == nil {
		return errors.New("remote workspace recovery has no original preparation intent")
	}
	name, err := workerOrcaWorktreeName(run.RunID)
	if err != nil {
		return err
	}
	pair, err := probeSupervisedOrcaTarget(ctx, s.Config)
	if err != nil {
		return err
	}
	worktree, found, err := findWorkerOrcaWorktree(ctx, s.Config, pair.RepositoryID, name, run.WorkspaceIntent.BaseCommit)
	if err != nil {
		return err
	}
	if !found {
		return errors.New("saved Orca worktree creation has no matching remote worktree; original request will not be repeated")
	}
	if err := applyWorkerOrcaWorkspace(s.Config, run, execution, worktree, run.WorkspaceIntent.BaseCommit); err != nil {
		return err
	}
	if operation.Status != "confirmed" {
		return s.changeOperation(operation.OperationID, "confirmed", operation.PlatformRequestID, mustJSON(worktree), "")
	}
	return s.save()
}
