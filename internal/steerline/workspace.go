package steerline

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var commitHash = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

func (c *Config) prepareWorktreeRoot() error {
	if c.WorktreeRoot == "" {
		c.WorktreeRoot = filepath.Join(c.RunDir, "worktrees")
	}
	if !filepath.IsAbs(c.WorktreeRoot) {
		return errors.New("worktree_root must resolve to an absolute path")
	}
	// Resolve ancestors before creating the directory, including junctions on Windows.
	root, e := resolveFuturePath(c.WorktreeRoot)
	if e != nil {
		return e
	}
	if inside(c.Repository, root) || inside(c.identity, root) || inside(root, c.RunDir) || inside(filepath.Join(c.RunDir, "runs"), root) || inside(root, filepath.Join(c.RunDir, "runs")) {
		return errors.New("worktree_root overlaps a checkout, Git metadata or runtime records")
	}
	c.WorktreeRoot = root
	return nil
}
func resolveFuturePath(path string) (string, error) {
	resolved, e := filepath.EvalSymlinks(path)
	if e == nil {
		return resolved, nil
	}
	if !os.IsNotExist(e) {
		return "", e
	}
	parent := filepath.Dir(path)
	if parent == path {
		return "", e
	}
	resolved, e = resolveFuturePath(parent)
	if e != nil {
		return "", e
	}
	return filepath.Join(resolved, filepath.Base(path)), nil
}
func gitRunner(c Config, dir string) Runner {
	return Runner{Dir: dir, Timeout: duration(c.CommandTimeout), Env: map[string]string{"GIT_TERMINAL_PROMPT": "0"}}
}

func planWorkspace(c Config, taskID, runID, base string) (*WorkspaceIntent, error) {
	if !regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`).MatchString(taskID) || strings.Contains(taskID, "..") {
		return nil, errors.New("task ID cannot safely form a branch name")
	}
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(runID) || !commitHash.MatchString(base) {
		return nil, errors.New("invalid workspace identity")
	}
	return &WorkspaceIntent{Path: filepath.Join(c.WorktreeRoot, runID), Branch: "codex/" + taskID + "-" + runID[:12], BaseCommit: base}, nil
}
func verifyWorkspace(ctx context.Context, c Config, i *WorkspaceIntent, initial bool) error {
	if i == nil || !filepath.IsAbs(i.Path) || i.Branch == "" || !commitHash.MatchString(i.BaseCommit) {
		return errors.New("incomplete workspace intent")
	}
	real, e := filepath.EvalSymlinks(i.Path)
	if e != nil {
		return e
	}
	if canonical(real) != canonical(i.Path) {
		return errors.New("worktree path was redirected")
	}
	r := gitRunner(c, i.Path)
	for _, check := range []struct {
		args []string
		want string
		path bool
	}{
		{[]string{"rev-parse", "--show-toplevel"}, i.Path, true},
		{[]string{"rev-parse", "--path-format=absolute", "--git-common-dir"}, c.identity, true},
		{[]string{"symbolic-ref", "--quiet", "HEAD"}, "refs/heads/" + i.Branch, false},
	} {
		out, e := r.Output(ctx, c.Git, check.args...)
		if e != nil {
			return e
		}
		got := strings.TrimSpace(string(out))
		want := check.want
		if check.path {
			got, e = filepath.EvalSymlinks(got)
			if e != nil {
				return e
			}
			got = canonical(got)
			want = canonical(want)
		}
		if got != want {
			return fmt.Errorf("worktree identity mismatch: expected %s", want)
		}
	}
	if initial {
		out, e := r.Output(ctx, c.Git, "rev-parse", "HEAD")
		if e != nil {
			return e
		}
		if strings.TrimSpace(string(out)) != i.BaseCommit {
			return errors.New("worktree HEAD differs from pinned base")
		}
	} else {
		if _, e := r.Output(ctx, c.Git, "merge-base", "--is-ancestor", i.BaseCommit, "HEAD"); e != nil {
			return errors.New("worktree HEAD no longer contains pinned base")
		}
	}
	return nil
}
