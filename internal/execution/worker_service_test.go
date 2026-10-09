package execution

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/execution/scheduler"
)

func TestWorkerFlowServiceFirstFeedbackKeepsSessionAndWorkspace(t *testing.T) {
	for _, test := range []struct {
		name, option, note string
	}{
		{"free_text", "", "Continue in the same worktree"},
		{"option_with_note", "continue", "Keep the existing commit and continue"},
		{"open_pr_feedback", "continue", "Add one more change before merge"},
	} {
		t.Run(test.name, func(t *testing.T) {
			testWorkerFlowServiceFirstFeedback(t, test.option, test.note, test.name == "open_pr_feedback")
		})
	}
}

func testWorkerFlowServiceFirstFeedback(t *testing.T, option, note string, openPR bool) {
	ctx := context.Background()
	c, commit, store := openDAGFixture(t)
	c.WorkflowProtocol, c.Host, c.Agent = executionmodel.Version, "local", "fixture"
	c.WorktreeRoot = filepath.Join(c.RunDir, "worktrees")
	if err := c.PrepareWorktreeRoot(); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", c.Repository, "push", "--quiet", "origin", "HEAD:refs/heads/main").CombinedOutput(); err != nil {
		t.Fatalf("push isolated fixture: %v: %s", err, out)
	}
	commands := newTestCommandService(store, c)
	input := taskBatchFixture(t, c, commit, batchTasks(8), nil)
	validation, err := commands.ValidateTaskBatch(ctx, c, input)
	if err != nil || !validation.Valid {
		t.Fatalf("validate isolated task: %+v %v", validation, err)
	}
	if _, err = commands.CreateTaskBatch(ctx, c, input, strings.Repeat("d", 32), validation.AuthorityVersion, "operator"); err != nil {
		t.Fatal(err)
	}
	authority, err := store.NativeAuthority(ctx, c.Identity())
	if err != nil {
		t.Fatal(err)
	}
	authority.AuthorityVersion++
	authority.DispatchPaused = false
	if err := store.transactRecords(ctx, func(tx *fixtureTransaction) error {
		return tx.UpdateProjectTaskAuthorityCAS(authority, authority.AuthorityVersion-1)
	}); err != nil {
		t.Fatal(err)
	}
	receipt, err := commands.ClaimNativeTask(ctx, c, TaskClaimOptions{TaskID: "8", RequestID: strings.Repeat("e", 32), ExpectedTaskVersion: 1, ExpectedAuthorityVersion: authority.AuthorityVersion, RequestedBy: "operator", Explicit: true})
	if err != nil {
		t.Fatal(err)
	}
	service, err := newTestService(c)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = service.Close() }()
	run := service.find("8")
	if run == nil || run.RunID != receipt.WorkflowRunID || run.ExecutionID != receipt.ExecutionID || run.AttemptID != receipt.AttemptID {
		t.Fatalf("fixed claim did not reconstruct: run=%+v receipt=%+v", run, receipt)
	}
	task := nativeTaskRecord(service.domain.Tasks, "8")
	if task == nil {
		t.Fatal("claimed task disappeared")
	}
	preparationConfig := service.Config
	if openPR {
		service.Config.Host = "other-host" // Initial preparation uses the run's fixed host.
	}
	err = service.prepareWorkerFlow(ctx, *task, run)
	service.Config = preparationConfig
	if err != nil {
		if os.Getenv("STEERLINE_REQUIRE_DETACHED_TESTS") != "1" && strings.Contains(err.Error(), scheduler.ErrDetachUnsupported.Error()) {
			t.Skipf("host cannot retain a worker across service exit: %v", err)
		}
		t.Fatal(err)
	}
	if service.workers[run.RunID] == nil {
		if os.Getenv("STEERLINE_REQUIRE_DETACHED_TESTS") != "1" && run.Error != nil && strings.Contains(*run.Error, scheduler.ErrDetachUnsupported.Error()) {
			t.Skipf("host cannot retain a worker across service exit: %s", *run.Error)
		}
		t.Fatalf("worker was not launched: reason=%q run=%+v", deref(run.Error), run)
	}
	firstWorktree := deref(run.WorktreePath)
	if firstWorktree == "" {
		t.Fatal("worker preparation lost its worktree")
	}
	finishTurn := func(expected string) {
		t.Helper()
		worker := service.workers[run.RunID]
		if worker == nil {
			_, execution := service.workflowRecords(run)
			t.Fatalf("original worker observer is missing: task=%s execution_status=%s pending_command=%s reason=%q", run.TaskID, execution.Status, execution.PendingCommandID, deref(run.Error))
		}
		for event := range worker.Events {
			if err := service.eventForRun(run, event); err != nil {
				t.Fatal(err)
			}
		}
		workerErr := <-worker.Done
		delete(service.workers, run.RunID)
		if err := service.finishedForRun(run, worker, workerErr); err != nil {
			t.Fatal(err)
		}
		if run.FinishTaskStatus != expected || run.Decision == nil || run.Decision.Status != "pending" {
			t.Fatalf("stopped worker did not reach the unified decision: %+v", run)
		}
		if expected == "waiting_pr_merge" && (run.Decision.Request.Question != workerPRFeedbackQuestion || len(run.Decision.Request.Options) != 0) {
			t.Fatal("a completed PR turn required a default continuation", run.Decision.Request)
		}
	}
	finishTurn("waiting_decision")
	initialExecution := service.executionRecord(run)
	initialPrompt, err := os.ReadFile(filepath.Join(c.RunDir, "runs", run.RunID, "turns", initialExecution.CurrentCommandID, "prompt.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for _, identity := range []string{"Fixed assigned worktree: " + firstWorktree, "Fixed assigned branch: " + initialExecution.Workspace.Branch, "Fixed base commit: " + receipt.BaseCommit} {
		if !strings.Contains(string(initialPrompt), identity) {
			t.Fatalf("initial dispatched prompt lacks %q: %s", identity, initialPrompt)
		}
	}
	if run.Process != nil {
		pid := run.Process.PID
		t.Cleanup(func() {
			if process, err := os.FindProcess(pid); err == nil {
				_ = process.Kill()
			}
		})
	}
	if openPR {
		workflow, execution := service.workflowRecords(run)
		url := "https://git.example/org/repo/pulls/8"
		info := executionmodel.PRInfo{URL: url, Repository: "org/repo", BaseBranch: "main", HeadRepository: "org/repo", HeadBranch: *run.WorktreeBranch, HeadRevision: receipt.BaseCommit, State: "open"}
		if err := BindWorkerPR(workflow, execution, info, url, "org/repo", false); err != nil {
			t.Fatal(err)
		}
		run.FinishTaskStatus, run.PR.URL, run.PR.State = "waiting_pr_merge", ptr(url), "open"
		if err := service.commitWorker(run); err != nil {
			t.Fatal(err)
		}
	}
	if len(run.Decision.Request.Options) != 1 || run.Decision.Request.Options[0].ID != "continue" || !run.Decision.Request.AllowFreeText {
		t.Fatalf("unified decision cannot accept both feedback forms: %+v", run.Decision.Request)
	}
	firstSession := service.executionRecord(run).SessionID
	if err := service.Close(); err != nil {
		t.Fatal(err)
	}
	service, err = newTestService(c)
	if err != nil {
		t.Fatal(err)
	}
	run = service.find("8")
	if run == nil || run.RunID != receipt.WorkflowRunID || run.ExecutionID != receipt.ExecutionID || service.executionRecord(run).SessionID != firstSession || run.Decision == nil || run.Decision.Status != "pending" {
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
	if err := service.publishCheckpoint(); err != nil {
		t.Fatal(err)
	}
	answer := executionmodel.DecisionAnswer{DecisionID: run.Decision.ID, RunID: run.RunID, OptionID: option, Note: note, AnsweredBy: "operator", ExpectedDecisionVersion: run.Decision.Version, ExpectedRunRevision: run.ControlRevision, ServiceInstanceID: service.State.Service.InstanceID}
	payload, err := json.Marshal(answer)
	if err != nil {
		t.Fatal(err)
	}
	command, _, err := (newTestCommandService(service.commandStore, c)).AnswerDecisionWithID(ctx, strings.Repeat("1", 32), answer.DecisionID, run.Decision.Version, DecisionAnswerKey(answer), answer.AnsweredBy, payload)
	if err != nil {
		t.Fatal(err)
	}
	record, _, err := (newTestCommandService(service.commandStore, c)).AcceptAnswer(ctx, command, answer)
	if err != nil {
		t.Fatal(err)
	}
	var saved executionmodel.DecisionAnswer
	if err := json.Unmarshal(record.Answer, &saved); err != nil {
		t.Fatal(err)
	}
	run.Decision.Version, run.Decision.Status = record.Version, record.Status
	run.Decision.OptionID, run.Decision.Note, run.Decision.AnsweredBy = saved.OptionID, saved.Note, saved.AnsweredBy
	run.Decision.AnsweredAt = &saved.AnsweredAt
	if err := service.publishCheckpoint(); err != nil {
		t.Fatal(err)
	}
	selectedConfig := service.Config
	if openPR {
		service.Config.Host = "other-host" // A different dispatch selection cannot reroute this local run.
	}
	err = service.applyWorkerFlowDecision(ctx, run)
	service.Config = selectedConfig
	if err != nil {
		t.Fatal(err)
	}
	feedbackPrompt := workerFeedbackText(answer, run.Decision.Request)
	fixedConfig, err := service.executionConfigForRun(run)
	require(t, err)
	_, currentExecution := service.workflowRecords(run)
	workspaceInstruction, err := workerWorkspaceInstruction(currentExecution.Workspace)
	require(t, err)
	fullPrompt := feedbackPrompt + workspaceInstruction + publicationTargetInstruction(fixedConfig)
	feedbackDir := filepath.Join(c.RunDir, "runs", run.RunID, "turns", command.CommandID)
	savedPrompt, err := os.ReadFile(filepath.Join(feedbackDir, "prompt.txt"))
	if err != nil || string(savedPrompt) != fullPrompt {
		t.Fatalf("full feedback text was not sent: prompt=%q err=%v", savedPrompt, err)
	}
	if openPR {
		finishTurn("waiting_pr_merge")
	} else {
		finishTurn("waiting_decision")
	}
	events, err := os.ReadFile(filepath.Join(feedbackDir, "fixture-input.json"))
	if err != nil {
		t.Fatal(err)
	}
	encodedFeedback, _ := json.Marshal(fullPrompt)
	if !strings.Contains(string(events), string(encodedFeedback)) {
		t.Fatalf("worker did not receive the full feedback: %q", events)
	}
	workflow, execution := service.workflowRecords(run)
	if workflow == nil || execution == nil || workflow.WorkflowRunID != receipt.WorkflowRunID || execution.ExecutionID != receipt.ExecutionID ||
		run.AttemptID != receipt.AttemptID || execution.SessionID != workerFlowMockSession || execution.TurnNumber != 2 ||
		canonical(deref(run.WorktreePath)) != canonical(firstWorktree) || len(service.domain.Attempts) != 1 {
		t.Fatalf("feedback replaced the fixed worker: workflow=%q/%q execution=%q/%q session=%q turn=%d attempt=%q/%q worktree=%q/%q attempts=%d", workflow.WorkflowRunID, receipt.WorkflowRunID, execution.ExecutionID, receipt.ExecutionID, execution.SessionID, execution.TurnNumber, run.AttemptID, receipt.AttemptID, deref(run.WorktreePath), firstWorktree, len(service.domain.Attempts))
	}
	currentHead, err := exec.Command("git", "-C", firstWorktree, "rev-parse", "HEAD").Output()
	if err != nil || strings.TrimSpace(string(currentHead)) != strings.TrimSpace(string(head)) {
		t.Fatal("feedback reset the partially committed HEAD")
	}
	// The next answer has an accepted command, but its send receipt is lost.
	// Reconciliation must preserve the original request without launching it again.
	uncertain := executionmodel.DecisionAnswer{DecisionID: run.Decision.ID, RunID: run.RunID, Note: "Do not send twice", AnsweredBy: "operator", ExpectedDecisionVersion: run.Decision.Version, ExpectedRunRevision: run.ControlRevision, ServiceInstanceID: service.State.Service.InstanceID}
	uncertainPayload, err := json.Marshal(uncertain)
	if err != nil {
		t.Fatal(err)
	}
	uncertainCommand, _, err := (newTestCommandService(service.commandStore, c)).AnswerDecisionWithID(ctx, strings.Repeat("2", 32), uncertain.DecisionID, run.Decision.Version, DecisionAnswerKey(uncertain), uncertain.AnsweredBy, uncertainPayload)
	if err != nil {
		t.Fatal(err)
	}
	uncertainRecord, _, err := (newTestCommandService(service.commandStore, c)).AcceptAnswer(ctx, uncertainCommand, uncertain)
	if err != nil {
		t.Fatal(err)
	}
	run.Decision.Version, run.Decision.Status = uncertainRecord.Version, uncertainRecord.Status
	run.Decision.Note, run.Decision.AnsweredBy = uncertain.Note, uncertain.AnsweredBy
	run.Decision.AnsweredAt = &uncertainRecord.UpdatedAt
	if err := service.publishCheckpoint(); err != nil {
		t.Fatal(err)
	}
	operationID, err := service.startWorkerOperation(run, "start_worker_turn", "worker-turn:"+execution.ExecutionID+":"+uncertainCommand.CommandID, map[string]string{"original_request": uncertainCommand.CommandID})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.changeWorkerOperation(run, operationID, "outcome_unknown", "", nil, "feedback receipt lost"); err != nil {
		t.Fatal(err)
	}
	before := len(service.domain.Operations)
	for i := 0; i < 2; i++ {
		if err := service.applyWorkerFlowDecision(ctx, run); err != nil {
			t.Fatal(err)
		}
	}
	if len(service.domain.Operations) != before || service.workers[run.RunID] != nil || execution.TurnNumber != 2 || run.Error == nil || !strings.Contains(*run.Error, "no duplicate message") {
		t.Fatalf("uncertain feedback was resent or lost: operations=%d/%d worker=%t turn=%d decision=%s error=%q", before, len(service.domain.Operations), service.workers[run.RunID] != nil, execution.TurnNumber, run.Decision.Status, deref(run.Error))
	}
}

func TestServiceRecoveryGateRequiresTransactionalOwner(t *testing.T) {
	service, run := waitingDecisionService(t)
	execution := service.executionRecord(run)
	execution.OwnerServiceInstanceID = service.State.Service.InstanceID
	service.adoptOrGateServiceExecutions()
	if execution.RecoveryRequired {
		t.Fatal("current service claim was gated")
	}
	execution.OwnerServiceInstanceID = "" // Crash after claim, before the next service write.
	service.adoptOrGateServiceExecutions()
	if !execution.RecoveryRequired || execution.RecoveryVersion != 1 || execution.OwnerServiceInstanceID != service.State.Service.InstanceID {
		t.Fatal("unowned persisted claim was started without a recovery decision")
	}
}

func TestRestartDoesNotSendPreviouslyAcceptedFeedback(t *testing.T) {
	service, run := waitingDecisionService(t)
	defer service.Close()
	answer := executionmodel.DecisionAnswer{DecisionID: run.Decision.ID, RunID: run.RunID, OptionID: "continue", Note: "saved before stop", AnsweredBy: "operator", ExpectedDecisionVersion: run.Decision.Version, ExpectedRunRevision: run.ControlRevision, ServiceInstanceID: service.State.Service.InstanceID}
	commands := newTestCommandService(service.commandStore, service.Config)
	command, _, err := commands.AnswerDecisionWithID(context.Background(), newID(), answer.DecisionID, answer.ExpectedDecisionVersion, DecisionAnswerKey(answer), answer.AnsweredBy, mustJSON(answer))
	if err != nil {
		t.Fatal(err)
	}
	record, _, err := commands.AcceptAnswer(context.Background(), command, answer)
	if err != nil {
		t.Fatal(err)
	}
	*service.decisionRecord(run) = record
	require(t, service.refreshNativeSnapshot(context.Background()))
	service.refreshRun(run)
	if err := service.commitWorker(run); err != nil {
		t.Fatal(err)
	}
	service.markServiceRecoveryGate()
	if err := service.commitWorker(run); err != nil {
		t.Fatal(err)
	}
	before := len(service.domain.Operations)
	if err := service.applyDecisions(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(service.domain.Operations) != before || !service.executionRecord(run).RecoveryRequired || run.Decision.Status != "answer_saved" {
		t.Fatal("restart sent the prior-service answer without fresh authorization")
	}
	workflow := service.workflowRecord(run)
	workflow.Status = "waiting_external"
	service.refreshRun(run)
	if err := service.applyDecisions(context.Background()); err != nil || workflow.Status != "waiting_external" {
		t.Fatalf("prior-service answer advanced PR feedback phase: %v, phase=%s", err, workflow.Status)
	}
}
