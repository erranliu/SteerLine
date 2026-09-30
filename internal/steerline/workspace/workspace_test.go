package workspace

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/execution"
)

type testProcess struct{ process *os.Process }

func (p testProcess) Identity() execution.ProcessIdentity { return execution.ProcessIdentity{} }
func (p testProcess) Kill()                               { _ = p.process.Kill() }
func (p testProcess) Close()                              {}

func TestLocalWorkspacePrepareInspectResourcesAndCleanup(t *testing.T) {
	ctx := context.Background()
	git := Git{Command: execution.LaunchCommand{Executable: "git"}, Timeout: 15 * time.Second, Start: func(cmd *exec.Cmd) (execution.StopHandle, error) {
		if err := cmd.Start(); err != nil {
			return nil, err
		}
		return testProcess{cmd.Process}, nil
	}}
	root := t.TempDir()
	repositoryPath := filepath.Join(root, "repository")
	foreignPath := filepath.Join(root, "foreign")
	prepareGitFixture(t, git, repositoryPath)
	prepareGitFixture(t, git, foreignPath)
	repository, err := ResolveRepository(ctx, git, repositoryPath)
	if err != nil {
		t.Fatal(err)
	}
	baseOutput, err := git.Output(ctx, repositoryPath, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	base := strings.TrimSpace(string(baseOutput))
	worktreeRoot := filepath.Join(root, "worktrees")
	intent := &Intent{Path: filepath.Join(worktreeRoot, "run-one"), Branch: "codex/fixture", BaseCommit: base}
	prepared, err := Prepare(ctx, git, repository, worktreeRoot, intent)
	if err != nil || prepared.Status != execution.OperationConfirmed || prepared.Inspection == nil || prepared.Inspection.Head != base {
		t.Fatalf("prepare result=%+v err=%v", prepared, err)
	}
	resources, err := ResolveLinkedResourcesAt(ctx, git, repository, intent.Path)
	if err != nil {
		t.Fatal(err)
	}
	if resources.Objects != filepath.Join(repository.CommonDir, "objects") || resources.Refs != filepath.Join(repository.CommonDir, "refs") || resources.Logs != filepath.Join(repository.CommonDir, "logs") || !strings.HasPrefix(resources.Admin, filepath.Join(repository.CommonDir, "worktrees")+string(filepath.Separator)) {
		t.Fatalf("linked Git resources escaped their fixed directories: %+v", resources)
	}
	exclude, err := ResolveInfoExclude(ctx, git, repository, intent.Path)
	if err != nil || exclude != filepath.Join(repository.CommonDir, "info", "exclude") {
		t.Fatalf("resolved Git exclude=%q err=%v", exclude, err)
	}
	if _, err := ResolveLinkedResourcesAt(ctx, git, repository, repositoryPath); err == nil {
		t.Fatal("main checkout received linked-worktree admin resources")
	}
	if _, err := ResolveLinkedResourcesAt(ctx, git, repository, filepath.Join(root, "missing")); err == nil {
		t.Fatal("missing redirected checkout was accepted")
	}
	if _, err := ValidateRoot(repository, filepath.Join(repository.Path, "worktrees"), filepath.Join(root, "runtime")); err == nil {
		t.Fatal("worktree root inside the checkout was accepted")
	}
	if _, err := ValidateRoot(repository, filepath.Join(root, "runtime", "runs", "nested"), filepath.Join(root, "runtime")); err == nil {
		t.Fatal("worktree root inside runtime records was accepted")
	}
	cleanup, err := Cleanup(ctx, git, repository, worktreeRoot, intent)
	if err != nil || cleanup.Status != execution.OperationConfirmed || !cleanup.WorktreeGone || !cleanup.BranchGone {
		t.Fatalf("cleanup result=%+v err=%v", cleanup, err)
	}
	foreignHead, err := git.Output(ctx, foreignPath, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if out, err := git.Output(ctx, foreignPath, "worktree", "add", "--no-track", "-b", "foreign", "--", intent.Path, strings.TrimSpace(string(foreignHead))); err != nil {
		t.Fatalf("create foreign worktree fixture: %v: %s", err, out)
	}
	if _, err := Inspect(ctx, git, repository, &Intent{Path: intent.Path, Branch: "foreign", BaseCommit: base}); err == nil {
		t.Fatal("worktree in another repository passed the fixed repository check")
	}
	if _, err := git.Output(ctx, foreignPath, "worktree", "remove", "--force", "--", intent.Path); err != nil {
		t.Fatal(err)
	}
	if _, err := git.Output(ctx, foreignPath, "branch", "-D", "--", "foreign"); err != nil {
		t.Fatal(err)
	}

	partialIntent := &Intent{Path: filepath.Join(worktreeRoot, "run-partial"), Branch: "codex/partial", BaseCommit: strings.Repeat("a", 40)}
	partial, err := Prepare(ctx, git, repository, worktreeRoot, partialIntent)
	if err == nil || partial.Status != execution.OperationUnknown || !partial.PathCreated {
		t.Fatalf("partial preparation facts were lost: result=%+v err=%v", partial, err)
	}
}

func prepareGitFixture(t *testing.T, git Git, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main", path}, {"-C", path, "config", "user.name", "Workspace Fixture"}, {"-C", path, "config", "user.email", "workspace@example.invalid"}} {
		if output, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
	if err := os.WriteFile(filepath.Join(path, "base.txt"), []byte("base\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"-C", path, "add", "base.txt"}, {"-C", path, "commit", "-q", "-m", "base"}} {
		if output, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
}
