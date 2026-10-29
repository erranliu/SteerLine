package app

import (
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/testmanagement"
	"os/exec"
	"strings"
	"testing"
)

// Uses actual pinned Git objects; no inline input or mutable checkout shortcut.
func appRepositoryManifest(t *testing.T, repo runnercontract.GenerateRepository) *runnercontract.RepositorySource {
	t.Helper()
	source := &runnercontract.RepositorySource{LocalPath: repo.LocalPath, Commit: repo.Commit, Tree: pmGit(t, repo.LocalPath, "rev-parse", repo.Commit+"^{tree}")}
	paths := repo.Paths
	if paths == nil {
		paths = strings.Split(pmGit(t, repo.LocalPath, "ls-tree", "-r", "--name-only", repo.Commit), "\n")
	}
	for _, path := range paths {
		raw, e := exec.Command("git", "-C", repo.LocalPath, "show", repo.Commit+":"+path).Output()
		require(t, e)
		source.Files = append(source.Files, runnercontract.RepositoryFile{Path: path, Blob: pmGit(t, repo.LocalPath, "rev-parse", repo.Commit+":"+path), SHA256: testmanagement.ContentDigest(raw), SizeBytes: len(raw)})
	}
	return source
}
