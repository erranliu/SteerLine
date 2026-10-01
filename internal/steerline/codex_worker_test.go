package steerline

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/codexapp"
	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

func TestAppServerPendingFeedbackRecoveryUsesNewTurnReceipt(t *testing.T) {
	for _, receiptExists := range []bool{true, false} {
		t.Run(fmtBool(receiptExists), func(t *testing.T) {
			workspace := t.TempDir()
			first, second := "00000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000002"
			state := &mockAppServerState{cwd: workspace, turns: []map[string]any{{"id": first, "status": "completed", "completedAt": 1}, {"id": second, "status": "completed", "completedAt": 2}}}
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			go func() {
				for {
					conn, err := listener.Accept()
					if err != nil {
						return
					}
					go mockAppServerConnection(conn, state)
				}
			}()
			identity, err := currentProcessIdentity()
			if err != nil {
				t.Fatal(err)
			}
			c := Config{RunDir: t.TempDir()}
			run := &Run{RunID: newID(), WorktreePath: &workspace}
			endpoint := "ws://" + listener.Addr().String()
			execution := &ExecutionRecord{Process: identity, CurrentCommandID: newID(), CurrentTurnID: first, LastStoppedTurnID: first, PendingCommandID: newID()}
			setTestCodexBinding(t, execution, endpoint, "generation", workerFlowMockSession)
			serverDir := filepath.Join(c.RunDir, "runs", run.RunID, "app-server")
			if err := os.MkdirAll(serverDir, 0700); err != nil {
				t.Fatal(err)
			}
			if err := AtomicWrite(filepath.Join(serverDir, "process.json"), mustJSON(codexworker.ProcessReceipt{Endpoint: endpoint, Generation: "generation", Identity: *identity})); err != nil {
				t.Fatal(err)
			}
			turnDir := filepath.Join(c.RunDir, "runs", run.RunID, "turns", execution.PendingCommandID)
			if err := os.MkdirAll(turnDir, 0700); err != nil {
				t.Fatal(err)
			}
			if receiptExists {
				if err := AtomicWrite(filepath.Join(turnDir, "turn-start.json"), mustJSON(map[string]any{"turn": map[string]string{"id": second}})); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			worker, completed, err := reattachAppServerWorker(ctx, c, run, execution)
			if !receiptExists {
				if err == nil || completed || worker != nil || execution.CurrentTurnID != first {
					t.Fatalf("lost receipt reused the preceding turn: completed=%v err=%v execution=%+v", completed, err, execution)
				}
				return
			}
			if err != nil || !completed || worker != nil || execution.PendingTurnID != second {
				t.Fatalf("pending feedback was not recovered: %v %v %+v", completed, err, execution)
			}
			outcome, err := codexworker.ReadAppServerOutcome(filepath.Join(turnDir, "appserver.jsonl"), workerFlowMockSession, second)
			if err != nil || !outcome.Completed {
				t.Fatalf("wrong recovered turn journal: %+v %v", outcome, err)
			}
			if _, err := os.Stat(filepath.Join(c.RunDir, "runs", run.RunID, "turns", execution.CurrentCommandID, "appserver.jsonl")); !os.IsNotExist(err) {
				t.Fatalf("recovery rewrote the preceding turn: %v", err)
			}
		})
	}
}

func TestAppServerStartReplayPreservesNativeSavedAnswer(t *testing.T) {
	first, second := "00000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000002"
	commandID := newID()
	run := &Run{RunID: newID(), ExecutionID: newID(), AttemptID: newID(), Decision: &DecisionState{Status: "answer_saved", Request: DecisionRequest{FormatVersion: contracts.DecisionRequestFormatVersion, Question: "Continue?", AllowFreeText: true}}}
	workflow := &WorkflowRunRecord{WorkflowRunID: run.RunID, WorkflowVersion: WorkerFlowVersion, Phase: "waiting_decision"}
	execution := &ExecutionRecord{ExecutionID: run.ExecutionID, WorkflowRunID: run.RunID, CurrentAttemptID: run.AttemptID, CurrentStep: "worker", CurrentTurnID: first, LastStoppedTurnID: first, PendingTurnID: second, PendingCommandID: commandID, TurnNumber: 1}
	setTestCodexBinding(t, execution, "ws://127.0.0.1:12345", "generation", workerFlowMockSession)
	service := &Service{domain: DomainSnapshot{Operations: []OperationRecord{{OperationID: newID(), IdempotencyKey: "worker-turn:" + run.ExecutionID + ":" + commandID, Status: "outcome_unknown"}}}}
	service.Config.identity = "test-project"
	run.TaskID, run.DomainTaskID, workflow.TaskID = "task", "task", "task"
	service.domain.Tasks = []TaskRecord{{TaskID: "task", Status: "in_progress"}}
	service.domain.Attempts = []StepAttemptRecord{{ExecutionID: run.ExecutionID, AttemptID: run.AttemptID, StepID: "worker", AttemptNumber: 1}}
	service.domain.WorkflowRuns, service.domain.Executions = []WorkflowRunRecord{*workflow}, []ExecutionRecord{*execution}
	workflow, execution = &service.domain.WorkflowRuns[0], &service.domain.Executions[0]
	run.Decision.ID, run.Decision.Version = newID(), 1
	run.Decision.AttemptID = run.AttemptID
	service.setRunDecision(run, run.Decision)
	if _, err := service.recordWorkerTurnStarted(run, workflow, execution, second); err != nil {
		t.Fatal(err)
	}
	if execution.CurrentCommandID != commandID || execution.CurrentTurnID != second || execution.PendingCommandID != "" || execution.TurnNumber != 2 || run.Decision.Status != "reply_accepted" || service.domain.Operations[0].Status != "confirmed" {
		t.Fatalf("second turn receipt was not applied: execution=%+v decision=%+v", execution, run.Decision)
	}
	fixed := executionapi.SessionContext{ProjectID: execution.ProjectIdentity, WorkflowRunID: run.RunID, ExecutionID: run.ExecutionID, AttemptID: run.AttemptID, SessionID: workerFlowMockSession, Generation: execution.SessionGeneration, CurrentTurnID: second}
	request := codexDecisionFixture(t, fixed, "pending", []executionapi.Question{{ID: "choice", Question: "Continue?", AllowFreeText: true}})
	service.setRunDecision(run, &DecisionState{ID: newID(), AttemptID: run.AttemptID, Version: 1, Status: "answer_saved", Request: request})
	workflow.Phase = "waiting_decision"
	run.FinishTaskStatus = "waiting_decision"
	if _, err := service.recordWorkerTurnStarted(run, workflow, execution, second); err != nil {
		t.Fatal(err)
	}
	if execution.TurnNumber != 2 || run.Decision.Status != "answer_saved" || workflow.Phase != "waiting_decision" || run.FinishTaskStatus != "waiting_decision" {
		t.Fatalf("start replay accepted an unsent answer or changed the waiting state: turns=%d decision=%s phase=%s finish=%s", execution.TurnNumber, run.Decision.Status, workflow.Phase, run.FinishTaskStatus)
	}
}

func TestAppServerRejectedEventReleasesObserver(t *testing.T) {
	service := &Service{}
	event := WorkerEvent{RunID: strings.Repeat("a", 32), Persisted: make(chan error, 1)}
	if err := service.eventWorkerFlowFor(nil, event); err == nil {
		t.Fatal("invalid event accepted")
	}
	select {
	case err := <-event.Persisted:
		if err == nil {
			t.Fatal("failed event acknowledged as durable")
		}
	default:
		t.Fatal("observer is stranded waiting for rejected event")
	}
}

func TestAppServerRecoveryDoesNotRaceInFlightLaunch(t *testing.T) {
	run := &Run{RunID: newID(), ExecutionID: newID(), DomainTaskID: "8", Protocol: string(executionapi.ProtocolLocalCodexAppServer), WorkflowVersion: WorkerFlowVersion}
	service := &Service{appServerLaunching: map[string]string{run.RunID: newID()}, State: Runtime{}, domain: DomainSnapshot{Tasks: []TaskRecord{{TaskID: "8", Status: "in_progress"}},
		WorkflowRuns: []WorkflowRunRecord{{WorkflowRunID: run.RunID, WorkflowVersion: WorkerFlowVersion, Phase: "queued"}},
		Executions:   []ExecutionRecord{{ExecutionID: run.ExecutionID, WorkflowRunID: run.RunID, PendingCommandID: newID()}}}, runs: []*Run{run},
	}
	before := digestValue(struct {
		View   Runtime
		Domain DomainSnapshot
	}{service.State, service.domain})
	if err := service.resumeAppServerWorkers(context.Background()); err != nil {
		t.Fatal(err)
	}
	if before != digestValue(struct {
		View   Runtime
		Domain DomainSnapshot
	}{service.State, service.domain}) {
		t.Fatal("recovery modified a still-running launch before its receipt")
	}
}

func TestAppServerLateReplyCannotModifyTerminalTask(t *testing.T) {
	for _, status := range []string{"closed", "discarded"} {
		route := simpleQuestionRoute(t, codexworker.QuestionRouteImplementationKey, "project", newID(), newID(), newID(), "generation", "turn", "original")
		run := &Run{RunID: newID(), TaskStatus: status, Decision: &DecisionState{ID: newID(), Status: "answer_saved", Request: DecisionRequest{QuestionRoute: &route}}}
		service := &Service{State: Runtime{}, domain: DomainSnapshot{Operations: []OperationRecord{{OperationID: "reply", Status: "in_flight"}}}, runs: []*Run{run}}
		before := digestValue(struct {
			View   Runtime
			Domain DomainSnapshot
		}{service.State, service.domain})
		late := &lifecycleReplyMessage{request: executionapi.LifecycleRequest{Input: executionapi.LifecycleInput{Feedback: &executionapi.FeedbackInput{DecisionID: run.Decision.ID, QuestionID: "original"}}}}
		if err := service.handleLifecycleFeedbackMessage(run.RunID, late); err != nil {
			t.Fatal(err)
		}
		if before != digestValue(struct {
			View   Runtime
			Domain DomainSnapshot
		}{service.State, service.domain}) {
			t.Fatalf("late reply changed %s task", status)
		}
	}
}

func fmtBool(value bool) string {
	if value {
		return "receipt_saved"
	}
	return "receipt_lost"
}

func appServerAuditService(t *testing.T) (*Service, *Run) {
	return appServerAuditServiceWithPolicy(t, workspacePolicyFixture())
}

func appServerAuditServiceWithPolicy(t *testing.T, policy executionapi.ExecutionPolicy) (*Service, *Run) {
	return appServerAuditServiceWithRequirements(t, policy, executionapi.ExecutionRequirements{}, "")
}

func appServerAuditServiceWithRequirements(t *testing.T, policy executionapi.ExecutionPolicy, requirements executionapi.ExecutionRequirements, tokenEnv string) (*Service, *Run) {
	t.Helper()
	c, commit, store := openDAGFixture(t)
	c.WorkflowProtocol, c.Host, c.Agent, c.Claimant = WorkerFlowVersion, "local", "codex", "operator"
	c.ExecutionPolicy = policy
	c.ExecutionRequirements = requirements
	if tokenEnv != "" {
		c.PR.TokenEnv = tokenEnv
	}
	if policy.Filesystem.WriteScope == "none" {
		settings, err := codexworker.NormalizeProfileConfig(codexworker.ProfileConfig{Executable: os.Args[0]}, policy, runtime.GOOS, "local", "codex")
		require(t, err)
		setTestCodexProfile(t, &c, settings)
	}
	require(t, validateWorkflowConfig(&c, nil))
	require(t, c.prepareWorktreeRoot())
	ctx := context.Background()
	if out, err := exec.Command("git", "-C", c.Repository, "push", "--quiet", "origin", "HEAD:refs/heads/main").CombinedOutput(); err != nil {
		t.Fatalf("fixture push: %v: %s", err, out)
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
	if _, err := commands.ClaimNativeTask(ctx, c, TaskClaimOptions{TaskID: "8", RequestID: newID(), ExpectedTaskVersion: 1, ExpectedAuthorityVersion: authority.AuthorityVersion, RequestedBy: "operator", Explicit: true}); err != nil {
		t.Fatal(err)
	}
	service, err := newTestService(c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close() })
	run := service.find("8")
	if run == nil {
		t.Fatal("claimed run missing")
	}
	return service, run
}

func TestAppServerUnknownTurnDoesNotOfferPostTurnFeedback(t *testing.T) {
	service, run := appServerAuditService(t)
	workflow, execution := service.workflowRecords(run)
	execution.CurrentTurnID, execution.CurrentCommandID = "00000000-0000-0000-0000-000000000001", newID()
	setTestCodexBinding(t, execution, "ws://127.0.0.1:12345", "generation", workerFlowMockSession)
	execution.PolicyResources = policyResourceReceiptFixture(workflow.ExecutionProfile.Digest, execution.Workspace.Worktree, execution)
	execution.Status, workflow.Phase = "blocked", "executing"
	run.ExecutionStatus = "unknown"
	if err := service.waitWorkerFlow(run, "observation disconnected"); err != nil {
		t.Fatal(err)
	}
	if run.Decision != nil {
		t.Fatal("unknown active turn was presented as stopped and solicited new feedback")
	}
	worker := &Worker{cancelObservation: func() {}, observationDone: make(chan struct{})}
	close(worker.observationDone)
	if err := service.applyAppServerRecovery(appServerRecoveryResult{runID: run.RunID, execution: *execution, worker: worker}); err != nil {
		t.Fatal(err)
	}
	workflow, execution = service.workflowRecords(run)
	if execution.Status != "active" || run.ExecutionStatus != "running" || workflow.Phase != "executing" || run.Error != nil {
		t.Fatalf("recovered active turn retained its blocked state: %+v %+v", execution, run)
	}
	delete(service.workers, run.RunID)
}

func TestAppServerSlowRecoveryLeavesServiceWriterAvailable(t *testing.T) {
	service, run := appServerAuditService(t)
	workflow, execution := service.workflowRecords(run)
	execution.PolicyResources = policyResourceReceiptFixture(workflow.ExecutionProfile.Digest, execution.Workspace.Worktree)
	identity, err := currentProcessIdentity()
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	execution.Process, run.Process = identity, identity
	endpoint := "ws://" + listener.Addr().String()
	setTestCodexBinding(t, execution, endpoint, "generation", workerFlowMockSession)
	execution.PolicyResources = policyResourceReceiptFixture(workflow.ExecutionProfile.Digest, execution.Workspace.Worktree, execution)
	execution.CurrentTurnID, execution.CurrentCommandID = "00000000-0000-0000-0000-000000000001", newID()
	execution.Status, workflow.Phase = "active", "executing"
	input := executionapi.TurnInput{CommandID: execution.CurrentCommandID, Prompt: "original persisted input"}
	if _, err := service.startWorkerOperation(run, "start_worker_turn", "worker-turn:"+execution.ExecutionID+":"+input.CommandID, &input); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(service.Config.RunDir, "runs", run.RunID, "app-server", "process.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := AtomicWrite(path, mustJSON(codexworker.ProcessReceipt{Endpoint: endpoint, Generation: "generation", Identity: *identity})); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service.runContext, service.workerMessages, service.State.Service.Ready = ctx, make(chan workerMessage, 4), true
	started := time.Now()
	if err := service.resumeAppServerWorkers(ctx); err != nil {
		t.Fatal(err)
	}
	if time.Since(started) > time.Second || !service.appServerRecovering[run.RunID] {
		t.Fatal("slow recovery blocked the service writer")
	}
	if err := service.resumeAppServerWorkers(ctx); err != nil {
		t.Fatal(err)
	}
	conn, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	cancel()
	done := make(chan struct{})
	go func() { service.forwarders.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("canceled recovery left a background probe")
	}
}

func testAppServerAnswerRecoveryPersistsBeforeNextQuestion(t *testing.T) {
	service, run := appServerAuditService(t)
	workflow, execution := service.workflowRecords(run)
	execution.CurrentTurnID, execution.CurrentCommandID = "00000000-0000-0000-0000-000000000001", newID()
	execution.Status, workflow.Phase = "active", "waiting_decision"
	run.FinishTaskStatus, run.ExecutionStatus = "waiting_decision", "running"
	setTestCodexBinding(t, execution, "ws://127.0.0.1:12345", "generation", workerFlowMockSession)
	execution.PolicyResources = policyResourceReceiptFixture(workflow.ExecutionProfile.Digest, execution.Workspace.Worktree, execution)
	execution.CurrentCommandID = newID()
	execution.CurrentTurnID = "00000000-0000-0000-0000-000000000003"
	fixed := *workerSessionContext(run, execution)
	fixed.ProjectID = service.Config.identity
	ask := codexDecisionFixture(t, fixed, "first", []executionapi.Question{{ID: "choice", Header: "Choice", Question: "Continue?", AllowFreeText: true}})
	var err error
	run.Decision, err = WriteDecisionRequest(run, ask)
	if err != nil {
		t.Fatal(err)
	}
	service.setRunDecision(run, run.Decision)
	record := service.decisionRecord(run)
	record.Status, record.AnsweredBy = "answer_saved", "tester"
	acceptedAnswer := DecisionAnswer{DecisionID: record.DecisionID, RunID: run.RunID, Answers: map[string][]string{"choice": {"A"}}, AnsweredBy: "tester", AnsweredAt: now()}
	record.Answer = mustJSON(acceptedAnswer)
	service.refreshRun(run)
	firstID := run.Decision.ID
	feedback := &executionapi.FeedbackInput{DecisionID: firstID, QuestionID: ask.QuestionRoute.QuestionID, Answer: &executionapi.Answer{Answers: acceptedAnswer.Answers}}
	if _, err := service.startWorkerOperation(run, "reply_execution_question", "worker-question-answer:"+execution.ExecutionID+":"+firstID, feedback); err != nil {
		t.Fatal(err)
	}
	workflow, execution = service.workflowRecords(run)
	fixed = *workerSessionContext(run, execution)
	next := codexDecisionFixture(t, fixed, "second", []executionapi.Question{{ID: "choice", Header: "Choice", Question: "Continue?", AllowFreeText: true}})
	execution.PendingQuestions = []PendingQuestion{{Questions: next.Questions, Route: *next.QuestionRoute}}
	path := filepath.Join(service.Config.RunDir, "runs", run.RunID, "turns", execution.CurrentCommandID, "appserver.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	event := codexapp.Envelope{Method: "item/completed", Params: mustJSON(map[string]any{"threadId": fixed.SessionID, "turnId": fixed.CurrentTurnID, "item": map[string]any{"id": "first", "type": "dynamicToolCall", "status": "completed", "success": true, "contentItems": []any{map[string]string{"type": "inputText", "text": `{"answers":{"choice":["A"]}}`}}}})}
	if err := AtomicWrite(path, append(mustJSON(event), '\n')); err != nil {
		t.Fatal(err)
	}
	if err := service.reconcileSavedLifecycleQuestionAnswer(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	snapshot, err := service.domainStore.Query(context.Background(), service.Config.identity)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, decision := range snapshot.Decisions {
		if decision.DecisionID == firstID {
			found = decision.Status == "reply_accepted"
		}
	}
	if !found || run.Decision.Request.QuestionRoute.QuestionID != "second" || len(snapshot.Executions[0].PendingQuestions) != 0 {
		t.Fatalf("original acceptance lost while advancing questions: %+v", snapshot.Decisions)
	}
}

func TestAppServerOutcomeUsesNativeTurnAndEarlierPRItem(t *testing.T) {
	thread, turn := "00000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000002"
	path := filepath.Join(t.TempDir(), "appserver.jsonl")
	events := []codexapp.Envelope{
		{Method: "turn/started", Params: mustJSON(map[string]any{"threadId": thread, "turn": map[string]any{"id": turn}})},
		{Method: "item/completed", Params: mustJSON(map[string]any{"threadId": thread, "turnId": turn, "item": map[string]any{"type": "agentMessage", "text": "PR https://git.example/o/r/pulls/8"}})},
		{Method: "turn/completed", Params: mustJSON(map[string]any{"threadId": thread, "turn": map[string]any{"id": turn, "status": "completed", "items": []any{map[string]any{"type": "agentMessage", "phase": "final_answer", "text": "Done"}}}})},
	}
	var raw []byte
	for _, event := range events {
		line, _ := json.Marshal(event)
		raw = append(raw, append(line, '\n')...)
	}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	outcome, err := codexworker.ReadAppServerOutcome(path, thread, turn)
	if err != nil || !outcome.Started || !outcome.Completed || outcome.NativeStatus != "completed" || outcome.LastMessage != "Done" || len(outcome.PRCandidates) != 1 || outcome.PRCandidates[0] != "https://git.example/o/r/pulls/8" {
		t.Fatalf("native outcome lost evidence: %+v %v", outcome, err)
	}
}

func TestRoutedDecisionRequiresAllQuestionIDs(t *testing.T) {
	route := simpleQuestionRoute(t, "example.implementation", "project", "workflow", "execution", "attempt", "generation", "turn", "native-question")
	questions := []executionapi.Question{
		{ExecutionID: "execution", AttemptID: "attempt", Generation: "generation", TurnID: "turn", ID: "one", Header: "One", Question: "First?", Options: []executionapi.DecisionOption{{ID: "a", Label: "A"}}},
		{ExecutionID: "execution", AttemptID: "attempt", Generation: "generation", TurnID: "turn", ID: "two", Header: "Two", Question: "Second?", AllowFreeText: true},
	}
	request := DecisionRequest{FormatVersion: 2, Question: "Two questions", AllowFreeText: true, Questions: questions, QuestionRoute: &route}
	answer := DecisionAnswer{AnsweredBy: "tester", Note: `{"one":["A"],"two":["scope"]}`}
	if err := normalizeNativeDecisionAnswer(&request, &answer); err != nil {
		t.Fatal(err)
	}
	if len(answer.Answers) != 2 {
		t.Fatal("native answers lost a question")
	}
	delete(answer.Answers, "two")
	if err := validateDecisionAnswer(&request, &answer); err == nil {
		t.Fatal("missing second answer was accepted")
	}
}

func TestMockAppServerThreadAndTurnContract(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	endpoint := "ws://" + listener.Addr().String()
	_ = listener.Close()
	cmd := exec.Command(os.Args[0], "-test.run=^TestWorkerFlowMockCodexHelper$", "--", "app-server", "--listen", endpoint)
	cmd.Env = append(os.Environ(), "STEERLINE_WORKER_FLOW_MOCK=1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	client, err := codexworker.ConnectAppServer(ctx, endpoint)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	var started struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	workspace := t.TempDir()
	if err := client.Call(ctx, "thread/start", map[string]any{"cwd": workspace, "dynamicTools": []any{codexapp.AskDynamicTool()}}, &started); err != nil {
		t.Fatal(err)
	}
	var turn struct {
		Turn struct {
			ID string `json:"id"`
		} `json:"turn"`
	}
	if err := client.Call(ctx, "turn/start", map[string]any{"threadId": started.Thread.ID, "input": []map[string]string{{"type": "text", "text": "hello"}}}, &turn); err != nil {
		t.Fatal(err)
	}
	if started.Thread.ID != workerFlowMockSession || !codexworker.ValidNativeID(turn.Turn.ID) || turn.Turn.ID == started.Thread.ID {
		t.Fatalf("mock native identities are invalid: %s %s", started.Thread.ID, turn.Turn.ID)
	}
	for {
		select {
		case <-ctx.Done():
			t.Fatal("mock turn did not complete")
		case event := <-client.Notifications():
			if event.Method == "turn/completed" {
				return
			}
		}
	}
}

func TestAppServerStartedEventCommitsOperationReceipt(t *testing.T) {
	service, run := appServerAuditService(t)
	_, execution := service.workflowRecords(run)
	execution.PolicyResources = policyResourceReceiptFixture(service.workflowRecord(run).ExecutionProfile.Digest, execution.Workspace.Worktree, execution)
	commandID, turnID := newID(), "00000000-0000-0000-0000-000000000001"
	setTestCodexBinding(t, execution, "ws://127.0.0.1:12345", "generation", workerFlowMockSession)
	execution.PendingCommandID, execution.PendingTurnID = commandID, turnID
	opID, err := service.startWorkerOperation(run, "start_worker_turn", "worker-turn:"+run.ExecutionID+":"+commandID, map[string]string{"command_id": commandID})
	require(t, err)
	event := WorkerEvent{Fact: executionapi.EventFact{Kind: executionapi.EventStarted, SessionID: workerFlowMockSession, StartedTurnID: turnID}}
	require(t, service.eventWorkerFlowFor(run, event))
	snapshot, err := service.domainStore.Query(context.Background(), service.Config.identity)
	require(t, err)
	for _, op := range snapshot.Operations {
		if op.OperationID == opID {
			if op.Status != "confirmed" || snapshot.Executions[0].CurrentTurnID != turnID || snapshot.Executions[0].PendingCommandID != "" {
				t.Fatalf("native start and operation were not committed together: operation=%s execution=%+v", op.Status, snapshot.Executions[0])
			}
			return
		}
	}
	t.Fatal("original start operation disappeared")
}
