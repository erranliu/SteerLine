package steerline

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/erranliu/SteerLine/internal/steerline/publication"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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

type localWorkerPublicationFixture struct {
	ctx      context.Context
	s        *Service
	run      *Run
	c        Config
	base     string
	turnID   string
	worktree string
	git      func(string, ...string) string
}

func newLocalWorkerPublicationFixture(t *testing.T) localWorkerPublicationFixture {
	t.Helper()
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
	c.PR = publication.Config{Kind: "command", BaseURL: "https://github.com", Repository: "owner/repo", Query: Command{Executable: exe, Args: []string{"api", "repos/{repository}/pulls/{number}"}}, Close: Command{Executable: exe, Args: []string{"pr", "close", "{pr_url}"}}}
	turnID := "00000000-0000-0000-0000-000000000001"
	branch, worktree := run.WorkspaceIntent.Branch, run.WorkspaceIntent.Path
	require(t, c.prepareWorktreeRoot())
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
	execution.Workspace = WorkspaceEvidence{Worktree: worktree, Branch: branch, Base: base}
	execution.Target.WorktreeID, execution.Target.Branch = worktree, branch
	s.refreshRun(run)
	require(t, s.commitWorker(run))
	return localWorkerPublicationFixture{ctx: ctx, s: s, run: run, c: c, base: base, turnID: turnID, worktree: worktree, git: git}
}

func TestLocalPublicationIntentMustCommitBeforeHostAction(t *testing.T) {
	f := newLocalWorkerPublicationFixture(t)
	t.Cleanup(func() { _ = f.s.Close() })
	workflow, execution := f.s.workflowRecords(f.run)
	_, err := f.s.domainStore.db.ExecContext(f.ctx, `CREATE TRIGGER reject_publication_intent BEFORE UPDATE ON projection BEGIN SELECT RAISE(ABORT, 'publication intent checkpoint failure'); END`)
	require(t, err)
	if _, err = f.s.publishLocalWorkerRun(f.ctx, f.run, workflow, execution, f.turnID); err == nil || !strings.Contains(err.Error(), "publication intent checkpoint failure") {
		t.Fatalf("publication intent fault was not injected: %v", err)
	}
	if got := f.git(f.worktree, "rev-parse", "HEAD"); got != f.base || !strings.Contains(f.git(f.worktree, "status", "--porcelain"), "publish.txt") {
		t.Fatalf("failed publication intent changed Git candidate: head=%s base=%s", got, f.base)
	}
	if _, err = os.Stat(os.Getenv("STEERLINE_TEST_HOST_GH_FILE") + ".creates"); !os.IsNotExist(err) {
		t.Fatalf("GitHub create ran before durable intent: stat error=%v", err)
	}
}

func TestLocalWorkerPublishUsesHostGitAndExactPR(t *testing.T) {
	f := newLocalWorkerPublicationFixture(t)
	ctx, s, run, c, base, turnID, worktree, git := f.ctx, f.s, f.run, f.c, f.base, f.turnID, f.worktree, f.git
	workflow, execution := s.workflowRecords(run)
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
	// Missing external evidence must leave the original request uncertain, even
	// though the commit and pushed branch can already be verified.
	prFile := os.Getenv("STEERLINE_TEST_HOST_GH_FILE")
	savedPR, err := os.ReadFile(prFile)
	require(t, err)
	require(t, os.Remove(prFile))
	if reconciled, err := s.reconcileLocalWorkerPublication(ctx, run); err == nil || reconciled || s.operationByKey(originalOperation.IdempotencyKey).Status != "in_flight" {
		t.Fatalf("missing PR evidence was not kept uncertain: reconciled=%t err=%v", reconciled, err)
	}
	createsBefore, err := os.ReadFile(prFile + ".creates")
	if err != nil || string(createsBefore) != "x" || git(worktree, "rev-parse", "HEAD") != head {
		t.Fatalf("unknown publication retried host actions: count=%q err=%v", createsBefore, err)
	}
	require(t, os.WriteFile(prFile, savedPR, 0600))
	reconciled, err := s.reconcileLocalWorkerPublication(ctx, run)
	if err != nil || !reconciled || s.operationByKey(originalOperation.IdempotencyKey).Status != "confirmed" || s.domain.WorkflowRuns[0].PRURL != info.URL {
		t.Fatalf("uncertain host publication did not reconcile the same PR: reconciled=%t err=%v operation=%+v workflow=%+v", reconciled, err, s.operationByKey(originalOperation.IdempotencyKey), s.domain.WorkflowRuns[0])
	}
	creates, err := os.ReadFile(os.Getenv("STEERLINE_TEST_HOST_GH_FILE") + ".creates")
	if err != nil || string(creates) != "x" {
		t.Fatalf("uncertain publication created another PR: count=%q err=%v", creates, err)
	}
}

func TestLocalWorkerPublishMarkerMustBeFinalAndBound(t *testing.T) {
	runID := strings.Repeat("a", 32)
	if !publication.RequestedByWorker("Done.\nSTEERLINE_PUBLISH_READY "+runID+"\n", runID) ||
		publication.RequestedByWorker("STEERLINE_PUBLISH_READY "+runID+"\nmore text", runID) ||
		publication.RequestedByWorker("STEERLINE_PUBLISH_READY "+strings.Repeat("b", 32), runID) {
		t.Fatal("local publication marker was not exact")
	}
}
