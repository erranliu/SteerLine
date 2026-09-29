package steerline

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkerFlowServiceFirstFeedbackKeepsSessionAndWorkspace(t *testing.T) {
	t.Setenv("STEERLINE_WORKER_FLOW_MOCK", "1")
	ctx := context.Background()
	c, commit, store := openDAGFixture(t)
	c.WorkflowProtocol, c.Host, c.Agent, c.Sandbox = WorkerFlowVersion, "local", "codex", "workspace-write"
	c.Claimant = "operator"
	c.Codex = Command{Executable: os.Args[0], Args: []string{"-test.run=^TestWorkerFlowMockCodexHelper$", "--"}}
	c.WorktreeRoot = filepath.Join(c.RunDir, "worktrees")
	if err := c.prepareWorktreeRoot(); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", c.Repository, "push", "--quiet", "origin", "HEAD:refs/heads/main").CombinedOutput(); err != nil {
		t.Fatalf("push isolated fixture: %v: %s", err, out)
	}
	commands := CommandService{Store: store, Project: c.Identity()}
	input := taskBatchFixture(t, c, commit, batchTasks(8), nil)
	validation, err := commands.ValidateTaskBatch(ctx, c, input)
	if err != nil || !validation.Valid {
		t.Fatalf("validate isolated task: %+v %v", validation, err)
	}
	if _, err = commands.CreateTaskBatch(ctx, c, input, strings.Repeat("d", 32), validation.AuthorityVersion, "operator"); err != nil {
		t.Fatal(err)
	}
	authority, err := store.nativeAuthority(ctx, c.Identity())
	if err != nil {
		t.Fatal(err)
	}
	authority.AuthorityVersion++
	authority.DispatchPaused = false
	if err := store.Transact(ctx, func(tx DomainTx) error {
		return tx.(*sqliteTx).UpdateProjectTaskAuthorityCAS(authority, authority.AuthorityVersion-1)
	}); err != nil {
		t.Fatal(err)
	}
	receipt, err := commands.ClaimNativeTask(ctx, c, TaskClaimOptions{TaskID: "8", RequestID: strings.Repeat("e", 32), ExpectedTaskVersion: 1, ExpectedAuthorityVersion: authority.AuthorityVersion, RequestedBy: "operator", Explicit: true})
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewService(c)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = service.Close() }()
	run := service.find("8")
	if run == nil || run.RunID != receipt.WorkflowRunID || run.ExecutionID != receipt.ExecutionID || run.AttemptID != receipt.AttemptID {
		t.Fatalf("fixed claim did not reconstruct: run=%+v receipt=%+v", run, receipt)
	}
	task := nativeTaskRecord(service.State.Tasks, "8")
	if task == nil {
		t.Fatal("claimed task disappeared")
	}
	if err = service.prepareWorkerFlow(ctx, *task, run); err != nil {
		t.Fatal(err)
	}
	if service.workers[run.RunID] == nil {
		if os.Getenv("STEERLINE_REQUIRE_DETACHED_TESTS") != "1" && run.Error != nil && strings.Contains(*run.Error, errDetachUnsupported.Error()) {
			t.Skipf("host cannot retain a worker across service exit: %s", *run.Error)
		}
		t.Fatalf("worker was not launched: reason=%q run=%+v", deref(run.Error), run)
	}
	firstWorktree := deref(run.WorktreePath)
	if firstWorktree == "" {
		t.Fatal("worker preparation lost its worktree")
	}
	finishTurn := func() {
		t.Helper()
		worker := service.workers[run.RunID]
		if worker == nil {
			t.Fatal("original worker observer is missing")
		}
		if err := service.selectWorker(run); err != nil {
			t.Fatal(err)
		}
		for event := range worker.Events {
			if err := service.event(event); err != nil {
				t.Fatal(err)
			}
		}
		workerErr := <-worker.Done
		delete(service.workers, run.RunID)
		if err := service.finished(workerErr); err != nil {
			t.Fatal(err)
		}
		service.parkWorker()
		if run.FinishTaskStatus != "waiting_decision" || run.Decision == nil || run.Decision.Status != "pending" {
			t.Fatalf("stopped worker did not reach the unified decision: %+v", run)
		}
	}
	finishTurn()
	firstSession := deref(run.SessionID)
	if err := service.Close(); err != nil {
		t.Fatal(err)
	}
	service, err = NewService(c)
	if err != nil {
		t.Fatal(err)
	}
	run = service.find("8")
	if run == nil || run.RunID != receipt.WorkflowRunID || run.ExecutionID != receipt.ExecutionID || deref(run.SessionID) != firstSession || run.Decision == nil || run.Decision.Status != "pending" {
		t.Fatalf("current-format restart lost the waiting worker: run=%+v", run)
	}
	partial := filepath.Join(firstWorktree, "partial.txt")
	if err := os.WriteFile(partial, []byte("partially committed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"-C", firstWorktree, "add", "partial.txt"}, {"-C", firstWorktree, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "partial"}} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("record partial commit: %v: %s", err, out)
		}
	}
	head, err := exec.Command("git", "-C", firstWorktree, "rev-parse", "HEAD").Output()
	if err != nil || strings.TrimSpace(string(head)) == receipt.BaseCommit {
		t.Fatal("partial commit did not advance HEAD")
	}
	service.State.Service.Ready = true
	service.State.Service.Status = "running"
	if err := service.save(); err != nil {
		t.Fatal(err)
	}
	answer := DecisionAnswer{DecisionID: run.Decision.ID, RunID: run.RunID, Note: "Continue in the same worktree", AnsweredBy: "operator", ExpectedDecisionVersion: run.Decision.Version, ExpectedRunRevision: run.ControlRevision, ServiceInstanceID: service.State.Service.InstanceID}
	payload, err := json.Marshal(answer)
	if err != nil {
		t.Fatal(err)
	}
	command, _, err := (CommandService{Store: service.domainStore, Project: c.Identity()}).AnswerDecisionWithID(ctx, strings.Repeat("1", 32), answer.DecisionID, run.Decision.Version, decisionAnswerKey(answer), answer.AnsweredBy, payload)
	if err != nil {
		t.Fatal(err)
	}
	record, _, err := (CommandService{Store: service.domainStore, Project: c.Identity()}).AcceptAnswer(ctx, command, answer)
	if err != nil {
		t.Fatal(err)
	}
	var saved DecisionAnswer
	if err := json.Unmarshal(record.Answer, &saved); err != nil {
		t.Fatal(err)
	}
	run.Decision.Version, run.Decision.Status = record.Version, record.Status
	run.Decision.OptionID, run.Decision.Note, run.Decision.AnsweredBy = saved.OptionID, saved.Note, saved.AnsweredBy
	run.Decision.AnsweredAt = &saved.AnsweredAt
	if err := service.save(); err != nil {
		t.Fatal(err)
	}
	if err := service.applyWorkerFlowDecision(ctx, run); err != nil {
		t.Fatal(err)
	}
	finishTurn()
	workflow, execution := service.minimalRecords(run)
	if workflow == nil || execution == nil || workflow.WorkflowRunID != receipt.WorkflowRunID || execution.ExecutionID != receipt.ExecutionID ||
		run.AttemptID != receipt.AttemptID || execution.SessionID != workerFlowMockSession || execution.TurnNumber != 2 ||
		deref(run.WorktreePath) != firstWorktree || len(service.State.Attempts) != 1 {
		t.Fatalf("feedback replaced the fixed worker: workflow=%+v execution=%+v run=%+v", workflow, execution, run)
	}
	currentHead, err := exec.Command("git", "-C", firstWorktree, "rev-parse", "HEAD").Output()
	if err != nil || strings.TrimSpace(string(currentHead)) != strings.TrimSpace(string(head)) {
		t.Fatal("feedback reset the partially committed HEAD")
	}
	// The next answer has an accepted command, but its send receipt is lost.
	// Reconciliation must preserve the original request without launching it again.
	uncertain := DecisionAnswer{DecisionID: run.Decision.ID, RunID: run.RunID, Note: "Do not send twice", AnsweredBy: "operator", ExpectedDecisionVersion: run.Decision.Version, ExpectedRunRevision: run.ControlRevision, ServiceInstanceID: service.State.Service.InstanceID}
	uncertainPayload, err := json.Marshal(uncertain)
	if err != nil {
		t.Fatal(err)
	}
	uncertainCommand, _, err := (CommandService{Store: service.domainStore, Project: c.Identity()}).AnswerDecisionWithID(ctx, strings.Repeat("2", 32), uncertain.DecisionID, run.Decision.Version, decisionAnswerKey(uncertain), uncertain.AnsweredBy, uncertainPayload)
	if err != nil {
		t.Fatal(err)
	}
	uncertainRecord, _, err := (CommandService{Store: service.domainStore, Project: c.Identity()}).AcceptAnswer(ctx, uncertainCommand, uncertain)
	if err != nil {
		t.Fatal(err)
	}
	run.Decision.Version, run.Decision.Status = uncertainRecord.Version, uncertainRecord.Status
	run.Decision.Note, run.Decision.AnsweredBy = uncertain.Note, uncertain.AnsweredBy
	run.Decision.AnsweredAt = &uncertainRecord.UpdatedAt
	if err := service.save(); err != nil {
		t.Fatal(err)
	}
	if _, err := service.startOperation("start_worker_turn", "worker-turn:"+execution.ExecutionID+":"+uncertainCommand.CommandID, map[string]string{"original_request": uncertainCommand.CommandID}); err != nil {
		t.Fatal(err)
	}
	before := len(service.State.Operations)
	for i := 0; i < 2; i++ {
		if err := service.applyWorkerFlowDecision(ctx, run); err != nil {
			t.Fatal(err)
		}
	}
	if len(service.State.Operations) != before || service.workers[run.RunID] != nil || execution.TurnNumber != 2 || run.Error == nil || !strings.Contains(*run.Error, "no duplicate message") {
		t.Fatalf("uncertain feedback was resent or lost: operations=%d/%d execution=%+v run=%+v", before, len(service.State.Operations), execution, run)
	}
}
