//go:build e2e

package testenv

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/tests/e2e/internal/controlled"
)

// The caller invokes this only after original Runner and owned-ref settlement.
// setup-delete removes this test's registration and saved details, not its files.
func (r *orcaTarget) cleanupLocalRegistration(s *Scenario, p *Project, owned map[string]string) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := r.removeLocalRegistration(ctx, p.Repo, owned, func(args ...string) (json.RawMessage, error) { return r.Call(ctx, s, p, args...) }, func() (bool, error) { return localWorktreeFilesSettled(ctx, p.Repo, owned) }); err != nil {
		return err
	}
	return controlled.Write(filepath.Join(s.Output, "orca-local-registration-cleanup.json"), map[string]any{"confirmed": true, "setup": r.Repository, "path": r.Root, "host": "local", "source_files_preserved": true})
}

func (r *orcaTarget) removeLocalRegistration(ctx context.Context, root string, owned map[string]string, call func(...string) (json.RawMessage, error), filesSettled func() (bool, error)) error {
	if !r.ownedLocalSetup || r.Environment != "" || r.Repository == "" || !sameOrcaPath(r.Root, root) {
		return errors.New("local registration ownership differs")
	}
	body, err := call("project", "setups")
	if err != nil {
		return err
	}
	var setups struct {
		Setups    []struct{ ID, RepoID, HostID, Path string } `json:"setups"`
		Truncated bool                                        `json:"truncated"`
	}
	if json.Unmarshal(body, &setups) != nil || setups.Setups == nil || setups.Truncated {
		return errors.New("local setup inventory is incomplete")
	}
	found := 0
	for _, setup := range setups.Setups {
		if setup.ID == r.Repository {
			if setup.RepoID != r.Repository || setup.HostID != "local" || !sameOrcaPath(setup.Path, root) {
				return errors.New("original local setup identity changed")
			}
			found++
		}
	}
	if found != 1 {
		return errors.New("original local setup is missing or ambiguous")
	}
	body, err = call("worktree", "list", "--repo", "id:"+r.Repository)
	if err != nil {
		return err
	}
	var trees struct {
		Worktrees  []orcaCleanupWorkspace `json:"worktrees"`
		TotalCount *int                   `json:"totalCount"`
		Truncated  *bool                  `json:"truncated"`
	}
	if json.Unmarshal(body, &trees) != nil || trees.Worktrees == nil || trees.Truncated == nil || *trees.Truncated || trees.TotalCount == nil || *trees.TotalCount != len(trees.Worktrees) {
		return errors.New("own local workspace inventory is incomplete")
	}
	remove, err := ownedOrcaWorktrees(r.Repository, root, trees.Worktrees, owned)
	if err != nil {
		return err
	}
	empty := func(id string) error {
		body, err := call("terminal", "list", "--worktree", "id:"+id)
		if err != nil {
			return err
		}
		var terminals struct {
			Terminals  []json.RawMessage                          `json:"terminals"`
			TotalCount *int                                       `json:"totalCount"`
			Truncated  *bool                                      `json:"truncated"`
			HostScope  struct{ HostIDs, OmittedHostIDs []string } `json:"hostScope"`
		}
		if json.Unmarshal(body, &terminals) != nil || terminals.Terminals == nil || len(terminals.Terminals) != 0 || terminals.TotalCount == nil || *terminals.TotalCount != 0 || terminals.Truncated == nil || *terminals.Truncated || terminals.HostScope.OmittedHostIDs == nil {
			return errors.New("own local setup terminal census is incomplete")
		}
		local := false
		for _, host := range terminals.HostScope.HostIDs {
			local = local || host == "local"
		}
		for _, host := range terminals.HostScope.OmittedHostIDs {
			if host == "local" {
				return errors.New("own local host was omitted")
			}
		}
		// Other paired hosts may be unavailable. This newly created setup and its
		// sole source workspace are explicitly local; no remote resource is owned.
		if !local {
			return errors.New("own local host is not listed")
		}
		return nil
	}
	for _, tree := range trees.Worktrees {
		if err = empty(tree.ID); err != nil {
			return err
		}
	}
	for _, id := range remove {
		if _, err = call("worktree", "rm", "--worktree", "id:"+id); err != nil {
			return err
		}
	}
	// Orca removal is asynchronous. Never resend rm: wait on read-only public,
	// filesystem and Git inventories before removing this test's registration.
	for {
		body, err = call("worktree", "list", "--repo", "id:"+r.Repository)
		if err != nil {
			return err
		}
		trees.Worktrees, trees.TotalCount, trees.Truncated = nil, nil, nil
		if json.Unmarshal(body, &trees) != nil || trees.Worktrees == nil || trees.Truncated == nil || *trees.Truncated || trees.TotalCount == nil || *trees.TotalCount != len(trees.Worktrees) {
			return errors.New("final local workspace inventory is incomplete")
		}
		remaining, err := ownedOrcaWorktrees(r.Repository, root, trees.Worktrees, owned)
		if err != nil {
			return err
		}
		settled, err := filesSettled()
		if err != nil {
			return err
		}
		if len(remaining) == 0 && settled {
			break
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("owned workspace deletion did not settle: %w", ctx.Err())
		case <-time.After(200 * time.Millisecond):
		}
	}
	if err = empty(trees.Worktrees[0].ID); err != nil {
		return err
	}
	if _, err = call("project", "setup-delete", "--setup", r.Repository, "--force"); err != nil {
		return err
	}
	body, err = call("project", "setups")
	if err != nil {
		return err
	}
	setups.Setups, setups.Truncated = nil, false
	if json.Unmarshal(body, &setups) != nil || setups.Setups == nil || setups.Truncated {
		return errors.New("final local setup inventory is incomplete")
	}
	for _, setup := range setups.Setups {
		if setup.ID == r.Repository {
			return errors.New("own local registration remains")
		}
	}
	return nil
}

func localWorktreeFilesSettled(ctx context.Context, root string, owned map[string]string) (bool, error) {
	for path := range owned {
		if _, err := os.Stat(path); err == nil {
			return false, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
	}
	body, err := exec.CommandContext(ctx, "git", "-C", root, "worktree", "list", "--porcelain", "-z").Output()
	if err != nil {
		return false, err
	}
	for _, field := range strings.Split(string(body), "\x00") {
		if !strings.HasPrefix(field, "worktree ") {
			continue
		}
		for path := range owned {
			if sameOrcaPath(strings.TrimPrefix(field, "worktree "), path) {
				return false, nil
			}
		}
	}
	return true, nil
}
