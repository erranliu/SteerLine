package steerline

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fakeHostGH(args []string) int {
	file := os.Getenv("STEERLINE_TEST_HOST_GH_FILE")
	var saved PRInfo
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
	if len(args) >= 2 && args[0] == "pr" && args[1] == "create" {
		head, err := exec.Command("git", "rev-parse", "HEAD").Output()
		if err != nil {
			return 1
		}
		branch, err := exec.Command("git", "symbolic-ref", "--short", "HEAD").Output()
		if err != nil {
			return 1
		}
		saved = PRInfo{URL: "https://github.com/owner/repo/pull/1", Repository: "owner/repo", BaseBranch: "main", HeadRepository: "owner/repo", HeadBranch: strings.TrimSpace(string(branch)), HeadRevision: strings.TrimSpace(string(head)), State: "open"}
		raw, _ := json.Marshal(saved)
		if err := os.WriteFile(file, raw, 0600); err != nil {
			return 1
		}
		fmt.Println(saved.URL)
		return 0
	}
	return 2
}

func TestLocalWorkerPublishUsesHostGitAndExactPR(t *testing.T) {
	ctx := context.Background()
	c, base, db := openDAGFixture(t)
	c.Host, c.Agent = "local", "codex"
	c.CommandTimeout = "10s"
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("STEERLINE_TEST_HOST_GH_FILE", filepath.Join(t.TempDir(), "pr.json"))
	c.PR = PRConfig{Kind: "command", BaseURL: "https://github.com", Repository: "owner/repo", Query: Command{Executable: exe, Args: []string{"api", "repos/{repository}/pulls/{number}"}}}
	runID, turnID := strings.Repeat("a", 32), "00000000-0000-0000-0000-000000000001"
	branch := "codex/101-" + runID[:12]
	worktree := filepath.Join(t.TempDir(), "worker")
	git := func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
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
	run := &Run{TaskID: "101", RunID: runID, AttemptID: strings.Repeat("d", 32), ExecutionID: strings.Repeat("c", 32), WorkflowVersion: WorkerFlowVersion, Protocol: string(ProtocolLocalCodexAppServer), FinishTaskStatus: "waiting_decision", WorktreePath: &worktree, WorktreeBranch: &branch,
		WorkspaceIntent: &WorkspaceIntent{Path: worktree, Branch: branch, BaseCommit: base}}
	workflow := &WorkflowRunRecord{WorkflowRunID: runID, WorkflowVersion: WorkerFlowVersion, Branch: "main", Phase: "waiting_decision"}
	execution := &ExecutionRecord{WorkflowRunID: runID, ExecutionID: run.ExecutionID, Workspace: WorkspaceEvidence{Worktree: worktree, Branch: branch}, CurrentTurnID: turnID, LastStoppedTurnID: turnID}
	s := &Service{Config: c, domainStore: db, store: &Store{Path: filepath.Join(c.RunDir, c.RuntimeFile), Project: c.Identity()},
		prs: &PRClient{Config: c.PR, Runner: Runner{Dir: c.Repository, Timeout: 10 * time.Second}},
		State: Runtime{SchemaVersion: runtimeSchemaFor(c), Repository: Repository{Path: c.Repository, Remote: c.Remote, Branch: c.Branch, Identity: c.Identity()},
			Runs: []*Run{run}, WorkflowRuns: []WorkflowRunRecord{*workflow}, Executions: []ExecutionRecord{*execution}}}
	workflow, execution = &s.State.WorkflowRuns[0], &s.State.Executions[0]
	info, err := s.publishLocalWorkerRun(ctx, run, workflow, execution, turnID)
	if err != nil {
		t.Fatal(err)
	}
	if info.URL != "https://github.com/owner/repo/pull/1" || s.State.WorkflowRuns[0].PRURL != info.URL || s.State.WorkflowRuns[0].Phase != "waiting_pr_merge" {
		t.Fatalf("host publication did not bind exact draft PR: info=%+v workflow=%+v", info, s.State.WorkflowRuns[0])
	}
	head := git(worktree, "rev-parse", "HEAD")
	if head == base || info.HeadRevision != head || git(worktree, "status", "--porcelain") != "" {
		t.Fatalf("host commit was not clean and fixed: base=%s head=%s info=%+v", base, head, info)
	}
	if _, err := s.publishLocalWorkerRun(ctx, run, workflow, execution, turnID); err == nil {
		t.Fatal("same turn repeated its confirmed publication")
	}
	s.State.Operations[0].Status = "in_flight"
	s.State.WorkflowRuns[0].Phase, s.State.WorkflowRuns[0].PRURL = "waiting_decision", ""
	run.FinishTaskStatus, run.PR.URL = "waiting_decision", nil
	reconciled, err := s.reconcileLocalWorkerPublication(ctx, run)
	if err != nil || !reconciled || s.State.Operations[0].Status != "confirmed" || s.State.WorkflowRuns[0].PRURL != info.URL {
		t.Fatalf("uncertain host publication did not reconcile the same PR: reconciled=%t err=%v operation=%+v workflow=%+v", reconciled, err, s.State.Operations[0], s.State.WorkflowRuns[0])
	}
}

func TestLocalWorkerPublishMarkerMustBeFinalAndBound(t *testing.T) {
	runID := strings.Repeat("a", 32)
	if !localWorkerRequestedPublish("Done.\nSTEERLINE_PUBLISH_READY "+runID+"\n", runID) ||
		localWorkerRequestedPublish("STEERLINE_PUBLISH_READY "+runID+"\nmore text", runID) ||
		localWorkerRequestedPublish("STEERLINE_PUBLISH_READY "+strings.Repeat("b", 32), runID) {
		t.Fatal("local publication marker was not exact")
	}
}
