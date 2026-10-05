package app

import (
	"context"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/erranliu/SteerLine/internal/execution"
)

func ValidateNativeStateLocation(config execution.Config) error {
	return validateNativeStateLocation(config)
}

func validateNativeStateLocation(config execution.Config) error {
	if config.RepositoryStateKey == "" || config.StateRoot == "" {
		return taskError("authority_mismatch", "application configuration lacks its fixed repository state location")
	}
	if err := config.ResolveIdentity(context.Background()); err != nil {
		return err
	}
	if config.RepositoryStateKey != RepositoryStateKey(config.Identity()) {
		return taskError("authority_mismatch", "repository state key differs from the resolved repository identity")
	}
	expected := filepath.Join(config.StateRoot, config.RepositoryStateKey)
	if canonicalPath(config.RunDir) != canonicalPath(expected) {
		return taskError("authority_mismatch", "native tasks require the repository's stable shared state directory; remove the custom run_dir setting")
	}
	return nil
}

// RepositoryStateKey derives the stable application state folder from an
// explicit Git common directory. It performs no I/O outside resolving symlinks.
func RepositoryStateKey(identity string) string {
	common := identity
	if resolved, err := filepath.EvalSymlinks(common); err == nil {
		common = resolved
	}
	name := filepath.Base(common)
	if strings.EqualFold(name, ".git") {
		name = filepath.Base(filepath.Dir(common))
	}
	name = strings.Trim(regexp.MustCompile(`[^a-zA-Z0-9._-]+`).ReplaceAllString(name, "-"), "-.")
	if name == "" {
		name = "repo"
	}
	return name + "-" + shortHash(canonicalPath(common))[:12]
}
