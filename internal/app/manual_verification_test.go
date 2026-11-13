package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/execution/backend"
	"github.com/erranliu/SteerLine/internal/execution/model"
	mv "github.com/erranliu/SteerLine/internal/manualverification"
	"github.com/erranliu/SteerLine/internal/storage/ports"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
)

func TestManualVerificationRunsPinnedPRHead(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	remote := filepath.Join(root, "remote.git")
	git := func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = dir
		body, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v %s", args, err, body)
		}
		return strings.TrimSpace(string(body))
	}
	git(root, "init", repo)
	git(root, "init", "--bare", remote)
	if err := os.WriteFile(filepath.Join(repo, "marker.txt"), []byte("preview head\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git(repo, "add", "--", "marker.txt")
	git(repo, "-c", "user.name=Manual Verification", "-c", "user.email=manual@example.invalid", "-c", "commit.gpgsign=false", "commit", "-m", "Preview head")
	head := git(repo, "rev-parse", "HEAD")
	git(repo, "push", remote, "HEAD:refs/pull/1/head")
	state := filepath.Join(root, "state")
	if err := os.Mkdir(state, 0700); err != nil {
		t.Fatal(err)
	}
	config := Config{Repository: repo, RunDir: state, Git: backend.Command{Executable: "git", Args: []string{"-c", "url." + filepath.ToSlash(remote) + ".insteadOf=https://github.com/example/preview.git"}}, ProcessHost: NewProcessHost(15 * time.Second)}
	if err := config.ResolveIdentity(ctx); err != nil {
		t.Fatal(err)
	}
	store, err := sqlite.OpenSQLiteStore(filepath.Join(state, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	// The worktree checks are shared by launch, result submission, and removal.
	service := newManualVerification(config, store)
	defer service.close()
	a := mv.Attempt{ID: strings.Repeat("a", 32), Head: head, Source: mv.SourcePR, Worktree: service.worktree(strings.Repeat("a", 32))}
	target := ports.RunnerPRTarget{Provider: model.PRProviderConfig{Kind: "command", BaseURL: "https://github.com", Repository: "example/preview"}, Number: 1, Base: "release"}
	const remoteURL = "https://github.com/example/preview.git"
	if err := service.checkout(ctx, a, target, remoteURL, false); err != nil {
		t.Fatal(err)
	}
	if err := service.verifyWorktree(ctx, a, true); err != nil {
		t.Fatal(err)
	}
	if branch := git(a.Worktree, "rev-parse", "--abbrev-ref", "HEAD"); branch != "HEAD" {
		t.Fatalf("worktree not detached: %s", branch)
	}
	if err := os.WriteFile(filepath.Join(a.Worktree, "marker.txt"), []byte("dirty"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := service.verifyWorktree(ctx, a, true); err == nil {
		t.Fatal("dirty worktree accepted")
	}
	other := a
	other.ID = strings.Repeat("b", 32)
	other.Worktree = service.worktree(other.ID)
	other.Head = strings.Repeat("a", len(head))
	if err := service.checkout(ctx, other, target, remoteURL, false); err == nil {
		t.Fatal("unexpected PR Head accepted")
	}

	git(repo, "-c", "user.name=Manual Verification", "-c", "user.email=manual@example.invalid", "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-m", "Target branch moved")
	latest := git(repo, "rev-parse", "HEAD")
	git(repo, "push", remote, "HEAD:refs/heads/release", "HEAD:refs/pull/1/head")
	other.Head = head
	if err := service.checkout(ctx, other, target, remoteURL, true); err != nil {
		t.Fatalf("merged PR position: %v", err)
	}
	resolved, err := service.remoteVerificationHead(ctx, target, remoteURL)
	if err != nil || resolved != latest {
		t.Fatalf("remote Head = %q, error = %v", resolved, err)
	}
	current := mv.Attempt{ID: strings.Repeat("c", 32), Head: resolved, Source: mv.SourceRemote, Worktree: service.worktree(strings.Repeat("c", 32))}
	if err := service.checkout(ctx, current, target, remoteURL, true); err != nil {
		t.Fatalf("remote Head: %v", err)
	}
	git(repo, "push", "--force", remote, head+":refs/heads/release")
	current.ID = strings.Repeat("d", 32)
	current.Worktree = service.worktree(current.ID)
	if err := service.checkout(ctx, current, target, remoteURL, true); err == nil {
		t.Fatal("moved remote Head accepted")
	}
}
