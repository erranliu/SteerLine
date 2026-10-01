package steerline

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/projection"
)

func TestCD62NativeFactsAndQuestionRoute(t *testing.T) {
	identity := executionapi.QuestionRouteIdentity{ProjectID: "project", WorkflowRunID: "workflow", ExecutionID: "execution", AttemptID: "attempt", Generation: "generation", TurnID: "turn", QuestionID: "receipt-1"}
	route := simpleQuestionRoute(t, "example.third-party", identity.ProjectID, identity.WorkflowRunID, identity.ExecutionID, identity.AttemptID, identity.Generation, identity.TurnID, identity.QuestionID)
	question := executionapi.Question{ExecutionID: identity.ExecutionID, AttemptID: identity.AttemptID, Generation: identity.Generation, TurnID: identity.TurnID,
		ID: "choice", Header: "Choice", Question: "Continue?", Options: []executionapi.DecisionOption{{ID: "yes", Label: "Yes"}}, AllowFreeText: true}
	fact := executionapi.EventFact{Kind: executionapi.EventQuestion, Questions: []executionapi.Question{question}, QuestionRoute: &route}
	request := decisionRequestFromEventFact(fact)
	if err := validateDecisionRequest(&request); err != nil {
		t.Fatal(err)
	}
	run := &Run{RunID: identity.WorkflowRunID, ExecutionID: identity.ExecutionID, AttemptID: identity.AttemptID}
	execution := &ExecutionRecord{ProjectIdentity: identity.ProjectID, WorkflowRunID: identity.WorkflowRunID, ExecutionID: identity.ExecutionID,
		CurrentAttemptID: identity.AttemptID, AppServerGeneration: identity.Generation, CurrentTurnID: identity.TurnID}
	if err := validateQuestionRouteForRun(run, execution, &route, []executionapi.Question{question}, route.ImplementationKey, route.CodecVersion); err != nil {
		t.Fatalf("center rejected a fixed generic question route: %v", err)
	}
	answerRoute := simpleQuestionRoute(t, "example.inbox", identity.ProjectID, identity.WorkflowRunID, identity.ExecutionID, identity.AttemptID, identity.Generation, identity.TurnID, "message-1")
	answerFact := executionapi.EventFact{QuestionRoute: &answerRoute, Questions: []executionapi.Question{{
		ExecutionID: identity.ExecutionID, AttemptID: identity.AttemptID, Generation: identity.Generation, TurnID: identity.TurnID,
		ID: "message-1", Header: "Choice", Question: "Continue?", Options: []executionapi.DecisionOption{{ID: "yes", Label: "Yes"}},
	}}}
	answerRequest := decisionRequestFromEventFact(answerFact)
	answer := DecisionAnswer{AnsweredBy: "tester", OptionID: "yes", Note: "Keep going"}
	if err := normalizeNativeDecisionAnswer(&answerRequest, &answer); err != nil {
		t.Fatal(err)
	}
	if got := nativeOrcaAnswerText(answer, answerRequest); got != "Yes\n\nKeep going" {
		t.Fatalf("common route did not preserve the answer text: %q", got)
	}
	conflict := question
	conflict.Question = "Changed text"
	if err := sameRoutedQuestion(&route, []executionapi.Question{question}, &route, []executionapi.Question{conflict}); err == nil {
		t.Fatal("center accepted conflicting content for a duplicate question identity")
	}
	if err := validateQuestionRouteForRun(run, execution, &route, []executionapi.Question{question}, "different.implementation", 1); err == nil {
		t.Fatal("center accepted a route from another implementation")
	}
	question.TurnID = "old-turn"
	if err := validateQuestionRouteForRun(run, execution, &route, []executionapi.Question{question}, route.ImplementationKey, route.CodecVersion); err == nil {
		t.Fatal("center accepted a question from another turn")
	}
	changed := route.Clone()
	changed.Purpose = "other"
	if err := changed.Validate(); err == nil {
		t.Fatal("center accepted a route with an unverified purpose")
	}
}

func TestCD62QuestionReceiptBarrier(t *testing.T) {
	t.Run("all_question_ids_validated", TestRoutedDecisionRequiresAllQuestionIDs)
	t.Run("legacy_decision_format_is_rejected", func(t *testing.T) {
		legacy := DecisionRecord{ProjectIdentity: "project", DecisionID: "decision", WorkflowRunID: "workflow", ExecutionID: "execution", AttemptID: "attempt", Version: 1, QuestionID: "question", Request: []byte(`{"question":"old","native_ask":{"call_id":"call"}}`)}
		_, err := projection.Run(TaskRecord{TaskID: "task"}, WorkflowRunRecord{WorkflowRunID: "workflow", TaskID: "task", WorkflowVersion: WorkerFlowVersion}, ExecutionRecord{ExecutionID: "execution", WorkflowRunID: "workflow", CurrentAttemptID: "attempt", CurrentStep: "worker", DecisionID: "decision"}, []StepAttemptRecord{{AttemptID: "attempt", ExecutionID: "execution", StepID: "worker"}}, []DecisionRecord{legacy}, nil)
		if err == nil || !strings.Contains(err.Error(), "unsupported format") {
			t.Fatalf("legacy native question data was not rejected by the shared projection: %v", err)
		}
		var execution ExecutionRecord
		if err := json.Unmarshal([]byte(`{"pending_asks":[]}`), &execution); err == nil {
			t.Fatal("legacy pending native questions were silently read")
		}
	})
	t.Run("TestAppServerAnswerRecoveryPersistsBeforeNextQuestion", testAppServerAnswerRecoveryPersistsBeforeNextQuestion)
	t.Run("message_persisted_before_ack", testCD62InboxCommitBeforeAck)
}

func testCD62InboxCommitBeforeAck(t *testing.T) {
	f := newEnvironmentFixture(t, fakeNativeOrcaState{}, "chosen-model")
	f.prepare(t)
	f.poll(t)
	f.poll(t)
	_, execution := f.service.workflowRecords(f.run)
	if nativeOrcaCurrentTurn(execution) == nil {
		t.Fatal("fixture did not create the fixed execution turn")
	}
	_, err := f.service.domainStore.db.Exec(`CREATE TRIGGER reject_question_commit BEFORE UPDATE ON projection BEGIN SELECT RAISE(FAIL, 'question commit failed'); END`)
	require(t, err)
	err = f.service.pollNativeOrcaWorker(context.Background(), f.run)
	if err == nil {
		t.Fatal("question delivery committed despite the injected transaction failure")
	}
	state := f.state(t)
	if state.Acked["delivery-ask-1"] {
		t.Fatal("message acknowledgement ran before question and consumption facts committed")
	}
}
