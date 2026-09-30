package orcaworker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

var ErrWorktreeIdentity = errors.New("Orca worktree does not match the fixed name, repository, base and branch")
var ErrWorktreeOutcomeUnknown = errors.New("remote worker worktree creation has no confirmed receipt")

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

func CreateWorktree(ctx context.Context, call Call, repoID, name, base string) (Worktree, error) {
	if found, exists, err := FindWorktree(ctx, call, repoID, name, base); err != nil {
		return Worktree{}, err
	} else if exists {
		return found, nil
	}
	raw, callErr := call(ctx, "worktree", "create", "--repo", "id:"+repoID, "--name", name, "--base-branch", base, "--no-parent", "--setup", "skip", "--json")
	if callErr != nil {
		if found, exists, err := FindWorktree(ctx, call, repoID, name, base); err == nil && exists {
			return found, nil
		}
		return Worktree{}, fmt.Errorf("%w: %w", ErrWorktreeOutcomeUnknown, callErr)
	}
	var result struct {
		Worktree Worktree `json:"worktree"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return Worktree{}, err
	}
	if err := ValidateWorktree(result.Worktree, name, repoID, base); err != nil {
		return Worktree{}, err
	}
	return result.Worktree, nil
}
