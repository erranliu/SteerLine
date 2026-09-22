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

// Each fetch writes a run-specific ref, so another local fetch cannot change the
// commit selected for this run by overwriting FETCH_HEAD or remote-tracking refs.
func fetchBase(ctx context.Context, c Config, runID string) (string, error) {
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(runID) {
		return "", errors.New("invalid run ID")
	}
	r := gitRunner(c, c.Repository)
	if _, e := r.Output(ctx, c.Git, "check-ref-format", "refs/heads/"+c.Branch); e != nil {
		return "", e
	}
	ref := "refs/steerline/bases/" + runID
	if _, e := r.Output(ctx, c.Git, "fetch", "--no-tags", "--no-recurse-submodules", "--no-write-fetch-head", "--", c.Remote, "refs/heads/"+c.Branch+":"+ref); e != nil {
		return "", fmt.Errorf("fetch mainline: %w", e)
	}
	out, e := r.Output(ctx, c.Git, "rev-parse", "--verify", ref+"^{commit}")
	if e != nil {
		return "", e
	}
	sha := strings.TrimSpace(string(out))
	if !commitHash.MatchString(sha) {
		return "", errors.New("invalid fetched commit hash")
	}
	return sha, nil
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
func prepareWorkspace(ctx context.Context, c Config, r *Run, source BeadsContext) (Config, error) {
	i := r.WorkspaceIntent
	if i == nil {
		return c, errors.New("missing durable workspace intent")
	}
	if e := c.prepareWorktreeRoot(); e != nil {
		return c, e
	}
	if canonical(filepath.Dir(i.Path)) != canonical(c.WorktreeRoot) {
		return c, errors.New("workspace intent path no longer matches worktree_root")
	}
	if _, e := os.Lstat(i.Path); !os.IsNotExist(e) {
		return c, fmt.Errorf("worktree path already exists or is inaccessible: %s", i.Path)
	}
	if e := os.MkdirAll(c.WorktreeRoot, 0700); e != nil {
		return c, e
	}
	git := gitRunner(c, c.Repository)
	if _, e := git.Output(ctx, c.Git, "check-ref-format", "refs/heads/"+i.Branch); e != nil {
		return c, e
	}
	refs, e := git.Output(ctx, c.Git, "for-each-ref", "--format=%(refname)", "refs/heads/"+i.Branch)
	if e != nil {
		return c, e
	}
	if strings.TrimSpace(string(refs)) != "" {
		return c, errors.New("worktree branch already exists")
	}
	// Reserve this previously absent directory atomically. Git can populate our
	// empty reservation, but a concurrently created path is never reused.
	if e = os.Mkdir(i.Path, 0700); e != nil {
		return c, e
	}
	// No force, reset or reuse: leave partial creation intact on any failure.
	if _, e = git.Output(ctx, c.Git, "worktree", "add", "--no-track", "-b", i.Branch, "--", i.Path, i.BaseCommit); e != nil {
		return c, e
	}
	if e = verifyWorkspace(ctx, c, i, true); e != nil {
		return c, e
	}
	return workerForWorkspace(ctx, c, r, source)
}

// Reused for an explicit clean-start retry without creating a new workspace or claim.
func workerForWorkspace(ctx context.Context, c Config, r *Run, source BeadsContext) (Config, error) {
	i := r.WorkspaceIntent
	if i == nil {
		return c, errors.New("missing workspace intent")
	}
	worker := c
	worker.Repository = i.Path
	for _, pair := range []struct {
		from Skill
		to   *Skill
	}{{c.StartSkill, &worker.StartSkill}, {c.FinishSkill, &worker.FinishSkill}} {
		path := pair.from.Path
		if !filepath.IsAbs(path) {
			path = filepath.Join(c.Repository, path)
		}
		rel, e := filepath.Rel(c.Repository, path)
		if e != nil || !inside(c.Repository, path) {
			return c, errors.New("skill path is outside source checkout")
		}
		pair.to.Path = filepath.Join(i.Path, rel)
	}
	if e := worker.validateSkills(); e != nil {
		return c, e
	}
	worker.workerEnv = map[string]string{"BEADS_DIR": source.Directory, "BEADS_ACTOR": r.Claimant}
	beads := Beads{Runner: Runner{Dir: i.Path, Timeout: duration(c.CommandTimeout), Env: worker.workerEnv}, Command: c.BD, Claimant: r.Claimant}
	actual, e := beads.Context(ctx)
	if e != nil {
		return c, e
	}
	if !source.Same(actual) {
		return c, errors.New("worktree Beads project/database does not match source")
	}
	task, e := beads.Show(ctx, r.TaskID)
	if e != nil {
		return c, e
	}
	if task.Status != "in_progress" || task.Assignee != r.Claimant {
		return c, errors.New("worktree task claim does not match dispatch")
	}
	return worker, nil
}

func verifyRunWorkspace(ctx context.Context, c Config, r *Run) error {
	i := r.WorkspaceIntent
	if i == nil || r.WorktreePath == nil || r.WorktreeBranch == nil || r.BaseCommit == nil {
		return errors.New("run has no verified scheduler-prepared workspace")
	}
	if canonical(*r.WorktreePath) != canonical(i.Path) || *r.WorktreeBranch != i.Branch || *r.BaseCommit != i.BaseCommit {
		return errors.New("prepared workspace fields disagree with durable intent")
	}
	return verifyWorkspace(ctx, c, i, false)
}

// Read-only verification works after implementation too: HEAD may advance, but
// the recorded base must remain in its history and the assigned branch must match.
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
