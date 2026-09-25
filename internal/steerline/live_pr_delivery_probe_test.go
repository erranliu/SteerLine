package steerline

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLiveNativeTaskDeliveryProbe(t *testing.T) {
	if os.Getenv("STEERLINE_LIVE_PR_ACCEPTANCE") != "1" {
		t.Skip("set STEERLINE_LIVE_PR_ACCEPTANCE=1 to query the fixed merged GitHub PR")
	}
	const prURL = "https://github.com/erranliu/SteerLine/pull/30"
	const baseCommit = "4c658c33db63372ea767ac48d8eebd003781e750"
	const expectedMerge = "696406f006f905ddbc494f09b3915d9c1a0295fb"
	ctx := context.Background()
	t.Setenv("STEERLINE_HOME", t.TempDir())
	root, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(t.TempDir(), "repo")
	git := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("git", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("clone", "--quiet", "--no-hardlinks", strings.TrimSpace(string(root)), repo)
	git("-C", repo, "checkout", "--quiet", "-B", "main", baseCommit)
	git("-C", repo, "remote", "set-url", "origin", "https://github.com/erranliu/SteerLine.git")
	const jq = `{url: .html_url, repository: .base.repo.full_name, base_branch: .base.ref, head_revision: .head.sha, state: (if .merged then "merged" elif .state == "closed" then "closed_unmerged" else .state end), merged: .merged, merged_at: .merged_at, merge_commit: .merge_commit_sha}`
	prConfig := PRConfig{Kind: "command", BaseURL: "https://github.com", Repository: "erranliu/SteerLine", Query: Command{Executable: "gh", Args: []string{"api", "repos/erranliu/SteerLine/pulls/{number}", "--jq", jq}}}
	verifier := &PRClient{Config: prConfig, Runner: Runner{Dir: repo, Timeout: 30 * time.Second}}
	live, err := verifier.Query(ctx, prURL)
	if err != nil || live.State != "merged" || !live.Merged || live.MergeCommit == nil || *live.MergeCommit != expectedMerge || live.HeadRevision == "" {
		t.Fatalf("live PR evidence differs from the fixed identity: %+v / %v", live, err)
	}
	worktree := filepath.Join(t.TempDir(), "worker")
	git("-C", repo, "worktree", "add", "--quiet", "--detach", worktree, live.HeadRevision)
	if got := git("-C", worktree, "rev-parse", "HEAD"); got != live.HeadRevision {
		t.Fatalf("worker Git HEAD is %s, expected %s", got, live.HeadRevision)
	}
	git("-C", worktree, "merge-base", "--is-ancestor", baseCommit, live.HeadRevision)
	if dirty := git("-C", worktree, "status", "--porcelain"); dirty != "" {
		t.Fatalf("worker Git checkout is dirty: %s", dirty)
	}
	check := exec.Command("go", "test", "./cmd/steerline", "-count=1", "-timeout", "3m")
	check.Dir = worktree
	if out, err := check.CombinedOutput(); err != nil {
		t.Fatalf("head revision verification failed: %v: %s", err, out)
	} else {
		t.Logf("head revision verification: %s", strings.TrimSpace(string(out)))
	}
	c := Config{TaskAuthority: "native", Repository: repo, Remote: "origin", Branch: "main", Git: Command{Executable: "git"}, CommandTimeout: "30s", PR: prConfig}
	key, err := repoStateKey(c)
	if err != nil {
		t.Fatal(err)
	}
	c.RunDir = filepath.Join(SteerlineStateRoot(), key)
	opened, err := OpenNativeTaskProject(ctx, &c)
	if err != nil {
		t.Fatal(err)
	}
	if err = opened.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := OpenSQLiteStore(filepath.Join(c.RunDir, "steerline.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	commands := CommandService{Store: store, Project: c.Identity()}
	batch := taskBatchFixture(t, c, baseCommit, batchTasks(300, 301), []TaskBatchEdge{{Predecessor: TaskBatchReference{LocalKey: "k300"}, Successor: TaskBatchReference{LocalKey: "k301"}}})
	if _, err = commands.CreateTaskBatch(ctx, c, batch, strings.Repeat("a", 32), 1, "live-probe"); err != nil {
		t.Fatal(err)
	}
	page, err := (QueryService{Store: store, Project: c.Identity()}).ShowNativeTask(ctx, "300")
	if err != nil {
		t.Fatal(err)
	}
	task, contract := page.Tasks[0].TaskRecord, page.Tasks[0].Contract
	step := deliveryStepContract()
	steps := ProjectSteps{Implement: step, Verify: step, PublishPR: step}
	steps.Verify.RequiredChecks = []string{"unit"}
	definition := DefaultMinimalWorkflowDefinition(false)
	verifiedAt := time.Now().UTC()
	workspace := WorkspaceEvidence{Repository: repo, Worktree: worktree, Branch: "codex/roadmap-after-pr29", Base: baseCommit, Head: live.HeadRevision, Dirty: false, VerifiedAt: verifiedAt}
	wfr := WorkflowRunRecord{ProjectIdentity: c.Identity(), WorkflowRunID: "live-wf-300", TaskID: "300", WorkflowVersion: MinimalWorkflowVersion, Definition: definition, ProjectSteps: steps, Phase: "waiting_pr_merge", Repository: repo, Branch: "main", SelectedResult: "live-exec-300", CreatedAt: verifiedAt, UpdatedAt: verifiedAt}
	execution := ExecutionRecord{ProjectIdentity: c.Identity(), ExecutionID: "live-exec-300", WorkflowRunID: wfr.WorkflowRunID, Status: "succeeded", CurrentStep: "waiting_pr_merge", CandidateRevision: live.HeadRevision, VerifiedRevision: live.HeadRevision, Workspace: workspace, CreatedAt: verifiedAt, UpdatedAt: verifiedAt}
	verifyResult, _ := json.Marshal(StepGateResult{StepID: "verify", Succeeded: true, Revision: live.HeadRevision, ResultID: "live-verify-300", Checks: []CheckEvidence{{Name: "unit", Status: "passed", Revision: live.HeadRevision}}, Workspace: workspace})
	publishResult, _ := json.Marshal(StepGateResult{StepID: "publish_pr", Succeeded: true, Revision: live.HeadRevision, PublishedHead: live.HeadRevision, PRURL: prURL, ResultID: "live-publish-300", Workspace: workspace})
	verifyAttempt := StepAttemptRecord{ProjectIdentity: c.Identity(), AttemptID: "live-verify-attempt-300", ExecutionID: execution.ExecutionID, StepID: "verify", AttemptNumber: 1, Status: "succeeded", ContractDigest: contract.Digest, ResultID: "live-verify-300", Result: verifyResult, CreatedAt: verifiedAt, UpdatedAt: verifiedAt}
	publishAttempt := StepAttemptRecord{ProjectIdentity: c.Identity(), AttemptID: "live-publish-attempt-300", ExecutionID: execution.ExecutionID, StepID: "publish_pr", AttemptNumber: 1, Status: "succeeded", ContractDigest: contract.Digest, ResultID: "live-publish-300", Result: publishResult, CreatedAt: verifiedAt, UpdatedAt: verifiedAt}
	task.Status, task.Claimant, task.Version, task.UpdatedAt = "in_progress", "live-probe", 2, verifiedAt
	if err = store.Transact(ctx, func(tx DomainTx) error {
		if e := tx.PutWorkflowRun(wfr); e != nil {
			return e
		}
		if e := tx.PutExecution(execution); e != nil {
			return e
		}
		if e := tx.PutStepAttempt(verifyAttempt); e != nil {
			return e
		}
		if e := tx.PutStepAttempt(publishAttempt); e != nil {
			return e
		}
		return tx.UpdateNativeTaskCAS(task, 1)
	}); err != nil {
		t.Fatal(err)
	}
	receipt, err := commands.CompleteNativeTask(ctx, c, "300", 2, wfr.WorkflowRunID, verifier)
	if err != nil || receipt.Status != "closed" || receipt.Delivery.MergeCommit != expectedMerge || receipt.Delivery.PRURL != prURL {
		t.Fatalf("live native delivery did not commit exact PR evidence: %+v / %v", receipt, err)
	}
	page, err = (QueryService{Store: store, Project: c.Identity()}).ShowNativeTask(ctx, "301")
	if err != nil || len(page.Tasks) != 1 || !page.Tasks[0].DependencyReady {
		t.Fatalf("live merged delivery did not release successor: %+v / %v", page, err)
	}
	var evidence TaskDeliveryEvidence
	if err = json.Unmarshal(receipt.Delivery.Evidence, &evidence); err != nil || !evidence.Merged || evidence.MergeCommit != expectedMerge || evidence.URL != prURL {
		t.Fatalf("persisted live merge evidence differs: %+v / %v", evidence, err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenSQLiteStore(filepath.Join(c.RunDir, "steerline.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	page, err = (QueryService{Store: reopened, Project: c.Identity()}).ShowNativeTask(ctx, "301")
	if err != nil || len(page.Tasks) != 1 || !page.Tasks[0].DependencyReady {
		t.Fatalf("reopened live delivery did not keep successor ready: %+v / %v", page, err)
	}
	commands.Store = reopened
	replayed, err := commands.CompleteNativeTask(ctx, c, "300", 2, wfr.WorkflowRunID, verifier)
	if err != nil || replayed.GraphRevision != receipt.GraphRevision || replayed.Delivery.MergeCommit != expectedMerge {
		t.Fatalf("reopened live delivery did not replay idempotently: %+v / %v", replayed, err)
	}
	t.Logf("LIVE DELIVERY PASS: PR=%s head=%s merge=%s task=300 successor=301 graph_revision=%d", prURL, live.HeadRevision, expectedMerge, receipt.GraphRevision)
}
