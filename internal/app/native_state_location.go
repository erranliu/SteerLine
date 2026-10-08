package app

import (
	"context"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/erranliu/SteerLine/internal/pathguard"
)

func ValidateNativeStateLocation(config Config) error {
	return validateNativeStateLocation(config)
}

func validateNativeStateLocation(config Config) error {
	if err := config.ResolveIdentity(context.Background()); err != nil {
		return err
	}
	key := RepositoryStateKey(config.Identity())
	expected := filepath.Join(StateRoot(), key)
	actualPath, actualErr := pathguard.ResolveProspective(config.RunDir)
	expectedPath, expectedErr := pathguard.ResolveProspective(expected)
	if actualErr != nil || expectedErr != nil || canonicalPath(actualPath) != canonicalPath(expectedPath) {
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
