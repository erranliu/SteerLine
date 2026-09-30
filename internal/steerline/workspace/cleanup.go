package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/erranliu/SteerLine/internal/steerline/execution"
)

type CleanupResult struct {
	Status       execution.OperationStatus `json:"status"`
	WorktreeGone bool                      `json:"worktree_absent"`
	BranchGone   bool                      `json:"branch_absent"`
}

func Cleanup(ctx context.Context, git Git, repository Repository, root string, intent *Intent) (CleanupResult, error) {
	result := CleanupResult{Status: execution.OperationUnknown}
	if intent == nil || !filepath.IsAbs(root) || !filepath.IsAbs(intent.Path) || canonical(filepath.Dir(intent.Path)) != canonical(root) || intent.Branch == "" || !commitPattern.MatchString(intent.BaseCommit) {
		result.Status = execution.OperationRejected
		return result, errors.New("cleanup intent does not match the fixed local workspace identity")
	}
	if err := validateOperationRoot(root); err != nil {
		result.Status = execution.OperationRejected
		return result, err
	}
	if err := repository.Verify(ctx, git); err != nil {
		result.Status = execution.OperationRejected
		return result, err
	}
	if _, err := git.Output(ctx, repository.Path, "check-ref-format", "refs/heads/"+intent.Branch); err != nil {
		result.Status = execution.OperationRejected
		return result, errors.New("workspace branch identity is invalid")
	}
	worktrees, err := git.Output(ctx, repository.Path, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return observeCleanup(ctx, git, repository, intent, result, err)
	}
	registered := false
	for _, record := range strings.Split(string(worktrees), "\x00") {
		if strings.HasPrefix(record, "worktree ") && canonical(strings.TrimPrefix(record, "worktree ")) == canonical(intent.Path) {
			registered = true
			break
		}
	}
	if registered {
		if err := Verify(ctx, git, repository, intent, false); err != nil {
			return result, fmt.Errorf("worktree identity could not be verified: %w", err)
		}
		if _, err := git.Output(ctx, repository.Path, "worktree", "remove", "--force", "--", intent.Path); err != nil {
			return observeCleanup(ctx, git, repository, intent, result, err)
		}
	} else if _, statErr := os.Lstat(intent.Path); statErr == nil {
		result.Status = execution.OperationRejected
		return result, errors.New("workspace path exists but is not the fixed Git worktree")
	} else if !os.IsNotExist(statErr) {
		return result, statErr
	}
	result.WorktreeGone = true
	if _, statErr := os.Lstat(intent.Path); statErr == nil {
		result.WorktreeGone = false
		return result, errors.New("workspace path remains after cleanup")
	} else if !os.IsNotExist(statErr) {
		result.WorktreeGone = false
		return result, statErr
	}
	refs, err := git.Output(ctx, repository.Path, "for-each-ref", "--format=%(refname)", "refs/heads/"+intent.Branch)
	if err != nil {
		return observeCleanup(ctx, git, repository, intent, result, err)
	}
	branchRef := "refs/heads/" + intent.Branch
	branchPresent := containsExactLine(string(refs), branchRef)
	if !branchPresent {
		result.BranchGone = true
	}
	if branchPresent {
		if _, err := git.Output(ctx, repository.Path, "merge-base", "--is-ancestor", intent.BaseCommit, branchRef); err != nil {
			return result, errors.New("workspace branch no longer contains its pinned base")
		}
		if _, err := git.Output(ctx, repository.Path, "branch", "-D", "--", intent.Branch); err != nil {
			return observeCleanup(ctx, git, repository, intent, result, err)
		}
	}
	worktrees, err = git.Output(ctx, repository.Path, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return observeCleanup(ctx, git, repository, intent, result, err)
	}
	for _, record := range strings.Split(string(worktrees), "\x00") {
		if strings.HasPrefix(record, "worktree ") && canonical(strings.TrimPrefix(record, "worktree ")) == canonical(intent.Path) {
			result.WorktreeGone = false
			return result, errors.New("worktree remains registered after cleanup")
		}
	}
	refs, err = git.Output(ctx, repository.Path, "for-each-ref", "--format=%(refname)", "refs/heads/"+intent.Branch)
	if err != nil {
		return observeCleanup(ctx, git, repository, intent, result, err)
	}
	if containsExactLine(string(refs), branchRef) {
		result.BranchGone = false
		return result, errors.New("workspace branch remains after cleanup")
	}
	result.BranchGone = true
	result.Status = execution.OperationConfirmed
	return result, nil
}

func observeCleanup(ctx context.Context, git Git, repository Repository, intent *Intent, result CleanupResult, cause error) (CleanupResult, error) {
	if _, err := os.Lstat(intent.Path); os.IsNotExist(err) {
		result.WorktreeGone = true
	} else if err == nil {
		result.WorktreeGone = false
	}
	if worktrees, err := git.Output(ctx, repository.Path, "worktree", "list", "--porcelain", "-z"); err == nil {
		registered := false
		for _, record := range strings.Split(string(worktrees), "\x00") {
			registered = registered || strings.HasPrefix(record, "worktree ") && canonical(strings.TrimPrefix(record, "worktree ")) == canonical(intent.Path)
		}
		if registered {
			result.WorktreeGone = false
		} else if result.WorktreeGone {
			result.WorktreeGone = true
		}
	}
	if refs, err := git.Output(ctx, repository.Path, "for-each-ref", "--format=%(refname)", "refs/heads/"+intent.Branch); err == nil {
		result.BranchGone = !containsExactLine(string(refs), "refs/heads/"+intent.Branch)
	}
	result.Status = execution.OperationUnknown
	return result, cause
}
