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
	s, run := appServerAuditService(t)
	c := s.Config
	base := run.WorkspaceIntent.BaseCommit
	c.CommandTimeout = "10s"
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("STEERLINE_TEST_HOST_GH_FILE", filepath.Join(t.TempDir(), "pr.json"))
	c.PR = PRConfig{Kind: "command", BaseURL: "https://github.com", Repository: "owner/repo", Query: Command{Executable: exe, Args: []string{"api", "repos/{repository}/pulls/{number}"}}}
	turnID := "00000000-0000-0000-0000-000000000001"
	branch := run.WorkspaceIntent.Branch
	worktree := run.WorkspaceIntent.Path
	require(t, c.prepareWorktreeRoot())
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
	s.Config, s.baseConfig = c, c
	s.prs = &PRClient{Config: c.PR, Runner: Runner{Dir: c.Repository, Timeout: 10 * time.Second}}
	workflow, execution := s.workflowRecords(run)
	workflow.Phase = "waiting_decision"
	execution.Status, execution.CurrentTurnID, execution.LastStoppedTurnID = "idle", turnID, turnID
	execution.Workspace = WorkspaceEvidence{Worktree: worktree, Branch: branch, Base: base}
	execution.Target.WorktreeID, execution.Target.Branch = worktree, branch
	s.refreshRun(run)
	require(t, s.commitWorker(run))
	beforeWorkflow := *workflow
	info, err := s.publishLocalWorkerRun(ctx, run, workflow, execution, turnID)
	if err != nil {
		t.Fatal(err)
	}
	if info.URL != "https://github.com/owner/repo/pull/1" || s.domain.WorkflowRuns[0].PRURL != info.URL || s.domain.WorkflowRuns[0].Phase != "waiting_pr_merge" {
		t.Fatalf("host publication did not bind exact draft PR: info=%+v workflow=%+v", info, s.domain.WorkflowRuns[0])
	}
	head := git(worktree, "rev-parse", "HEAD")
	if head == base || info.HeadRevision != head || git(worktree, "status", "--porcelain") != "" {
		t.Fatalf("host commit was not clean and fixed: base=%s head=%s info=%+v", base, head, info)
	}
	if _, err := s.publishLocalWorkerRun(ctx, run, workflow, execution, turnID); err == nil {
		t.Fatal("same turn repeated its confirmed publication")
	}
	// Simulate a host action completed before its database receipt was saved.
	originalOperation := *s.operationByKey("local-publish:" + run.RunID + ":" + turnID)
	originalOperation.Status, originalOperation.Receipt = "in_flight", nil
	require(t, s.domainStore.Transact(ctx, func(tx DomainTx) error {
		raw := tx.(*sqliteTx).tx
		if err := tx.UpdateOperation(originalOperation); err != nil {
			return err
		}

		_, err := raw.ExecContext(ctx, `UPDATE workflow_runs SET phase=?,payload=? WHERE project_identity=? AND workflow_run_id=?`, beforeWorkflow.Phase, mustJSON(beforeWorkflow), c.Identity(), run.RunID)
		return err
	}))
	require(t, s.refreshNativeSnapshot(ctx))
	if s.operationByKey(originalOperation.IdempotencyKey).Status != "in_flight" {
		t.Fatalf("crash fixture did not restore intent: %+v", s.domain.Operations)
	}
	reconciled, err := s.reconcileLocalWorkerPublication(ctx, run)
	if err != nil || !reconciled || s.operationByKey(originalOperation.IdempotencyKey).Status != "confirmed" || s.domain.WorkflowRuns[0].PRURL != info.URL {
		t.Fatalf("uncertain host publication did not reconcile the same PR: reconciled=%t err=%v operation=%+v workflow=%+v", reconciled, err, s.operationByKey(originalOperation.IdempotencyKey), s.domain.WorkflowRuns[0])
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
