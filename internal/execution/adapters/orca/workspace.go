package orcaworker

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

const WorktreeTrashDirectory = ".orca-worktree-trash"

func WorkerWorktreeName(taskID, workflowRunID string) (string, error) {
	number, err := strconv.ParseUint(taskID, 10, 64)
	if err != nil || number == 0 || strconv.FormatUint(number, 10) != taskID || !executionbackend.StableExecutionID(workflowRunID) {
		return "", errors.New("Orca worktree requires a canonical task number and stable workflow identity")
	}
	return "sl-" + taskID + "-" + workflowRunID, nil
}

var ErrWorktreeIdentity = errors.New("Orca worktree does not match the fixed name, repository, base and branch")

type Worktree struct {
	ID               string `json:"id"`
	RepoID           string `json:"repoId"`
	Path             string `json:"path"`
	Branch           string `json:"branch"`
	Head             string `json:"head"`
	DisplayName      string `json:"displayName"`
	BaseRef          string `json:"baseRef"`
	Comment          string `json:"comment,omitempty"`
	CreatedWithAgent string `json:"createdWithAgent,omitempty"`
}

// WorkspaceBinding preserves native path values, including remote paths whose
// syntax belongs to a different host. Local filesystem validation is separate.
func WorkspaceBinding(value Worktree, repository, base string) (executionbackend.WorkspaceInspection, error) {
	branch := strings.TrimPrefix(value.Branch, "refs/heads/")
	if branch == "" || value.Path == "" || value.Head != base {
		return executionbackend.WorkspaceInspection{}, errors.New("remote worker worktree identity is incomplete")
	}
	return executionbackend.WorkspaceInspection{Repository: repository, Worktree: value.Path, Branch: branch, Base: base, Head: base, CheckedAt: time.Now().UTC()}, nil
}

func ValidateWorktree(value Worktree, name, repoID, base string) error {
	branch := strings.TrimPrefix(value.Branch, "refs/heads/")
	if value.DisplayName != name || value.RepoID != repoID || value.ID == "" ||
		!filepath.IsAbs(filepath.FromSlash(value.Path)) || branch == "" ||
		value.Head != base || value.BaseRef != base {
		return ErrWorktreeIdentity
	}
	return nil
}

func FindWorktree(ctx context.Context, call Call, repoID, name, base string) (Worktree, bool, error) {
	raw, err := call(ctx, "worktree", "list", "--repo", "id:"+repoID, "--limit", "1000", "--json")
	if err != nil {
		return Worktree{}, false, err
	}
	var listing struct {
		Worktrees []Worktree `json:"worktrees"`
		Truncated bool       `json:"truncated"`
	}
	if err := json.Unmarshal(raw, &listing); err != nil || listing.Truncated {
		return Worktree{}, false, errors.Join(err, errors.New("Orca worktree list is incomplete"))
	}
	var found *Worktree
	for i := range listing.Worktrees {
		candidate := &listing.Worktrees[i]
		if candidate.DisplayName != name {
			continue
		}
		if found != nil {
			return Worktree{}, false, errors.New("more than one Orca worktree has the fixed worker name")
		}
		found = candidate
	}
	if found == nil {
		return Worktree{}, false, nil
	}
	if err := ValidateWorktree(*found, name, repoID, base); err != nil {
		return Worktree{}, false, err
	}
	return *found, true, nil
}
