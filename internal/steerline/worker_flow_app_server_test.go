package steerline

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
			execution := &ExecutionRecord{Process: identity, SessionID: workerFlowMockSession, AppServerEndpoint: "ws://" + listener.Addr().String(), AppServerGeneration: "generation", CurrentCommandID: newID(), CurrentTurnID: first, LastStoppedTurnID: first, PendingCommandID: newID()}
			serverDir := filepath.Join(c.RunDir, "runs", run.RunID, "app-server")
			if err := os.MkdirAll(serverDir, 0700); err != nil {
				t.Fatal(err)
			}
			if err := AtomicWrite(filepath.Join(serverDir, "process.json"), mustJSON(appServerProcessReceipt{Endpoint: execution.AppServerEndpoint, Generation: execution.AppServerGeneration, Identity: *identity})); err != nil {
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
			outcome, err := readAppServerTurnOutcome(filepath.Join(turnDir, "appserver.jsonl"), workerFlowMockSession, second)
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
	run := &Run{RunID: newID(), ExecutionID: newID(), Decision: &DecisionState{Status: "answer_saved"}}
	workflow := &WorkflowRunRecord{WorkflowRunID: run.RunID, WorkflowVersion: WorkerFlowVersion, Phase: "waiting_decision"}
	execution := &ExecutionRecord{ExecutionID: run.ExecutionID, WorkflowRunID: run.RunID, SessionID: workerFlowMockSession, CurrentTurnID: first, LastStoppedTurnID: first, PendingTurnID: second, PendingCommandID: commandID, TurnNumber: 1}
	service := &Service{State: Runtime{Operations: []OperationRecord{{OperationID: newID(), IdempotencyKey: "worker-turn:" + run.ExecutionID + ":" + commandID, Status: "outcome_unknown"}}}}
	if err := service.recordAppServerTurnStarted(run, workflow, execution, second); err != nil {
		t.Fatal(err)
	}
	if execution.CurrentCommandID != commandID || execution.CurrentTurnID != second || execution.PendingCommandID != "" || execution.TurnNumber != 2 || run.Decision.Status != "reply_accepted" || service.State.Operations[0].Status != "confirmed" {
		t.Fatalf("second turn receipt was not applied: %+v %+v", execution, run.Decision)
	}
	run.Decision = &DecisionState{Status: "answer_saved", Request: DecisionRequest{NativeAsk: &codexAskCall{CallID: "pending"}}}
	workflow.Phase = "waiting_decision"
	run.FinishTaskStatus = "waiting_decision"
	if err := service.recordAppServerTurnStarted(run, workflow, execution, second); err != nil {
		t.Fatal(err)
	}
	if execution.TurnNumber != 2 || run.Decision.Status != "answer_saved" || workflow.Phase != "waiting_decision" || run.FinishTaskStatus != "waiting_decision" {
		t.Fatal("start replay accepted an unsent answer or changed the waiting state")
	}
}

func TestAppServerRejectedEventReleasesObserver(t *testing.T) {
	service := &Service{}
	event := WorkerEvent{RunID: strings.Repeat("a", 32), Persisted: make(chan error, 1)}
	if err := service.eventWorkerFlow(event); err == nil {
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
	run := &Run{RunID: newID(), ExecutionID: newID(), DomainTaskID: "8", Protocol: string(ProtocolLocalCodexAppServer), WorkflowVersion: WorkerFlowVersion}
	service := &Service{appServerLaunching: map[string]string{run.RunID: newID()}, State: Runtime{
		Runs:         []*Run{run},
		Tasks:        []TaskRecord{{TaskID: "8", Status: "in_progress"}},
		WorkflowRuns: []WorkflowRunRecord{{WorkflowRunID: run.RunID, WorkflowVersion: WorkerFlowVersion, Phase: "queued"}},
		Executions:   []ExecutionRecord{{ExecutionID: run.ExecutionID, WorkflowRunID: run.RunID, PendingCommandID: newID()}},
	}}
	before := digestValue(service.State)
	if err := service.resumeAppServerWorkers(context.Background()); err != nil {
		t.Fatal(err)
	}
	if before != digestValue(service.State) {
		t.Fatal("recovery modified a still-running launch before its receipt")
	}
}

func TestAppServerLateReplyCannotModifyTerminalTask(t *testing.T) {
	for _, status := range []string{"closed", "discarded"} {
		run := &Run{RunID: newID(), TaskStatus: status, Decision: &DecisionState{ID: newID(), Status: "answer_saved", Request: DecisionRequest{NativeAsk: &codexAskCall{CallID: "original"}}}}
		service := &Service{State: Runtime{Runs: []*Run{run}, Operations: []OperationRecord{{OperationID: "reply", Status: "in_flight"}}}}
		before := digestValue(service.State)
		if err := service.handleAppServerReplyMessage(run.RunID, &appServerReplyMessage{decisionID: run.Decision.ID, opID: "reply", callID: "original"}); err != nil {
			t.Fatal(err)
		}
		if before != digestValue(service.State) {
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
	t.Helper()
	c, commit, store := openDAGFixture(t)
	c.WorkflowProtocol, c.Host, c.Agent, c.Claimant = WorkerFlowVersion, "local", "codex", "operator"
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
	service, err := NewService(c)
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
	workflow, execution := service.minimalRecords(run)
	execution.SessionID, execution.CurrentTurnID, execution.CurrentCommandID = workerFlowMockSession, "00000000-0000-0000-0000-000000000001", newID()
	execution.Status, workflow.Phase = "blocked", "executing"
	run.SessionID, run.ExecutionStatus = ptr(execution.SessionID), "unknown"
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
	workflow, execution = service.minimalRecords(run)
	if execution.Status != "active" || run.ExecutionStatus != "running" || workflow.Phase != "executing" || run.Error != nil {
		t.Fatalf("recovered active turn retained its blocked state: %+v %+v", execution, run)
	}
	delete(service.workers, run.RunID)
}

func TestAppServerSlowRecoveryLeavesServiceWriterAvailable(t *testing.T) {
	service, run := appServerAuditService(t)
	workflow, execution := service.minimalRecords(run)
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
	execution.AppServerEndpoint, execution.AppServerGeneration = "ws://"+listener.Addr().String(), "generation"
	execution.SessionID, execution.CurrentTurnID, execution.CurrentCommandID = workerFlowMockSession, "00000000-0000-0000-0000-000000000001", newID()
	execution.Status, workflow.Phase = "active", "executing"
	run.SessionID = ptr(execution.SessionID)
	path := filepath.Join(service.Config.RunDir, "runs", run.RunID, "app-server", "process.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := AtomicWrite(path, mustJSON(appServerProcessReceipt{Endpoint: execution.AppServerEndpoint, Generation: execution.AppServerGeneration, Identity: *identity})); err != nil {
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

func TestAppServerServiceTwoTurnsPersistNativeReceipts(t *testing.T) {
	service, run := appServerAuditService(t)
	workspace := t.TempDir()
	state := &mockAppServerState{cwd: workspace}
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
	_, execution := service.minimalRecords(run)
	execution.Process, run.Process = identity, identity
	execution.AppServerEndpoint, execution.AppServerGeneration = "ws://"+listener.Addr().String(), "generation"
	execution.Workspace.Worktree, run.WorktreePath = workspace, &workspace
	service.Config.Sandbox = "read-only"
	service.Config.Codex.Executable = os.Args[0]
	path := filepath.Join(service.Config.RunDir, "runs", run.RunID, "app-server", "process.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := AtomicWrite(path, mustJSON(appServerProcessReceipt{Endpoint: execution.AppServerEndpoint, Generation: execution.AppServerGeneration, Identity: *identity})); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	previousTurn := ""
	for i := 1; i <= 2; i++ {
		commandID := newID()
		if err := service.startWorkerFlowTurn(ctx, run, "complete this turn", commandID); err != nil {
			t.Fatal(err)
		}
		worker := service.workers[run.RunID]
		if worker == nil {
			t.Fatalf("no observer after start: %s", deref(run.Error))
		}
		service.active, service.worker = run, worker
		for {
			select {
			case <-ctx.Done():
				t.Fatal("service observer did not complete")
			case event, ok := <-worker.Events:
				if !ok {
					goto finished
				}
				if err := service.event(event); err != nil {
					t.Fatal(err)
				}
			}
		}
	finished:
		if err := <-worker.Done; err != nil {
			t.Fatal(err)
		}
		delete(service.workers, run.RunID)
		if err := service.finishWorkerFlow(nil); err != nil {
			t.Fatal(err)
		}
		service.active, service.worker = nil, nil
		_, execution = service.minimalRecords(run)
		if execution.TurnNumber != uint64(i) || execution.CurrentCommandID != commandID || execution.CurrentTurnID == previousTurn || execution.CurrentTurnID != execution.LastStoppedTurnID || execution.SessionID != workerFlowMockSession || execution.Status != "idle" || run.Decision == nil {
			t.Fatalf("turn %d receipt was not persisted: %+v", i, execution)
		}
		previousTurn = execution.CurrentTurnID
	}
}

func TestAppServerAnswerRecoveryPersistsBeforeNextQuestion(t *testing.T) {
	service, run := appServerAuditService(t)
	workflow, execution := service.minimalRecords(run)
	execution.SessionID, execution.CurrentTurnID, execution.CurrentCommandID = workerFlowMockSession, "00000000-0000-0000-0000-000000000001", newID()
	execution.Status, workflow.Phase = "active", "waiting_decision"
	run.FinishTaskStatus, run.ExecutionStatus = "waiting_decision", "running"
	run.SessionID = ptr(execution.SessionID)
	ask := codexAskCall{RequestID: json.RawMessage(`0`), ThreadID: execution.SessionID, TurnID: execution.CurrentTurnID, CallID: "first", Generation: "generation", Questions: []codexAskQuestion{{ID: "choice", Header: "Choice", Question: "Continue?", AllowFreeText: true}}}
	var err error
	run.Decision, err = appServerAskDecision(run, ask)
	if err != nil {
		t.Fatal(err)
	}
	run.Decision.Status, run.Decision.Answers, run.Decision.AnsweredBy = "answer_saved", map[string][]string{"choice": {"A"}}, "tester"
	run.Decision.AnsweredAt = ptr(now())
	firstID := run.Decision.ID
	if _, err := service.startOperation("reply_appserver_question", "appserver-answer:"+execution.ExecutionID+":"+firstID, map[string]string{"call": "first"}); err != nil {
		t.Fatal(err)
	}
	workflow, execution = service.minimalRecords(run)
	next := ask
	next.CallID, next.RequestID = "second", json.RawMessage(`1`)
	execution.PendingAsks = []codexAskCall{next}
	path := filepath.Join(service.Config.RunDir, "runs", run.RunID, "turns", execution.CurrentCommandID, "appserver.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	event := codexAppEnvelope{Method: "item/completed", Params: mustJSON(map[string]any{"threadId": ask.ThreadID, "turnId": ask.TurnID, "item": map[string]any{"id": ask.CallID, "type": "dynamicToolCall", "status": "completed", "success": true, "contentItems": []any{map[string]string{"type": "inputText", "text": `{"answers":{"choice":["A"]}}`}}}})}
	if err := AtomicWrite(path, append(mustJSON(event), '\n')); err != nil {
		t.Fatal(err)
	}
	if err := service.reconcileAppServerReplyFromJournal(run, workflow, execution); err != nil {
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
	if !found || run.Decision.Request.NativeAsk.CallID != "second" || len(snapshot.Executions[0].PendingAsks) != 0 {
		t.Fatalf("original acceptance lost while advancing questions: %+v", snapshot.Decisions)
	}
}

func TestAppServerOutcomeUsesNativeTurnAndEarlierPRItem(t *testing.T) {
	thread, turn := "00000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000002"
	path := filepath.Join(t.TempDir(), "appserver.jsonl")
	events := []codexAppEnvelope{
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
	outcome, err := readAppServerTurnOutcome(path, thread, turn)
	if err != nil || !outcome.Started || !outcome.Completed || outcome.NativeStatus != "completed" || outcome.LastMessage != "Done" || len(outcome.PRCandidates) != 1 || outcome.PRCandidates[0] != "https://git.example/o/r/pulls/8" {
		t.Fatalf("native outcome lost evidence: %+v %v", outcome, err)
	}
}

func TestNativeDecisionRequiresAllQuestionIDs(t *testing.T) {
	ask := codexAskCall{RequestID: json.RawMessage(`0`), ThreadID: "00000000-0000-0000-0000-000000000001", TurnID: "00000000-0000-0000-0000-000000000002", CallID: "call", Generation: "generation", Questions: []codexAskQuestion{{ID: "one", Header: "One", Question: "First?", Options: []DecisionOption{{ID: "a", Label: "A"}}}, {ID: "two", Header: "Two", Question: "Second?", AllowFreeText: true}}}
	request := DecisionRequest{Question: "Two questions", AllowFreeText: true, NativeAsk: &ask}
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

func TestAppServerRecoveryReplaysOnlyUnresolvedQuestion(t *testing.T) {
	thread, turn := "00000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000002"
	path := filepath.Join(t.TempDir(), "appserver.jsonl")
	makeCall := func(id string) codexAppEnvelope {
		return codexAppEnvelope{ID: json.RawMessage(`0`), Method: "item/tool/call", Params: mustJSON(map[string]any{"threadId": thread, "turnId": turn, "callId": id, "tool": codexAskToolName, "arguments": map[string]any{"questions": []any{map[string]any{"id": "choice", "header": "Choice", "question": "Continue?", "options": []any{map[string]any{"id": "yes", "label": "Yes"}}, "allow_free_text": false}}}})}
	}
	events := []codexAppEnvelope{makeCall("resolved"), {Method: "item/completed", Params: mustJSON(map[string]any{"threadId": thread, "turnId": turn, "item": map[string]any{"type": "dynamicToolCall", "id": "resolved"}})}, makeCall("pending")}
	var raw []byte
	for _, event := range events {
		raw = append(raw, append(mustJSON(event), '\n')...)
	}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	run := &Run{}
	execution := &ExecutionRecord{SessionID: thread}
	replay, err := appServerPendingAskEvents(path, run, execution, turn)
	if err != nil || len(replay) != 1 {
		t.Fatalf("unresolved request not recovered: %d %v", len(replay), err)
	}
	call, err := parseCodexAskCall(replay[0], thread, turn)
	if err != nil || call.CallID != "pending" {
		t.Fatalf("wrong native request replayed: %+v %v", call, err)
	}
}

func TestAppServerAnswerEvidenceRequiresMatchingNativeContent(t *testing.T) {
	thread, turn := "00000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000002"
	path := filepath.Join(t.TempDir(), "appserver.jsonl")
	call := codexAskCall{ThreadID: thread, TurnID: turn, CallID: "call-1"}
	completed := codexAppEnvelope{Method: "item/completed", Params: mustJSON(map[string]any{"threadId": thread, "turnId": turn, "item": map[string]any{"id": call.CallID, "type": "dynamicToolCall", "status": "completed", "success": true, "contentItems": []any{map[string]string{"type": "inputText", "text": `{"answers":{"choice":["A"]}}`}}}})}
	if err := os.WriteFile(path, append(mustJSON(completed), '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	matched, err := appServerAnswerEvidence(path, call, map[string][]string{"choice": {"A"}})
	if err != nil || !matched {
		t.Fatalf("matching reply was lost: %v %v", matched, err)
	}
	matched, err = appServerAnswerEvidence(path, call, map[string][]string{"choice": {"B"}})
	if err != nil || matched {
		t.Fatalf("wrong answer was accepted: %v %v", matched, err)
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
	client, err := connectWorkerAppServer(ctx, endpoint)
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
	if err := client.Call(ctx, "thread/start", map[string]any{"cwd": workspace, "dynamicTools": []any{codexAskDynamicTool()}}, &started); err != nil {
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
	if started.Thread.ID != workerFlowMockSession || !codexThreadID.MatchString(turn.Turn.ID) || turn.Turn.ID == started.Thread.ID {
		t.Fatalf("mock native identities are invalid: %s %s", started.Thread.ID, turn.Turn.ID)
	}
	for {
		select {
		case <-ctx.Done():
			t.Fatal("mock turn did not complete")
		case event := <-client.notifications:
			if event.Method == "turn/completed" {
				return
			}
		}
	}
}
