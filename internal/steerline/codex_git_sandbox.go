package steerline

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
)

// Windows workspace-write needs both Git's shared object store and this linked
// worktree's own admin directory to stage and commit from a Codex step.
func codexLinkedWorktreeWriteArgs(ctx context.Context, c Config, worktree string) ([]string, error) {
	if runtime.GOOS != "windows" || c.Sandbox != "workspace-write" {
		return nil, nil
	}
	if c.identity == "" {
		return nil, errors.New("Codex Git sandbox requires a resolved repository identity")
	}
	r := gitRunner(c, worktree)
	readPath := func(args ...string) (string, error) {
		out, err := r.Output(ctx, c.Git, args...)
		if err != nil {
			return "", err
		}
		path := strings.TrimSpace(string(out))
		if !filepath.IsAbs(path) {
			return "", fmt.Errorf("Git returned a non-absolute metadata path %q", path)
		}
		return filepath.EvalSymlinks(path)
	}
	common, err := readPath("rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return nil, fmt.Errorf("resolve Codex Git common directory: %w", err)
	}
	if canonical(common) != canonical(c.identity) {
		return nil, errors.New("Codex Git common directory differs from the fixed repository identity")
	}
	objects, err := readPath("rev-parse", "--path-format=absolute", "--git-path", "objects")
	if err != nil {
		return nil, fmt.Errorf("resolve Codex Git object directory: %w", err)
	}
	if canonical(objects) != canonical(filepath.Join(common, "objects")) {
		return nil, errors.New("Codex Git objects are outside the fixed repository")
	}
	admin, err := readPath("rev-parse", "--absolute-git-dir")
	if err != nil {
		return nil, fmt.Errorf("resolve Codex linked-worktree admin directory: %w", err)
	}
	rel, err := filepath.Rel(filepath.Join(common, "worktrees"), admin)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.Dir(rel) != "." {
		return nil, errors.New("Codex Git admin directory is not a fixed linked worktree")
	}
	return []string{"--add-dir", objects, "--add-dir", admin}, nil
}
