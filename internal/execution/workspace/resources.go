package workspace

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

type Resources struct {
	Objects string
	Refs    string
	Logs    string
	Admin   string
}

// ResolveLinkedResources returns the four fixed writable Git locations for a
// verified linked worktree. It never returns an authorization argument.
func ResolveLinkedResources(ctx context.Context, git Git, repository Repository, intent *executionbackend.WorkspaceIntent) (Resources, error) {
	if err := Verify(ctx, git, repository, intent, false); err != nil {
		return Resources{}, err
	}
	readPath := func(args ...string) (string, error) {
		out, err := git.Output(ctx, intent.Path, args...)
		if err != nil {
			return "", err
		}
		return resolveGitPath(strings.TrimSpace(string(out)))
	}
	common, err := readPath("rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil || canonical(common) != canonical(repository.CommonDir) {
		return Resources{}, errors.Join(err, errors.New("Git common directory differs from the fixed repository identity"))
	}
	resources := Resources{}
	for _, item := range []struct {
		name string
		args []string
		want string
		dest *string
	}{
		{"objects", []string{"rev-parse", "--path-format=absolute", "--git-path", "objects"}, filepath.Join(common, "objects"), &resources.Objects},
		{"refs", []string{"rev-parse", "--path-format=absolute", "--git-path", "refs"}, filepath.Join(common, "refs"), &resources.Refs},
		{"logs", []string{"rev-parse", "--path-format=absolute", "--git-path", "logs"}, filepath.Join(common, "logs"), &resources.Logs},
	} {
		path, err := readPath(item.args...)
		if err != nil {
			return Resources{}, fmt.Errorf("resolve Git %s directory: %w", item.name, err)
		}
		if canonical(path) != canonical(item.want) {
			return Resources{}, fmt.Errorf("Git %s directory is outside the fixed repository", item.name)
		}
		*item.dest = path
	}
	admin, err := readPath("rev-parse", "--absolute-git-dir")
	if err != nil {
		return Resources{}, fmt.Errorf("resolve linked-worktree admin directory: %w", err)
	}
	rel, err := filepath.Rel(filepath.Join(common, "worktrees"), admin)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.Dir(rel) != "." {
		return Resources{}, errors.New("Git admin directory is not a fixed linked worktree")
	}
	resources.Admin = admin
	return resources, nil
}

// ResolveLinkedResourcesAt resolves a verified linked worktree's identity from
// its fixed local path, then returns the same four narrowly scoped Git paths.
func ResolveLinkedResourcesAt(ctx context.Context, git Git, repository Repository, path string) (Resources, error) {
	if !filepath.IsAbs(path) {
		return Resources{}, errors.New("linked worktree path must be absolute")
	}
	if err := repository.Verify(ctx, git); err != nil {
		return Resources{}, err
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return Resources{}, err
	}
	if canonical(real) != canonical(path) {
		return Resources{}, errors.New("linked worktree path was redirected")
	}
	common, err := git.Output(ctx, path, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return Resources{}, err
	}
	resolvedCommon, err := resolveGitPath(strings.TrimSpace(string(common)))
	if err != nil || canonical(resolvedCommon) != canonical(repository.CommonDir) {
		return Resources{}, errors.Join(err, errors.New("linked worktree belongs to a different Git repository"))
	}
	branch, err := git.Output(ctx, path, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		return Resources{}, err
	}
	head, err := git.Output(ctx, path, "rev-parse", "HEAD")
	if err != nil {
		return Resources{}, err
	}
	intent := &executionbackend.WorkspaceIntent{Path: path, Branch: strings.TrimSpace(string(branch)), BaseCommit: strings.TrimSpace(string(head))}
	return ResolveLinkedResources(ctx, git, repository, intent)
}

// ResolveInfoExclude verifies the shared Git exclude file against the fixed
// repository identity before a caller uses it for its bounded relay entry.
func ResolveInfoExclude(ctx context.Context, git Git, repository Repository, worktree string) (string, error) {
	if err := repository.Verify(ctx, git); err != nil {
		return "", err
	}
	if !filepath.IsAbs(worktree) {
		return "", errors.New("worktree path must be absolute")
	}
	common, err := git.Output(ctx, worktree, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", err
	}
	resolvedCommon, err := resolveGitPath(strings.TrimSpace(string(common)))
	if err != nil || canonical(resolvedCommon) != canonical(repository.CommonDir) {
		return "", errors.Join(err, errors.New("Git common directory differs from the fixed repository identity"))
	}
	path, err := git.Output(ctx, worktree, "rev-parse", "--path-format=absolute", "--git-path", "info/exclude")
	if err != nil {
		return "", err
	}
	excludePath := strings.TrimSpace(string(path))
	if !filepath.IsAbs(excludePath) {
		return "", errors.New("Git returned a non-absolute exclude file path")
	}
	resolved, err := resolveFuturePath(excludePath)
	if err != nil {
		return "", err
	}
	if canonical(resolved) != canonical(filepath.Join(repository.CommonDir, "info", "exclude")) {
		return "", errors.New("Git exclude file is outside the fixed repository")
	}
	return resolved, nil
}
