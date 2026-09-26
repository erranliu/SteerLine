package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestFilterGeneratedRunOutputPreservesOtherValidationChanges(t *testing.T) {
	repoRoot := t.TempDir()
	outDir := filepath.Join(repoRoot, "docs", "validation", "test-feedback", "run with spaces")
	generated := generatedRunOutputPaths(repoRoot, outDir)
	status := strings.Join([]string{
		"?? docs/validation/test-feedback/run with spaces/go-test.jsonl",
		"?? docs/validation/test-feedback/run with spaces/go-test.stderr.txt",
		"?? docs/validation/test-feedback/run with spaces/summary.json",
		"?? docs/validation/test-feedback/README.md",
		" M docs/validation/test-feedback/coverage-map.md",
		" M internal/steerline/service_test.go",
		"",
	}, "\x00")

	got := filterGeneratedRunOutput([]byte(status), generated)
	want := strings.Join([]string{
		"?? docs/validation/test-feedback/README.md",
		" M docs/validation/test-feedback/coverage-map.md",
		" M internal/steerline/service_test.go",
	}, "\n")
	if got != want {
		t.Fatalf("filtered worktree status = %q, want %q", got, want)
	}
}

func TestGeneratedRunOutputPathsOutsideRepositoryExcludeNothing(t *testing.T) {
	repoRoot := filepath.Join(t.TempDir(), "repo")
	outDir := filepath.Join(t.TempDir(), "reports")
	if paths := generatedRunOutputPaths(repoRoot, outDir); len(paths) != 0 {
		t.Fatalf("outside output paths were excluded from source status: %v", paths)
	}
}
