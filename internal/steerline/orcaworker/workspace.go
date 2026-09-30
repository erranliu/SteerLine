package orcaworker

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
)

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
