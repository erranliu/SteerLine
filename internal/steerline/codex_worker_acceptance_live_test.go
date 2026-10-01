package steerline

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

// Historical detached-process acceptance. The current local worker lifecycle
// is verified by TestLocalCodexManualRecoveryLive.
func TestAppServerServiceParallelRecoveryLive(t *testing.T) {
	if os.Getenv("STEERLINE_APP_SERVER_SERVICE_LIVE_LEGACY") != "1" {
		t.Skip("historical detached-process acceptance")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	c, commit, store := openDAGFixture(t)
	c.Host, c.Agent, c.Claimant = "local", "codex", "operator"
	setTestImplementationProfiles(t, &c, codexworker.ProfileConfig{Executable: "codex", Model: "gpt-6-luna", ReasoningEffort: "low"}, nil)
	c.CommandTimeout = "45s"
	c.WorktreeRoot = filepath.Join(c.RunDir, "worktrees")
	if err := c.prepareWorktreeRoot(); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", c.Repository, "push", "--quiet", "origin", "HEAD:refs/heads/main").CombinedOutput(); err != nil {
		t.Fatalf("push isolated fixture: %v: %s", err, out)
	}
	commands := CommandService{Store: store, Project: c.Identity()}
	if _, err := commands.CreateTaskBatch(ctx, c, taskBatchFixture(t, c, commit, batchTasks(8, 9), nil), newID(), 1, "operator"); err != nil {
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
	for _, taskID := range []string{"8", "9"} {
		graph, err := store.taskGraphSnapshot(ctx, c.Identity())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := commands.ClaimNativeTask(ctx, c, TaskClaimOptions{TaskID: taskID, RequestID: newID(), ExpectedTaskVersion: graph.ByID[taskID].Version, ExpectedAuthorityVersion: graph.Authority.AuthorityVersion, RequestedBy: "operator", Explicit: true}); err != nil {
			t.Fatal(err)
		}
	}
	service, err := NewService(c)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = service.Close() }()
	run8, run9 := service.find("8"), service.find("9")
	if run8 == nil || run9 == nil || run8.RunID == run9.RunID {
		t.Fatal("two independent workflow runs were not claimed")
	}
	for _, run := range []*Run{run8, run9} {
		if err := prepareWorktreeOnly(ctx, c, run); err != nil {
			t.Fatal(err)
		}
		run.WorktreePath, run.WorktreeBranch, run.BaseCommit = ptr(run.WorkspaceIntent.Path), ptr(run.WorkspaceIntent.Branch), ptr(run.WorkspaceIntent.BaseCommit)
		workspace, err := service.inspectWorkflowWorkspace(ctx, run)
		if err != nil {
			t.Fatal(err)
		}
		_, execution := service.workflowRecords(run)
		applyWorkflowWorkspaceEvidence(run, execution, workspace)
		if err := service.commitWorker(run); err != nil {
			t.Fatal(err)
		}
	}
	processes := make([]executionapi.ProcessIdentity, 0, 2)
	t.Cleanup(func() {
		for _, worker := range service.workers {
			_ = worker.detachLocalObservation(context.Background())
		}
		for _, runID := range []string{run8.RunID, run9.RunID} {
			path := filepath.Join(c.RunDir, "runs", runID, "app-server", "process.json")
			var receipt codexworker.ProcessReceipt
			if data, err := os.ReadFile(path); err == nil && json.Unmarshal(data, &receipt) == nil {
				processes = append(processes, receipt.Identity)
			}
		}
		for _, identity := range processes {
			if inspectProcess(&identity) == "matching process still exists" {
				if process, err := os.FindProcess(identity.PID); err == nil {
					_ = process.Kill()
				}
			}
		}
	})
	processEvent := func(run *Run, event WorkerEvent) {
		t.Helper()
		if err := service.eventForRun(run, event); err != nil {
			t.Fatal(err)
		}
	}
	finish := func(run *Run) {
		t.Helper()
		worker := service.workers[run.RunID]
		if worker == nil {
			t.Fatalf("worker observer missing for task %s: %s", run.TaskID, deref(run.Error))
		}
		for {
			select {
			case <-ctx.Done():
				t.Fatal("native turn did not complete before timeout")
			case event, ok := <-worker.Events:
				if ok {
					processEvent(run, event)
					continue
				}
				if err := <-worker.Done; err != nil {
					t.Fatal(err)
				}
				delete(service.workers, run.RunID)
				if err := service.finishWorkerFlowFor(run, nil); err != nil {
					t.Fatal(err)
				}
				return
			}
		}
	}
	answer := func(run *Run, values map[string][]string, note string) {
		t.Helper()
		decision := run.Decision
		if decision == nil || decision.Status != "pending" {
			t.Fatalf("task %s has no pending decision", run.TaskID)
		}
		input := DecisionAnswer{DecisionID: decision.ID, RunID: run.RunID, Answers: values, Note: note, AnsweredBy: "operator", ExpectedDecisionVersion: decision.Version, ExpectedRunRevision: run.ControlRevision, ServiceInstanceID: service.State.Service.InstanceID}
		payload, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		command, _, err := (CommandService{Store: service.domainStore, Project: c.Identity()}).AnswerDecisionWithID(ctx, newID(), input.DecisionID, decision.Version, decisionAnswerKey(input), input.AnsweredBy, payload)
		if err != nil {
			t.Fatal(err)
		}
		record, _, err := (CommandService{Store: service.domainStore, Project: c.Identity()}).AcceptAnswer(ctx, command, input)
		if err != nil {
			t.Fatal(err)
		}
		decision.Version, decision.Status = record.Version, record.Status
		decision.Answers, decision.Note, decision.AnsweredBy = input.Answers, input.Note, input.AnsweredBy
		decision.AnsweredAt = &record.UpdatedAt
		if err := service.publishCheckpoint(); err != nil {
			t.Fatal(err)
		}
		if err := service.applyWorkerFlowDecision(ctx, run); err != nil {
			t.Fatal(err)
		}
	}
	if err := service.startWorkerFlowTurn(ctx, run8, "Call steerline_ask with one question (id choice, header Choice, question Should I reply A or B?, options A and B, allow_free_text false). Wait for the answer. Then reply with the chosen letter only.", newID()); err != nil {
		t.Fatal(err)
	}
	if worker := service.workers[run8.RunID]; worker == nil {
		t.Fatalf("first real Codex worker missing: %s", deref(run8.Error))
	} else {
		for run8.Decision == nil || run8.Decision.Request.QuestionRoute == nil {
			select {
			case <-ctx.Done():
				t.Fatal("first task did not ask through SteerLine")
			case event, ok := <-worker.Events:
				if !ok {
					t.Fatal("first native turn ended before its question")
				}
				processEvent(run8, event)
			}
		}
	}
	_, execution8 := service.workflowRecords(run8)
	firstTurn, firstThread, firstWorkspace := execution8.CurrentTurnID, execution8.SessionID, execution8.Workspace.Worktree
	if execution8.Process == nil || firstTurn == "" || firstThread == "" {
		t.Fatal("first task lost its native process, thread or turn")
	}
	processes = append(processes, *execution8.Process)
	if err := service.startWorkerFlowTurn(ctx, run9, "Reply READY only. Do not call steerline_ask.", newID()); err != nil {
		t.Fatal(err)
	}
	_, execution9 := service.workflowRecords(run9)
	if execution9.Process == nil || execution9.Process.PID == execution8.Process.PID || execution9.Workspace.Worktree == firstWorkspace {
		t.Fatal("second task did not receive an isolated app-server and worktree")
	}
	processes = append(processes, *execution9.Process)
	finish(run9)
	if run8.Decision == nil || run8.Decision.Status != "pending" || run8.Decision.Request.QuestionRoute == nil {
		t.Fatal("second task completion displaced the first native question")
	}
	_, execution9 = service.workflowRecords(run9)
	if execution9.SessionID == firstThread || execution9.CurrentTurnID == "" {
		t.Fatal("second task did not retain an independent native thread and turn")
	}
	callID := run8.Decision.Request.QuestionRoute.QuestionID
	if err := service.workers[run8.RunID].detachLocalObservation(ctx); err != nil {
		t.Fatal(err)
	}
	delete(service.workers, run8.RunID)
	if err := service.Close(); err != nil {
		t.Fatal(err)
	}
	service, err = NewService(c)
	if err != nil {
		t.Fatal(err)
	}
	run8 = service.find("8")
	_, execution8 = service.workflowRecords(run8)
	decisionStatus, hasRoutedQuestion := "", false
	if run8.Decision != nil {
		decisionStatus, hasRoutedQuestion = run8.Decision.Status, run8.Decision.Request.QuestionRoute != nil
	}
	t.Logf("before recovery: task=%s decision=%s routed=%t execution=%s turn=%s stopped=%s", run8.TaskStatus, decisionStatus, hasRoutedQuestion, execution8.Status, execution8.CurrentTurnID, execution8.LastStoppedTurnID)
	if err := service.resumeAppServerWorkers(ctx); err != nil {
		t.Fatal(err)
	}
	_, execution8 = service.workflowRecords(run8)
	if run8.Decision == nil || run8.Decision.Request.QuestionRoute == nil || run8.Decision.Request.QuestionRoute.QuestionID != callID || execution8.SessionID != firstThread || execution8.CurrentTurnID != firstTurn || service.workers[run8.RunID] == nil {
		ask := ""
		if run8.Decision != nil && run8.Decision.Request.QuestionRoute != nil {
			ask = run8.Decision.Request.QuestionRoute.QuestionID
		}
		t.Fatalf("restart failed to reattach original question: task=%s decision=%s ask=%s/%s execution=%s thread=%s/%s turn=%s/%s stopped=%s worker=%t reason=%q", run8.TaskStatus, decisionStatus, ask, callID, execution8.Status, execution8.SessionID, firstThread, execution8.CurrentTurnID, firstTurn, execution8.LastStoppedTurnID, service.workers[run8.RunID] != nil, deref(run8.Error))
	}
	service.State.Service.Ready, service.State.Service.Status = true, "running"
	if err := service.publishCheckpoint(); err != nil {
		t.Fatal(err)
	}
	answer(run8, map[string][]string{"choice": {"A"}}, "")
	finish(run8)
	_, execution8 = service.workflowRecords(run8)
	if execution8.LastStoppedTurnID != firstTurn || run8.Decision == nil || run8.Decision.Request.QuestionRoute != nil {
		t.Fatal("in-turn answer did not finish the original turn and offer post-turn feedback")
	}
	answer(run8, nil, "Feedback: reply ACK only.")
	finish(run8)
	_, execution8 = service.workflowRecords(run8)
	if execution8.SessionID != firstThread || execution8.Workspace.Worktree != firstWorkspace || execution8.CurrentTurnID == firstTurn || execution8.TurnNumber != 2 {
		t.Fatal("post-turn feedback did not stay on the original thread and worktree")
	}
	t.Logf("service-level native acceptance: run8=%s thread=%s turns=%s,%s; run9=%s thread=%s turn=%s; recovered call=%s", run8.RunID, firstThread, firstTurn, execution8.CurrentTurnID, run9.RunID, execution9.SessionID, execution9.CurrentTurnID, callID)
}

type appServerHostExitReceipt struct {
	Config    Config                       `json:"config"`
	Home      string                       `json:"home"`
	RunID     string                       `json:"run_id"`
	Process   executionapi.ProcessIdentity `json:"process"`
	ThreadID  string                       `json:"thread_id"`
	TurnID    string                       `json:"turn_id"`
	CallID    string                       `json:"call_id"`
	Workspace string                       `json:"workspace"`
}

// The helper exits the service-owning process while Codex awaits a tool
// response. TestLocalCodexManualRecoveryLive verifies the managed worker exit.
func TestAppServerHostExitHelper(t *testing.T) {
	root := os.Getenv("STEERLINE_APP_SERVER_HOST_EXIT_ROOT")
	if os.Getenv("STEERLINE_APP_SERVER_HOST_EXIT_HELPER") != "1" || root == "" {
		t.Skip("host-exit helper")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Minute)
	defer cancel()
	c, commit, store := openDAGFixture(t)
	c.Host, c.Agent, c.Claimant = "local", "codex", "operator"
	setTestImplementationProfiles(t, &c, codexworker.ProfileConfig{Executable: "codex", Model: "gpt-6-luna", ReasoningEffort: "low"}, nil)
	c.CommandTimeout = "45s"
	c.WorktreeRoot = filepath.Join(c.RunDir, "worktrees")
	if err := c.prepareWorktreeRoot(); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", c.Repository, "push", "--quiet", "origin", "HEAD:refs/heads/main").CombinedOutput(); err != nil {
		t.Fatalf("push isolated fixture: %v: %s", err, out)
	}
	commands := CommandService{Store: store, Project: c.Identity()}
	if _, err := commands.CreateTaskBatch(ctx, c, taskBatchFixture(t, c, commit, batchTasks(8), nil), newID(), 1, "operator"); err != nil {
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
	graph, err := store.taskGraphSnapshot(ctx, c.Identity())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := commands.ClaimNativeTask(ctx, c, TaskClaimOptions{TaskID: "8", RequestID: newID(), ExpectedTaskVersion: graph.ByID["8"].Version, ExpectedAuthorityVersion: graph.Authority.AuthorityVersion, RequestedBy: "operator", Explicit: true}); err != nil {
		t.Fatal(err)
	}
	service, err := NewService(c)
	if err != nil {
		t.Fatal(err)
	}
	run := service.find("8")
	t.Cleanup(func() {
		for _, worker := range service.workers {
			_ = worker.detachLocalObservation(context.Background())
		}
		_, saved := service.workflowRecords(run)
		if saved != nil && saved.Process != nil && inspectProcess(saved.Process) == "matching process still exists" {
			if process, err := os.FindProcess(saved.Process.PID); err == nil {
				_ = process.Kill()
			}
		}
		_ = service.Close()
	})
	if err := prepareWorktreeOnly(ctx, c, run); err != nil {
		t.Fatal(err)
	}
	run.WorktreePath, run.WorktreeBranch, run.BaseCommit = ptr(run.WorkspaceIntent.Path), ptr(run.WorkspaceIntent.Branch), ptr(run.WorkspaceIntent.BaseCommit)
	workspace, err := service.inspectWorkflowWorkspace(ctx, run)
	if err != nil {
		t.Fatal(err)
	}
	_, execution := service.workflowRecords(run)
	applyWorkflowWorkspaceEvidence(run, execution, workspace)
	if err := service.commitWorker(run); err != nil {
		t.Fatal(err)
	}
	if err := service.startWorkerFlowTurn(ctx, run, "Call steerline_ask with one question (id choice, header Choice, question Should I reply A or B?, options A and B, allow_free_text false). Wait for the answer. Then reply with the chosen letter only.", newID()); err != nil {
		t.Fatal(err)
	}
	worker := service.workers[run.RunID]
	if worker == nil {
		t.Fatal("original worker observer is missing")
	}
	for run.Decision == nil || run.Decision.Request.QuestionRoute == nil {
		select {
		case <-ctx.Done():
			t.Fatal("native question did not arrive before host exit")
		case event, ok := <-worker.Events:
			if !ok {
				t.Fatal("native turn ended before host exit")
			}
			if err := service.eventForRun(run, event); err != nil {
				t.Fatal(err)
			}
		}
	}
	_, execution = service.workflowRecords(run)
	if execution.Process == nil {
		t.Fatal("native app-server has no durable process identity")
	}
	receipt := appServerHostExitReceipt{Config: c, Home: os.Getenv("STEERLINE_HOME"), RunID: run.RunID, Process: *execution.Process, ThreadID: execution.SessionID, TurnID: execution.CurrentTurnID, CallID: run.Decision.Request.QuestionRoute.QuestionID, Workspace: execution.Workspace.Worktree}
	if err := AtomicWrite(filepath.Join(root, "host-exit-receipt.json"), mustJSON(receipt)); err != nil {
		t.Fatal(err)
	}
	// os.Exit closes the service process and its observer without invoking test
	// cleanup. The parent owns the isolated tree and exact process cleanup.
	os.Exit(0)
}

func TestAppServerHostExitRecoveryLive(t *testing.T) {
	if os.Getenv("STEERLINE_APP_SERVER_SERVICE_LIVE_LEGACY") != "1" {
		t.Skip("historical detached-process acceptance")
	}
	root, err := os.MkdirTemp("", "sl-host-exit-")
	if err != nil {
		t.Fatal(err)
	}
	var receipt appServerHostExitReceipt
	t.Cleanup(func() {
		_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil || entry.IsDir() || entry.Name() != "process.json" {
				return nil
			}
			var processReceipt codexworker.ProcessReceipt
			if data, err := os.ReadFile(path); err == nil && json.Unmarshal(data, &processReceipt) == nil && inspectProcess(&processReceipt.Identity) == "matching process still exists" {
				if process, err := os.FindProcess(processReceipt.Identity.PID); err == nil {
					_ = process.Kill()
				}
			}
			return nil
		})
		if inspectProcess(&receipt.Process) == "matching process still exists" {
			if process, err := os.FindProcess(receipt.Process.PID); err == nil {
				_ = process.Kill()
			}
		}
		_ = os.RemoveAll(root)
	})
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^TestAppServerHostExitHelper$", "-test.timeout=8m")
	cmd.Env = overrideEnv(os.Environ(), map[string]string{"STEERLINE_APP_SERVER_HOST_EXIT_HELPER": "1", "STEERLINE_APP_SERVER_HOST_EXIT_ROOT": root, "TEMP": root, "TMP": root})
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("service host exited before the native question was saved: %v: %s", err, output)
	}
	data, err := os.ReadFile(filepath.Join(root, "host-exit-receipt.json"))
	if err != nil || json.Unmarshal(data, &receipt) != nil || receipt.Home == "" || receipt.RunID == "" || receipt.ThreadID == "" || receipt.TurnID == "" || receipt.CallID == "" || inspectProcess(&receipt.Process) != "matching process still exists" {
		t.Fatalf("app-server did not survive its service process: receipt=%+v error=%v", receipt, err)
	}
	t.Setenv("STEERLINE_HOME", receipt.Home)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	service, err := NewService(receipt.Config)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	run := service.find("8")
	if run == nil || run.RunID != receipt.RunID {
		t.Fatal("new host lost the fixed workflow run")
	}
	if err := service.resumeAppServerWorkers(ctx); err != nil {
		t.Fatal(err)
	}
	worker := service.workers[run.RunID]
	_, execution := service.workflowRecords(run)
	if worker == nil || run.Decision == nil || run.Decision.Request.QuestionRoute == nil || run.Decision.Request.QuestionRoute.QuestionID != receipt.CallID || execution.SessionID != receipt.ThreadID || execution.CurrentTurnID != receipt.TurnID || execution.Workspace.Worktree != receipt.Workspace {
		t.Fatalf("new host did not recover the original request: worker=%t reason=%q", worker != nil, deref(run.Error))
	}
	defer worker.detachLocalObservation(context.Background())
	service.State.Service.Ready, service.State.Service.Status = true, "running"
	if err := service.publishCheckpoint(); err != nil {
		t.Fatal(err)
	}
	answer := DecisionAnswer{DecisionID: run.Decision.ID, RunID: run.RunID, Answers: map[string][]string{"choice": {"A"}}, AnsweredBy: "operator", ExpectedDecisionVersion: run.Decision.Version, ExpectedRunRevision: run.ControlRevision, ServiceInstanceID: service.State.Service.InstanceID}
	command, _, err := (CommandService{Store: service.domainStore, Project: service.Config.identity}).AnswerDecisionWithID(ctx, newID(), answer.DecisionID, run.Decision.Version, decisionAnswerKey(answer), answer.AnsweredBy, mustJSON(answer))
	if err != nil {
		t.Fatal(err)
	}
	record, _, err := (CommandService{Store: service.domainStore, Project: service.Config.identity}).AcceptAnswer(ctx, command, answer)
	if err != nil {
		t.Fatal(err)
	}
	run.Decision.Version, run.Decision.Status = record.Version, record.Status
	run.Decision.Answers, run.Decision.AnsweredBy = answer.Answers, answer.AnsweredBy
	run.Decision.AnsweredAt = &record.UpdatedAt
	if err := service.publishCheckpoint(); err != nil {
		t.Fatal(err)
	}
	if err := service.applyWorkerFlowDecision(ctx, run); err != nil {
		t.Fatal(err)
	}
	for {
		select {
		case <-ctx.Done():
			t.Fatal("recovered native turn did not finish")
		case event, ok := <-worker.Events:
			if ok {
				if err := service.eventForRun(run, event); err != nil {
					t.Fatal(err)
				}
				continue
			}
			if err := <-worker.Done; err != nil {
				t.Fatal(err)
			}
			delete(service.workers, run.RunID)
			if err := service.finishWorkerFlowFor(run, nil); err != nil {
				t.Fatal(err)
			}
			_, execution = service.workflowRecords(run)
			if execution.LastStoppedTurnID != receipt.TurnID || run.Decision == nil || run.Decision.Request.QuestionRoute != nil {
				t.Fatal("new host did not settle the original native turn")
			}
			t.Logf("host-exit native recovery: process=%d thread=%s turn=%s call=%s", receipt.Process.PID, receipt.ThreadID, receipt.TurnID, receipt.CallID)
			return
		}
	}
}
