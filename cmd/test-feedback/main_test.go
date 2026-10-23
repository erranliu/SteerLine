package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestFilterGeneratedRunOutputPreservesOtherValidationChanges(t *testing.T) {
	repoRoot := t.TempDir()
	outDir := filepath.Join(repoRoot, "SteerlineFlow", "testing", "validation", "test-feedback", "run with spaces")
	generated := generatedRunOutputPaths(repoRoot, outDir)
	status := strings.Join([]string{
		"?? SteerlineFlow/testing/validation/test-feedback/run with spaces/go-test.jsonl",
		"?? SteerlineFlow/testing/validation/test-feedback/run with spaces/go-test.stderr.txt",
		"?? SteerlineFlow/testing/validation/test-feedback/run with spaces/summary.json",
		"?? SteerlineFlow/testing/validation/test-feedback/README.md",
		" M SteerlineFlow/testing/validation/test-feedback/coverage-map.md",
		" M internal/app/host/service_test.go",
		"",
	}, "\x00")

	got := filterGeneratedRunOutput([]byte(status), generated)
	want := strings.Join([]string{
		"?? SteerlineFlow/testing/validation/test-feedback/README.md",
		" M SteerlineFlow/testing/validation/test-feedback/coverage-map.md",
		" M internal/app/host/service_test.go",
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
