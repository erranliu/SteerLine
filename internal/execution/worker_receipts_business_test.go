package execution

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	resourceprocess "github.com/erranliu/SteerLine/internal/execution/process"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"

	"github.com/erranliu/SteerLine/internal/execution/reconstruction"
)

func TestAppServerStartReplayPreservesNativeSavedAnswer(t *testing.T) {
	first, second := "00000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000002"
	commandID := newID()
	run := &reconstruction.RunView{RunID: newID(), ExecutionID: newID(), AttemptID: newID(), Decision: &executionmodel.DecisionState{Status: "answer_saved", Request: executionmodel.DecisionRequest{FormatVersion: executionmodel.DecisionRequestFormatVersion, Question: "Continue?", AllowFreeText: true}}}
	workflow := &executionmodel.WorkflowRunRecord{WorkflowRunID: run.RunID, WorkflowVersion: executionmodel.Version, Phase: "waiting_decision"}
	execution := &executionmodel.ExecutionRecord{ExecutionID: run.ExecutionID, WorkflowRunID: run.RunID, CurrentAttemptID: run.AttemptID, CurrentStep: "worker", CurrentTurnID: first, LastStoppedTurnID: first, PendingTurnID: second, PendingCommandID: commandID, TurnNumber: 1}
	setNeutralBinding(t, execution, "ws://127.0.0.1:12345", "generation", workerFlowMockSession)
	service := &Service{domain: reconstruction.Snapshot{Operations: []executionmodel.OperationRecord{{OperationID: newID(), IdempotencyKey: "worker-turn:" + run.ExecutionID + ":" + commandID, Status: "outcome_unknown"}}}}
	service.Config.identity = "test-project"
	run.TaskID, run.DomainTaskID, workflow.TaskID = "task", "task", "task"
	service.domain.Tasks = []taskmodel.TaskRecord{{TaskID: "task", Status: "in_progress"}}
	service.domain.Attempts = []executionmodel.StepAttemptRecord{{ExecutionID: run.ExecutionID, AttemptID: run.AttemptID, StepID: "worker", AttemptNumber: 1}}
	service.domain.WorkflowRuns, service.domain.Executions = []executionmodel.WorkflowRunRecord{*workflow}, []executionmodel.ExecutionRecord{*execution}
	workflow, execution = &service.domain.WorkflowRuns[0], &service.domain.Executions[0]
	originalReceipt := mustJSON(executionbackend.LifecycleReceipt{Operation: executionbackend.Operation{Receipt: []byte(`{"launch":{"effective":{"agent":"original-agent"}}}`)}, State: executionbackend.LifecycleStateFacts{InputAccepted: true, SessionID: workerFlowMockSession, TurnID: second}})
	service.domain.Operations[0].Receipt = append([]byte(nil), originalReceipt...)
	service.domain.Operations[0].PlatformRequestID = "original-native-request"
	run.Decision.ID, run.Decision.Version = newID(), 1
	run.Decision.AttemptID = run.AttemptID
	service.setRunDecision(run, run.Decision)
	if _, err := service.recordWorkerTurnStarted(run, workflow, execution, second); err != nil {
		t.Fatal(err)
	}
	if execution.CurrentCommandID != commandID || execution.CurrentTurnID != second || execution.PendingCommandID != "" || execution.TurnNumber != 2 || run.Decision.Status != "reply_accepted" || service.domain.Operations[0].Status != "confirmed" {
		t.Fatalf("second turn receipt was not applied: execution=%+v decision=%+v", execution, run.Decision)
	}
	if !bytes.Equal(service.domain.Operations[0].Receipt, originalReceipt) || service.domain.Operations[0].PlatformRequestID != "original-native-request" {
		t.Fatal("started confirmation replaced the original accepted/native receipt")
	}
	fixed := executionbackend.SessionContext{ProjectID: execution.ProjectIdentity, WorkflowRunID: run.RunID, ExecutionID: run.ExecutionID, AttemptID: run.AttemptID, SessionID: workerFlowMockSession, Generation: execution.SessionGeneration, CurrentTurnID: second}
	request := neutralDecisionFixture(t, fixed, "pending", []executionbackend.Question{{ID: "choice", Question: "Continue?", AllowFreeText: true}})
	service.setRunDecision(run, &executionmodel.DecisionState{ID: newID(), AttemptID: run.AttemptID, Version: 1, Status: "answer_saved", Request: request})
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
	run := &reconstruction.RunView{RunID: newID(), ExecutionID: newID(), DomainTaskID: "8", Protocol: string(neutralProtocol), WorkflowVersion: executionmodel.Version}
	service := &Service{lifecycleEffects: map[string]pendingLifecycleEffect{newID(): {runID: run.RunID}}, State: reconstruction.Runtime{}, domain: reconstruction.Snapshot{Tasks: []taskmodel.TaskRecord{{TaskID: "8", Status: "in_progress"}},
		WorkflowRuns: []executionmodel.WorkflowRunRecord{{WorkflowRunID: run.RunID, WorkflowVersion: executionmodel.Version, Phase: "queued"}},
		Executions:   []executionmodel.ExecutionRecord{{ExecutionID: run.ExecutionID, WorkflowRunID: run.RunID, PendingCommandID: newID()}}}, runs: []*reconstruction.RunView{run},
	}
	before := digestValue(struct {
		View   reconstruction.Runtime
		Domain reconstruction.Snapshot
	}{service.State, service.domain})
	if err := service.resumeWorkerObservers(context.Background()); err != nil {
		t.Fatal(err)
	}
	if before != digestValue(struct {
		View   reconstruction.Runtime
		Domain reconstruction.Snapshot
	}{service.State, service.domain}) {
		t.Fatal("recovery modified a still-running launch before its receipt")
	}
}

func TestAppServerLateReplyCannotModifyTerminalTask(t *testing.T) {
	for _, status := range []string{"closed", "discarded"} {
		route := simpleQuestionRoute(t, neutralQuestionKey, "project", newID(), newID(), newID(), "generation", "turn", "original")
		run := &reconstruction.RunView{RunID: newID(), TaskStatus: status, Decision: &executionmodel.DecisionState{ID: newID(), Status: "answer_saved", Request: executionmodel.DecisionRequest{QuestionRoute: &route}}}
		service := &Service{State: reconstruction.Runtime{}, domain: reconstruction.Snapshot{Operations: []executionmodel.OperationRecord{{OperationID: "reply", Status: "in_flight"}}}, runs: []*reconstruction.RunView{run}}
		before := digestValue(struct {
			View   reconstruction.Runtime
			Domain reconstruction.Snapshot
		}{service.State, service.domain})
		late := lifecycleEffectResult{request: executionbackend.LifecycleRequest{Operation: executionbackend.Operation{ID: "reply", Action: string(executionbackend.LifecycleDeliverFeedback)}, Input: executionbackend.LifecycleInput{Feedback: &executionbackend.FeedbackInput{DecisionID: run.Decision.ID, QuestionID: "original"}}}}
		service.lifecycleEffects = map[string]pendingLifecycleEffect{"reply": {runID: run.RunID, apply: func(*reconstruction.RunView, lifecycleEffectResult) error {
			t.Fatal("terminal task consumed a late receipt")
			return nil
		}}}
		if err := service.handleWorkerMessage(Signal{runID: run.RunID, effect: &late}); !errors.Is(err, executionmodel.ErrTurnStale) {
			t.Fatalf("late reply admission: %v", err)
		}
		if before != digestValue(struct {
			View   reconstruction.Runtime
			Domain reconstruction.Snapshot
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

func TestAppServerUnknownTurnDoesNotOfferPostTurnFeedback(t *testing.T) {
	service, run := neutralAuditService(t)
	workflow, execution := service.workflowRecords(run)
	execution.CurrentTurnID, execution.CurrentCommandID = "00000000-0000-0000-0000-000000000001", newID()
	setNeutralBinding(t, execution, "ws://127.0.0.1:12345", "generation", workerFlowMockSession)
	execution.PolicyResources = neutralPolicyReceipt(workflow.ExecutionProfile.Digest, execution.Workspace.Worktree, execution)
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
	result := appServerRecoveryFixture(t, service, run)
	result.worker = worker
	if err := service.applyWorkerRecovery(result); err != nil {
		t.Fatal(err)
	}
	workflow, execution = service.workflowRecords(run)
	if execution.Status != "active" || run.ExecutionStatus != "running" || workflow.Phase != "executing" || run.Error != nil {
		t.Fatalf("recovered active turn retained its blocked state: %+v %+v", execution, run)
	}
	delete(service.workers, run.RunID)
}

func TestAppServerSlowRecoveryLeavesServiceWriterAvailable(t *testing.T) {
	service, run := neutralAuditService(t)
	workflow, execution := service.workflowRecords(run)
	execution.PolicyResources = neutralPolicyReceipt(workflow.ExecutionProfile.Digest, execution.Workspace.Worktree, execution)
	identity, err := resourceprocess.CurrentIdentity()
	if err != nil {
		t.Fatal(err)
	}

	execution.Process, run.Process = identity, identity
	endpoint := "fixture-endpoint"
	setNeutralBinding(t, execution, endpoint, "generation", workerFlowMockSession)
	execution.PolicyResources = neutralPolicyReceipt(workflow.ExecutionProfile.Digest, execution.Workspace.Worktree, execution)
	execution.CurrentTurnID, execution.CurrentCommandID = "00000000-0000-0000-0000-000000000001", newID()
	execution.Status, workflow.Phase = "active", "executing"
	input := executionbackend.TurnInput{CommandID: execution.CurrentCommandID, Prompt: "original persisted input"}
	if _, err := service.startWorkerOperation(run, "start_worker_turn", "worker-turn:"+execution.ExecutionID+":"+input.CommandID, &input); err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	service.lifecycleResolver = neutralReceiptResolver{entered: entered}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service.runContext, service.signalQueue, service.State.Service.Ready = ctx, make(chan Signal, 4), true
	started := time.Now()
	if err := service.resumeWorkerObservers(ctx); err != nil {
		t.Fatal(err)
	}
	if time.Since(started) > time.Second || !service.workerRecovering[run.RunID] {
		t.Fatal("slow recovery blocked the service writer")
	}
	if err := service.resumeWorkerObservers(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("recovery probe did not reach the injected read-only port")
	}

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
	service, run := neutralAuditService(t)
	workflow, execution := service.workflowRecords(run)
	execution.CurrentTurnID, execution.CurrentCommandID = "00000000-0000-0000-0000-000000000001", newID()
	execution.Status, workflow.Phase = "active", "waiting_decision"
	run.FinishTaskStatus, run.ExecutionStatus = "waiting_decision", "running"
	setNeutralBinding(t, execution, "ws://127.0.0.1:12345", "generation", workerFlowMockSession)
	execution.PolicyResources = neutralPolicyReceipt(workflow.ExecutionProfile.Digest, execution.Workspace.Worktree, execution)
	execution.CurrentCommandID = newID()
	execution.CurrentTurnID = "00000000-0000-0000-0000-000000000003"
	fixed := *workerSessionContext(run, execution)
	fixed.ProjectID = service.Config.identity
	ask := neutralDecisionFixture(t, fixed, "first", []executionbackend.Question{{ID: "choice", Header: "Choice", Question: "Continue?", AllowFreeText: true}})
	var err error
	run.Decision, err = WriteDecisionRequest(run, ask)
	if err != nil {
		t.Fatal(err)
	}
	service.setRunDecision(run, run.Decision)
	record := service.decisionRecord(run)
	record.Status, record.AnsweredBy = "answer_saved", "tester"
	acceptedAnswer := executionmodel.DecisionAnswer{DecisionID: record.DecisionID, RunID: run.RunID, Answers: map[string][]string{"choice": {"A"}}, AnsweredBy: "tester", AnsweredAt: now()}
	record.Answer = mustJSON(acceptedAnswer)
	service.refreshRun(run)
	firstID := run.Decision.ID
	feedback := &executionbackend.FeedbackInput{DecisionID: firstID, QuestionID: ask.QuestionRoute.QuestionID, Answer: &executionbackend.Answer{Answers: acceptedAnswer.Answers}}
	if _, err := service.startWorkerOperation(run, "reply_execution_question", "worker-question-answer:"+execution.ExecutionID+":"+firstID, feedback); err != nil {
		t.Fatal(err)
	}
	workflow, execution = service.workflowRecords(run)
	fixed = *workerSessionContext(run, execution)
	next := neutralDecisionFixture(t, fixed, "second", []executionbackend.Question{{ID: "choice", Header: "Choice", Question: "Continue?", AllowFreeText: true}})
	execution.PendingQuestions = []executionmodel.PendingQuestion{{Questions: next.Questions, Route: *next.QuestionRoute}}
	service.lifecycleResolver = neutralReceiptResolver{confirmedAnswer: true}

	if err := service.reconcileSavedLifecycleQuestionAnswer(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	snapshot, err := readServiceSnapshot(context.Background(), service.domainStore, service.Config.identity)
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

func TestRoutedDecisionRequiresAllQuestionIDs(t *testing.T) {
	route := simpleQuestionRoute(t, "example.implementation", "project", "workflow", "execution", "attempt", "generation", "turn", "native-question")
	questions := []executionbackend.Question{
		{ExecutionID: "execution", AttemptID: "attempt", Generation: "generation", TurnID: "turn", ID: "one", Header: "One", Question: "First?", Options: []executionbackend.DecisionOption{{ID: "a", Label: "A"}}},
		{ExecutionID: "execution", AttemptID: "attempt", Generation: "generation", TurnID: "turn", ID: "two", Header: "Two", Question: "Second?", AllowFreeText: true},
	}
	request := executionmodel.DecisionRequest{FormatVersion: 2, Question: "Two questions", AllowFreeText: true, Questions: questions, QuestionRoute: &route}
	answer := executionmodel.DecisionAnswer{AnsweredBy: "tester", Note: `{"one":["A"],"two":["scope"]}`}
	if err := NormalizeNativeDecisionAnswer(&request, &answer); err != nil {
		t.Fatal(err)
	}
	if len(answer.Answers) != 2 {
		t.Fatal("native answers lost a question")
	}
	delete(answer.Answers, "two")
	if err := executionmodel.ValidateDecisionAnswer(&request, &answer); err == nil {
		t.Fatal("missing second answer was accepted")
	}
}

func TestAppServerStartedEventCommitsOperationReceipt(t *testing.T) {
	service, run := neutralAuditService(t)
	_, execution := service.workflowRecords(run)
	execution.PolicyResources = neutralPolicyReceipt(service.workflowRecord(run).ExecutionProfile.Digest, execution.Workspace.Worktree, execution)
	commandID, turnID := newID(), "00000000-0000-0000-0000-000000000001"
	setNeutralBinding(t, execution, "ws://127.0.0.1:12345", "generation", workerFlowMockSession)
	execution.PendingCommandID, execution.PendingTurnID = commandID, turnID
	opID, err := service.startWorkerOperation(run, "start_worker_turn", "worker-turn:"+run.ExecutionID+":"+commandID, map[string]string{"command_id": commandID})
	require(t, err)
	event := WorkerEvent{Fact: executionbackend.EventFact{Kind: executionbackend.EventStarted, SessionID: workerFlowMockSession, StartedTurnID: turnID}}
	require(t, service.eventWorkerFlowFor(run, event))
	snapshot, err := readServiceSnapshot(context.Background(), service.domainStore, service.Config.identity)
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
