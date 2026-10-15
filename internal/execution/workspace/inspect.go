package workspace

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
)

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

func canonical(path string) string {
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		return strings.ToLower(path)
	}
	return path
}
