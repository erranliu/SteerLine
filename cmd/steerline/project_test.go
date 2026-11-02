package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestRoadmapProjectPreparationCommand(t *testing.T) {
	if runProjectCommand([]string{"prepare-roadmap", "-repository", "relative"}) == nil || runProjectCommand([]string{"unknown"}) == nil {
		t.Fatal("invalid project preparation command accepted")
	}
	root := t.TempDir()
	if output, err := exec.Command("git", "init", "-b", "main", root).CombinedOutput(); err != nil {
		t.Fatal(err, string(output))
	}
	if runProjectCommand([]string{"prepare-roadmap", "-repository", root, "-check"}) == nil {
		t.Fatal("missing files passed a read-only check")
	}
	if _, err := os.Stat(filepath.Join(root, "SteerlineFlow")); !os.IsNotExist(err) {
		t.Fatal("check wrote project material")
	}
	if err := runProjectCommand([]string{"prepare-roadmap", "-repository", root}); err != nil {
		t.Fatal(err)
	}
	if err := runProjectCommand([]string{"prepare-roadmap", "-repository", root, "-check"}); err != nil {
		t.Fatal(err)
	}
}
