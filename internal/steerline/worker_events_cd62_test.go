package steerline

import (
	"encoding/json"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/projection"
	"strings"
	"testing"
)

func TestCD62NativeFactsAndQuestionRoute(t *testing.T) {
	t.Run("question_ids_are_scoped_to_the_original_turn", func(t *testing.T) {
		old := simpleQuestionRoute(t, "example.impl", "project", "workflow", "execution", "attempt", "generation", "old-turn", "reused-call")
		current := simpleQuestionRoute(t, "example.impl", "project", "workflow", "execution", "attempt", "generation", "current-turn", "reused-call")
		oldQuestions := []executionapi.Question{{ExecutionID: "execution", AttemptID: "attempt", Generation: "generation", TurnID: "old-turn", ID: "choice", Question: "Previous question", AllowFreeText: true}}
		currentQuestions := cloneExecutionQuestions(oldQuestions)
		currentQuestions[0].TurnID, currentQuestions[0].Question = "current-turn", "New question"
		request := decisionRequestFromEventFact(executionapi.EventFact{QuestionRoute: &old, Questions: oldQuestions})
		s := &Service{domain: DomainSnapshot{Decisions: []DecisionRecord{{ExecutionID: "execution", QuestionID: old.QuestionID, Request: mustJSON(request)}}}}
		seen, err := s.routedQuestionSeen(&Run{}, &ExecutionRecord{ExecutionID: "execution"}, &current, currentQuestions)
		if err != nil || seen {
			t.Fatalf("a call ID from an earlier turn conflicts with a new bound question: seen=%t err=%v", seen, err)
		}
		if _, err := s.routedQuestionSeen(&Run{}, &ExecutionRecord{ExecutionID: "execution"}, &old, currentQuestions); err == nil {
			t.Fatal("changed content for the same original question was accepted")
		}
	})
	t.Run("opaque_started_turn_and_gap", func(t *testing.T) {
		s, run := neutralAuditService(t)
		workflow, record := s.workflowRecords(run)
		commandID := newID()
		record.PolicyResources = neutralPolicyReceipt(workflow.ExecutionProfile.Digest, record.Workspace.Worktree, record)
		record.PendingCommandID, record.PendingTurnID = commandID, "opaque-turn"
		s.refreshRun(run)
		_, err := s.startWorkerOperation(run, "start_worker_turn", "worker-turn:"+record.ExecutionID+":"+commandID, map[string]string{"command_id": commandID})
		require(t, err)
		event := WorkerEvent{RunID: run.RunID, AttemptID: run.AttemptID, Fact: executionapi.EventFact{Kind: executionapi.EventStarted, SessionID: "opaque-session", StartedTurnID: record.PendingTurnID}}
		foreign := event
		foreign.Fact.StartedTurnID = "other-turn"
		if err := s.eventWorkerFlowFor(run, foreign); err == nil || s.executionRecord(run).TurnNumber != 0 {
			t.Fatal("start without the original input receipt advanced a turn")
		}
		require(t, s.eventWorkerFlowFor(run, event))
		workflow, record = s.workflowRecords(run)
		if record.CurrentTurnID != "opaque-turn" || record.PendingCommandID != "" || record.TurnNumber != 1 {
			t.Fatalf("generic start was not accepted against its fixed intent: %+v", record)
		}
		require(t, s.eventWorkerFlowFor(run, event))
		if s.executionRecord(run).TurnNumber != 1 {
			t.Fatal("replayed start counted a second turn")
		}
		event.Fact = executionapi.EventFact{Kind: executionapi.EventGap, Gap: true}
		require(t, s.eventWorkerFlowFor(run, event))
		if s.executionRecord(run).LastStoppedTurnID != "" || s.workflowRecord(run).Phase != workflow.Phase {
			t.Fatal("observation gap was treated as a stopped worker")
		}
	})
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
		CurrentAttemptID: identity.AttemptID, Generation: identity.Generation, CurrentTurnID: identity.TurnID, Protocol: string(neutralProtocol)}
	setNeutralBinding(t, execution, "ws://127.0.0.1:12345", identity.Generation, workerFlowMockSession)
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
	if got := routedAnswerText(answer, answerRequest); got != "Yes\n\nKeep going" {
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
}

func routedAnswerText(answer DecisionAnswer, request DecisionRequest) string {
	if request.QuestionRoute == nil {
		return workerFeedbackText(answer, request)
	}
	return strings.Join(answer.Answers[request.QuestionRoute.QuestionID], "\n\n")
}
