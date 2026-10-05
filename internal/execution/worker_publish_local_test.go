package execution

import (
	"context"
	"encoding/json"
	"fmt"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/execution/publication"
)

func fakeHostGH(args []string) int {
	file := os.Getenv("STEERLINE_TEST_HOST_GH_FILE")
	var saved executionmodel.PRInfo
	if raw, err := os.ReadFile(file); err == nil {
		_ = json.Unmarshal(raw, &saved)
	}
	if len(args) >= 2 && args[0] == "api" {
		if strings.Contains(args[1], "?state=all&head=") {
			if saved.URL == "" {
				fmt.Println("[]")
				return 0
			}
			_ = json.NewEncoder(os.Stdout).Encode([]any{map[string]any{
				"html_url": saved.URL, "state": "open",
				"head": map[string]any{"ref": saved.HeadBranch, "sha": saved.HeadRevision, "repo": map[string]string{"full_name": saved.Repository}},
				"base": map[string]string{"ref": saved.BaseBranch},
			}})
			return 0
		}
		_ = json.NewEncoder(os.Stdout).Encode(saved)
		return 0
	}
	if len(args) >= 2 && args[0] == "pr" && args[1] == "close" {
		saved.State = "closed_unmerged"
		raw, _ := json.Marshal(saved)
		if err := os.WriteFile(file, raw, 0600); err != nil {
			return 1
		}
		_ = json.NewEncoder(os.Stdout).Encode(saved)
		return 0
	}
	if len(args) >= 2 && args[0] == "pr" && args[1] == "create" {
		countPath := file + ".creates"
		count, _ := os.ReadFile(countPath)
		if err := os.WriteFile(countPath, []byte(strings.TrimSpace(string(count))+"x"), 0600); err != nil {
			return 1
		}
		head, err := exec.Command("git", "rev-parse", "HEAD").Output()
		if err != nil {
			return 1
		}
		branch, err := exec.Command("git", "symbolic-ref", "--short", "HEAD").Output()
		if err != nil {
			return 1
		}
		saved = executionmodel.PRInfo{URL: "https://github.com/owner/repo/pull/1", Repository: "owner/repo", BaseBranch: "main", HeadRepository: "owner/repo", HeadBranch: strings.TrimSpace(string(branch)), HeadRevision: strings.TrimSpace(string(head)), State: "open"}
		raw, _ := json.Marshal(saved)
		if err := os.WriteFile(file, raw, 0600); err != nil {
			return 1
		}
		fmt.Println(saved.URL)
		return 0
	}
	return 2
}

type localWorkerPublicationFixture struct {
	ctx      context.Context
	s        *Service
	run      *reconstructiontypes.RunView
	c        Config
	base     string
	turnID   string
	worktree string
	git      func(string, ...string) string
}

func newLocalWorkerPublicationFixture(t *testing.T) localWorkerPublicationFixture {
	t.Helper()
	ctx := context.Background()
	s, run := neutralAuditService(t)
	c := s.Config
	base := run.WorkspaceIntent.BaseCommit
	c.CommandTimeout = "10s"
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("STEERLINE_TEST_HOST_GH_FILE", filepath.Join(t.TempDir(), "pr.json"))
	c.PR = publication.Config{Kind: "command", BaseURL: "https://github.com", Repository: "owner/repo", Query: executionbackend.Command{Executable: exe, Args: []string{"api", "repos/{repository}/pulls/{number}"}}, Close: executionbackend.Command{Executable: exe, Args: []string{"pr", "close", "{pr_url}"}}}
	turnID := "00000000-0000-0000-0000-000000000001"
	branch, worktree := run.WorkspaceIntent.Branch, run.WorkspaceIntent.Path
	require(t, c.PrepareWorktreeRoot())
	git := func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		out, e := cmd.CombinedOutput()
		if e != nil {
			t.Fatalf("git %v: %v: %s", args, e, out)
		}
		return strings.TrimSpace(string(out))
	}
	git(c.Repository, "worktree", "add", "-b", branch, worktree, base)
	git(worktree, "config", "user.name", "Fixture")
	git(worktree, "config", "user.email", "fixture@example.invalid")
	git(worktree, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(worktree, "publish.txt"), []byte("ready\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git(worktree, "add", "publish.txt")
	s.Config, s.baseConfig = c, c
	workflow, execution := s.workflowRecords(run)
	workflow.Phase = "waiting_decision"
	execution.Status, execution.CurrentTurnID, execution.LastStoppedTurnID = "idle", turnID, turnID
	execution.Workspace = executionmodel.WorkspaceEvidence{Worktree: worktree, Branch: branch, Base: base}
	execution.Target.WorktreeID, execution.Target.Branch = worktree, branch
	s.refreshRun(run)
	require(t, s.commitWorker(run))
	return localWorkerPublicationFixture{ctx: ctx, s: s, run: run, c: c, base: base, turnID: turnID, worktree: worktree, git: git}
}
