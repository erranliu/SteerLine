package steerline

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
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
	c := Config{Repository: repo, Git: Command{Executable: "git"}, CommandTimeout: "10s", ExecutionPolicy: workspacePolicyFixture()}
	if err := c.ResolveIdentity(ctx); err != nil {
		t.Fatal(err)
	}
	args, err := codexLinkedWorktreeWriteArgs(ctx, c, worktree)
	if err != nil {
		t.Fatal(err)
	}
	admin := runGit("-C", worktree, "rev-parse", "--absolute-git-dir")
	objects := filepath.Join(c.Identity(), "objects")
	refs := filepath.Join(c.Identity(), "refs")
	logs := filepath.Join(c.Identity(), "logs")
	if len(args) != 8 || args[0] != "--add-dir" || canonical(args[1]) != canonical(objects) || args[2] != "--add-dir" || canonical(args[3]) != canonical(refs) || args[4] != "--add-dir" || canonical(args[5]) != canonical(logs) || args[6] != "--add-dir" || canonical(args[7]) != canonical(admin) {
		t.Fatalf("unexpected Codex Git write grants: %q", args)
	}
	if _, err := codexLinkedWorktreeWriteArgs(ctx, c, repo); err == nil {
		t.Fatal("main repository was accepted as a linked-worktree admin directory")
	}
	c.identity = filepath.Join(t.TempDir(), "foreign.git")
	if _, err := codexLinkedWorktreeWriteArgs(ctx, c, worktree); err == nil {
		t.Fatal("foreign repository identity was allowed to grant Git metadata write access")
	}
}
