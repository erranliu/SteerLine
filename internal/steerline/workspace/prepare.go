package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

type Preparation struct {
	Status             executionbackend.OperationStatus      `json:"status"`
	PathCreated        bool                                  `json:"path_created"`
	WorktreeRegistered bool                                  `json:"worktree_registered"`
	BranchPresent      bool                                  `json:"branch_present"`
	Inspection         *executionbackend.WorkspaceInspection `json:"inspection,omitempty"`
}

func Prepare(ctx context.Context, git Git, repository Repository, root string, intent *executionbackend.WorkspaceIntent) (Preparation, error) {
	result := Preparation{Status: executionbackend.OperationUnknown}
	if intent == nil || !filepath.IsAbs(root) || !filepath.IsAbs(intent.Path) || canonical(filepath.Dir(intent.Path)) != canonical(root) {
		result.Status = executionbackend.OperationRejected
		return result, errors.New("workspace intent does not match the validated worktree root")
	}
	if err := validateOperationRoot(root); err != nil {
		result.Status = executionbackend.OperationRejected
		return result, err
	}
	if err := repository.Verify(ctx, git); err != nil {
		result.Status = executionbackend.OperationRejected
		return result, err
	}
	if !commitPattern.MatchString(intent.BaseCommit) || intent.Branch == "" {
		result.Status = executionbackend.OperationRejected
		return result, errors.New("workspace branch or pinned base is invalid")
	}
	if _, err := git.Output(ctx, repository.Path, "check-ref-format", "refs/heads/"+intent.Branch); err != nil {
		result.Status = executionbackend.OperationRejected
		return result, err
	}
	if _, err := os.Lstat(intent.Path); !os.IsNotExist(err) {
		result.Status = executionbackend.OperationRejected
		return result, fmt.Errorf("worktree path already exists or is inaccessible: %s", intent.Path)
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return result, err
	}
	refs, err := git.Output(ctx, repository.Path, "for-each-ref", "--format=%(refname)", "refs/heads/"+intent.Branch)
	if err != nil {
		return result, err
	}
	if containsExactLine(string(refs), "refs/heads/"+intent.Branch) {
		result.BranchPresent, result.Status = true, executionbackend.OperationRejected
		return result, errors.New("worktree branch already exists")
	}
	if err = os.Mkdir(intent.Path, 0700); err != nil {
		return result, err
	}
	result.PathCreated = true
	if _, err = git.Output(ctx, repository.Path, "worktree", "add", "--no-track", "-b", intent.Branch, "--", intent.Path, intent.BaseCommit); err != nil {
		result = observePreparation(ctx, git, repository, intent, result)
		return result, err
	}
	if err := Verify(ctx, git, repository, intent, true); err != nil {
		result = observePreparation(ctx, git, repository, intent, result)
		return result, err
	}
	inspection, err := Inspect(ctx, git, repository, intent)
	if err != nil {
		result = observePreparation(ctx, git, repository, intent, result)
		return result, err
	}
	result.Status, result.WorktreeRegistered, result.BranchPresent = executionbackend.OperationConfirmed, true, true
	result.Inspection = &inspection
	return result, nil
}

func observePreparation(ctx context.Context, git Git, repository Repository, intent *executionbackend.WorkspaceIntent, result Preparation) Preparation {
	worktrees, err := git.Output(ctx, repository.Path, "worktree", "list", "--porcelain", "-z")
	if err == nil {
		for _, record := range strings.Split(string(worktrees), "\x00") {
			if strings.HasPrefix(record, "worktree ") && canonical(strings.TrimPrefix(record, "worktree ")) == canonical(intent.Path) {
				result.WorktreeRegistered = true
				break
			}
		}
	}
	refs, err := git.Output(ctx, repository.Path, "for-each-ref", "--format=%(refname)", "refs/heads/"+intent.Branch)
	if err == nil {
		result.BranchPresent = containsExactLine(string(refs), "refs/heads/"+intent.Branch)
	}
	if result.WorktreeRegistered {
		if inspection, inspectErr := Inspect(ctx, git, repository, intent); inspectErr == nil {
			result.Inspection = &inspection
		}
	}
	return result
}

func containsExactLine(value, expected string) bool {
	for _, line := range strings.Split(strings.TrimSpace(value), "\n") {
		if strings.TrimSpace(line) == expected {
			return true
		}
	}
	return false
}
