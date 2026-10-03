package codexworker_test

import (
	"context"
	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/processhost"
	"github.com/erranliu/SteerLine/internal/steerline/workspace"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestCodexLinkedWorktreeWriteArgsOnlyGrantsFixedGitPaths(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows workspace-write Git metadata gate")
	}
	ctx := context.Background()
	repo := filepath.Join(t.TempDir(), "repo")
	worktree := filepath.Join(t.TempDir(), "linked")
	runGit := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("git", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	runGit("init", "-q", repo)
	if err := os.WriteFile(filepath.Join(repo, "base.txt"), []byte("base"), 0600); err != nil {
		t.Fatal(err)
	}
	runGit("-C", repo, "add", "base.txt")
	runGit("-C", repo, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "commit", "-qm", "base")
	runGit("-C", repo, "worktree", "add", "-qb", "validation", worktree)
	git := workspace.Git{Command: executionapi.LaunchCommand{Executable: "git"}, Timeout: 10 * time.Second, Start: processhost.New(time.Second).StartManaged}
	c, err := workspace.ResolveRepository(ctx, git, repo)
	require(t, err)
	args, err := linkedWriteArgs(ctx, git, c, worktree)
	if err != nil {
		t.Fatal(err)
	}
	admin := runGit("-C", worktree, "rev-parse", "--absolute-git-dir")
	objects := filepath.Join(c.CommonDir, "objects")
	refs := filepath.Join(c.CommonDir, "refs")
	logs := filepath.Join(c.CommonDir, "logs")
	if len(args) != 8 || args[0] != "--add-dir" || canonicalPath(args[1]) != canonicalPath(objects) || args[2] != "--add-dir" || canonicalPath(args[3]) != canonicalPath(refs) || args[4] != "--add-dir" || canonicalPath(args[5]) != canonicalPath(logs) || args[6] != "--add-dir" || canonicalPath(args[7]) != canonicalPath(admin) {
		t.Fatalf("unexpected Codex Git write grants: %q", args)
	}
	if _, err := linkedWriteArgs(ctx, git, c, repo); err == nil {
		t.Fatal("main repository was accepted as a linked-worktree admin directory")
	}
	c.CommonDir = filepath.Join(t.TempDir(), "foreign.git")
	if _, err := linkedWriteArgs(ctx, git, c, worktree); err == nil {
		t.Fatal("foreign repository identity was allowed to grant Git metadata write access")
	}
}
func linkedWriteArgs(ctx context.Context, git workspace.Git, repo workspace.Repository, path string) ([]string, error) {
	r, err := workspace.ResolveLinkedResourcesAt(ctx, git, repo, path)
	if err != nil {
		return nil, err
	}
	return codexworker.LinkedWorktreeWriteArgs(r.Objects, r.Refs, r.Logs, r.Admin)
}
func canonicalPath(path string) string {
	value, err := filepath.Abs(path)
	if err == nil {
		path = value
	}
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		path = strings.ToLower(path)
	}
	return path
}
