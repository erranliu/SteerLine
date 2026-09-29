package steerline

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestWorkerFlowDeliveryCommitsWithoutStepGatesAndUnlocksSuccessor(t *testing.T) {
	ctx := context.Background()
	c, commit, store := openDAGFixture(t)
	commands := CommandService{Store: store, Project: c.Identity()}
	input := taskBatchFixture(t, c, commit, batchTasks(8, 9), []TaskBatchEdge{{Predecessor: TaskBatchReference{LocalKey: "k8"}, Successor: TaskBatchReference{LocalKey: "k9"}}})
	validation, err := commands.ValidateTaskBatch(ctx, c, input)
	if err != nil || !validation.Valid {
		t.Fatalf("validate fixture: %+v %v", validation, err)
	}
	if _, err = commands.CreateTaskBatch(ctx, c, input, strings.Repeat("a", 32), validation.AuthorityVersion, "operator"); err != nil {
		t.Fatal(err)
	}
	graph, err := store.taskGraphSnapshot(ctx, c.Identity())
	if err != nil {
		t.Fatal(err)
	}
	task := graph.ByID["8"]
	contract, err := nativeContractForTask(ctx, store, c.Identity(), task)
	if err != nil {
		t.Fatal(err)
	}
	created := time.Now().UTC()
	url := "https://git.example/o/r/pulls/8"
	wf := WorkflowRunRecord{ProjectIdentity: c.Identity(), WorkflowRunID: "worker-flow-wf-8", TaskID: task.TaskID, WorkflowVersion: WorkerFlowVersion, Phase: "waiting_pr_merge", Repository: c.Repository, Branch: "main", SelectedResult: "worker-flow-exec-8", PRURL: url, PRState: "open", CreatedAt: created, UpdatedAt: created}
	execution := ExecutionRecord{ProjectIdentity: c.Identity(), ExecutionID: wf.SelectedResult, WorkflowRunID: wf.WorkflowRunID, Agent: "codex", Status: "idle", SessionID: "session-8", Workspace: WorkspaceEvidence{Branch: "codex/task-8", Dirty: true}, CreatedAt: created, UpdatedAt: created}
	claimed := task
	claimed.Status = "in_progress"
	claimed.Claimant = "operator"
	claimed.Version++
	claimed.UpdatedAt = created
	if err = store.Transact(ctx, func(tx DomainTx) error {
		if e := tx.(*sqliteTx).UpdateNativeTaskCAS(claimed, task.Version); e != nil {
			return e
		}
		if e := tx.PutWorkflowRun(wf); e != nil {
			return e
		}
		return tx.PutExecution(execution)
	}); err != nil {
		t.Fatal(err)
	}
	mergedAt := time.Now().UTC()
	proof := TaskDeliveryEvidence{PRInstance: "https://git.example", URL: url, Repository: "o/r", PRNumber: 8, BaseBranch: "main", State: "merged", Merged: true, MergedAt: mergedAt, MergeCommit: commit}
	evidence, err := json.Marshal(proof)
	if err != nil {
		t.Fatal(err)
	}
	delivery := TaskDeliveryRecord{ProjectIdentity: c.Identity(), TaskID: "8", SelectedWorkflowRunID: wf.WorkflowRunID, PRInstance: proof.PRInstance, PRRepository: proof.Repository, PRNumber: 8, PRURL: url, MergeCommit: commit, MergedAt: mergedAt, RecordedAt: created, Evidence: evidence}
	receipt, err := commands.commitNativeDelivery(ctx, store, graph.Authority.AuthorityVersion, claimed, wf, execution, nil, contract, StepGateResult{PRURL: url}, delivery)
	if err != nil || receipt.Status != "closed" {
		t.Fatalf("commit without step results: receipt=%+v error=%v", receipt, err)
	}
	page, err := (QueryService{Store: store, Project: c.Identity()}).ShowNativeTask(ctx, "8")
	if err != nil || len(page.Tasks) != 1 || page.Tasks[0].Status != "closed" || page.Tasks[0].Delivery == nil {
		t.Fatalf("missing committed delivery: page=%+v error=%v", page, err)
	}
	snapshot, err := store.Query(ctx, c.Identity())
	if err != nil {
		t.Fatal(err)
	}
	if latest := latestNativeWorkflow(snapshot.WorkflowRuns, "8"); latest == nil || latest.Phase != "completed" {
		t.Fatalf("workflow did not complete with delivery: %+v", latest)
	}
	ready, err := (QueryService{Store: store, Project: c.Identity()}).ReadyNativeTasks(ctx)
	if err != nil || len(ready) != 1 || ready[0].TaskID != "9" {
		t.Fatalf("successor was not released: ready=%+v error=%v", ready, err)
	}
}

func TestWorkerFlowClaimNeedsNoProjectSteps(t *testing.T) {
	ctx := context.Background()
	c, commit, store := openDAGFixture(t)
	c.WorkflowProtocol = WorkerFlowVersion
	if out, err := exec.Command("git", "-C", c.Repository, "push", "--quiet", "origin", "HEAD:refs/heads/main").CombinedOutput(); err != nil {
		t.Fatalf("push fixture branch: %v: %s", err, out)
	}
	commands := CommandService{Store: store, Project: c.Identity()}
	input := taskBatchFixture(t, c, commit, batchTasks(8), nil)
	validation, err := commands.ValidateTaskBatch(ctx, c, input)
	if err != nil || !validation.Valid {
		t.Fatalf("validate fixture: %+v %v", validation, err)
	}
	if _, err = commands.CreateTaskBatch(ctx, c, input, strings.Repeat("b", 32), validation.AuthorityVersion, "operator"); err != nil {
		t.Fatal(err)
	}
	graph, err := store.taskGraphSnapshot(ctx, c.Identity())
	if err != nil {
		t.Fatal(err)
	}
	authority := graph.Authority
	authority.AuthorityVersion++
	authority.DispatchPaused = false
	if err := store.Transact(ctx, func(tx DomainTx) error {
		return tx.(*sqliteTx).UpdateProjectTaskAuthorityCAS(authority, authority.AuthorityVersion-1)
	}); err != nil {
		t.Fatal(err)
	}
	receipt, err := commands.ClaimNativeTask(ctx, c, TaskClaimOptions{TaskID: "8", RequestID: strings.Repeat("c", 32), ExpectedTaskVersion: graph.ByID["8"].Version, ExpectedAuthorityVersion: authority.AuthorityVersion, RequestedBy: "operator", Explicit: true})
	if err != nil || receipt.Status != "claim_reserved" {
		t.Fatalf("claim without project steps: receipt=%+v error=%v", receipt, err)
	}
	snapshot, err := store.Query(ctx, c.Identity())
	if err != nil {
		t.Fatal(err)
	}
	wf := latestNativeWorkflow(snapshot.WorkflowRuns, "8")
	if wf == nil || wf.WorkflowVersion != WorkerFlowVersion || wf.Phase != "queued" || wf.SelectedResult != receipt.ExecutionID || len(snapshot.Attempts) != 1 || snapshot.Attempts[0].StepID != "worker" {
		t.Fatalf("claim did not reserve one worker identity: workflow=%+v attempts=%+v", wf, snapshot.Attempts)
	}
}
