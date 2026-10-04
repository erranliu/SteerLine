package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

var commitPattern = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

type Repository struct {
	Path      string
	CommonDir string
}

func ResolveRepository(ctx context.Context, git Git, checkout string) (Repository, error) {
	root, err := git.Output(ctx, checkout, "rev-parse", "--show-toplevel")
	if err != nil {
		return Repository{}, err
	}
	path, err := resolveGitPath(strings.TrimSpace(string(root)))
	if err != nil {
		return Repository{}, err
	}
	common, err := git.Output(ctx, path, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return Repository{}, err
	}
	commonPath, err := resolveGitPath(strings.TrimSpace(string(common)))
	if err != nil {
		return Repository{}, err
	}
	return Repository{Path: path, CommonDir: commonPath}, nil
}

func (r Repository) Verify(ctx context.Context, git Git) error {
	if !filepath.IsAbs(r.Path) || !filepath.IsAbs(r.CommonDir) {
		return errors.New("fixed repository identity must contain absolute checkout and Git common paths")
	}
	actual, err := ResolveRepository(ctx, git, r.Path)
	if err != nil {
		return err
	}
	if canonical(actual.Path) != canonical(r.Path) || canonical(actual.CommonDir) != canonical(r.CommonDir) {
		return errors.New("Git repository differs from its fixed checkout or common directory")
	}
	return nil
}

func Verify(ctx context.Context, git Git, repository Repository, intent *executionbackend.WorkspaceIntent, initial bool) error {
	if intent == nil || !filepath.IsAbs(intent.Path) || intent.Branch == "" || !commitPattern.MatchString(intent.BaseCommit) {
		return errors.New("incomplete workspace intent")
	}
	if err := repository.Verify(ctx, git); err != nil {
		return err
	}
	real, err := filepath.EvalSymlinks(intent.Path)
	if err != nil {
		return err
	}
	if canonical(real) != canonical(intent.Path) {
		return errors.New("worktree path was redirected")
	}
	for _, check := range []struct {
		args []string
		want string
		path bool
	}{
		{[]string{"rev-parse", "--show-toplevel"}, intent.Path, true},
		{[]string{"rev-parse", "--path-format=absolute", "--git-common-dir"}, repository.CommonDir, true},
		{[]string{"symbolic-ref", "--quiet", "HEAD"}, "refs/heads/" + intent.Branch, false},
	} {
		out, err := git.Output(ctx, intent.Path, check.args...)
		if err != nil {
			return err
		}
		got, want := strings.TrimSpace(string(out)), check.want
		if check.path {
			got, err = resolveGitPath(got)
			if err != nil {
				return err
			}
			got, want = canonical(got), canonical(want)
		}
		if got != want {
			return fmt.Errorf("worktree identity mismatch: expected %s", want)
		}
	}
	if initial {
		out, err := git.Output(ctx, intent.Path, "rev-parse", "HEAD")
		if err != nil {
			return err
		}
		if strings.TrimSpace(string(out)) != intent.BaseCommit {
			return errors.New("worktree HEAD differs from pinned base")
		}
	} else if _, err := git.Output(ctx, intent.Path, "merge-base", "--is-ancestor", intent.BaseCommit, "HEAD"); err != nil {
		return fmt.Errorf("worktree HEAD no longer contains pinned base: %w", err)
	}
	return nil
}

func Inspect(ctx context.Context, git Git, repository Repository, intent *executionbackend.WorkspaceIntent) (executionbackend.WorkspaceInspection, error) {
	if err := Verify(ctx, git, repository, intent, false); err != nil {
		return executionbackend.WorkspaceInspection{}, err
	}
	head, err := git.Output(ctx, intent.Path, "rev-parse", "HEAD")
	if err != nil {
		return executionbackend.WorkspaceInspection{}, err
	}
	status, err := git.Output(ctx, intent.Path, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return executionbackend.WorkspaceInspection{}, err
	}
	return executionbackend.WorkspaceInspection{Repository: repository.Path, Worktree: intent.Path, Branch: intent.Branch, Base: intent.BaseCommit, Head: strings.TrimSpace(string(head)), Dirty: len(status) != 0, CheckedAt: time.Now().UTC()}, nil
}

// ValidateRoot resolves existing symlink ancestors before applying the original
// worktree/runtime overlap rules.
func ValidateRoot(repository Repository, worktreeRoot, runDir string) (string, error) {
	if !filepath.IsAbs(worktreeRoot) {
		return "", errors.New("worktree_root must resolve to an absolute path")
	}
	root, err := resolveFuturePath(worktreeRoot)
	if err != nil {
		return "", err
	}
	if inside(repository.Path, root) || inside(repository.CommonDir, root) || inside(root, runDir) || inside(filepath.Join(runDir, "runs"), root) || inside(root, filepath.Join(runDir, "runs")) {
		return "", errors.New("worktree_root overlaps a checkout, Git metadata or runtime records")
	}
	return root, nil
}

func resolveGitPath(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("Git returned a non-absolute path %q", path)
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(resolved), nil
}

func resolveFuturePath(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return resolved, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	parent := filepath.Dir(path)
	if parent == path {
		return "", err
	}
	resolved, err = resolveFuturePath(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(resolved, filepath.Base(path)), nil
}

func validateOperationRoot(root string) error {
	resolved, err := resolveFuturePath(root)
	if err != nil {
		return err
	}
	if canonical(resolved) != canonical(root) {
		return errors.New("worktree root was redirected")
	}
	return nil
}

func inside(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func canonical(path string) string {
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		return strings.ToLower(path)
	}
	return path
}
