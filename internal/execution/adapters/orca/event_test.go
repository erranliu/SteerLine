package orcaworker

import (
	"encoding/json"
	"reflect"
	"testing"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

func TestCD62NativeFactsAndQuestionRoute(t *testing.T) {
	fixed := executionbackend.SessionContext{ProjectID: "project", WorkflowRunID: "workflow", ExecutionID: "execution", AttemptID: "attempt",
		TaskID: "task-original", Target: executionbackend.ExecutionTarget{OrchestrationRun: "run-original"}, Generation: "incarnation-original", CurrentTurnID: "turn-original"}
	t.Run("TestNativeOrcaRejectsLateDispatchMessages", func(t *testing.T) {
		testCD62OrcaLateDispatch(t, fixed)
	})
	t.Run("replayed_question_matches_normalized_saved_content", func(t *testing.T) {
		message := Message{ID: "question", FromHandle: "dispatch:ctx-original", Type: "question", Payload: json.RawMessage(`{"taskId":"task-original","dispatchId":"ctx-original","question":"  Which path?  ","options":["  Direct  "]}`)}
		first, matched, err := DecodeMessage(message, "task-original", "ctx-original", "term-original", fixed)
		if err != nil || !matched || len(first.Questions) != 1 {
			t.Fatalf("first question was not decoded: %+v / %v", first, err)
		}
		saved := first.Questions[0]
		content := executionmodel.DecisionRequest{Question: saved.Question, Options: append([]executionbackend.DecisionOption(nil), saved.Options...), AllowFreeText: saved.AllowFreeText}
		if err := executionmodel.NormalizeDecisionContent(&content); err != nil {
			t.Fatal(err)
		}
		saved.Question, saved.Options = content.Question, content.Options
		replayed, _, err := DecodeMessage(message, "task-original", "ctx-original", "term-original", fixed)
		if err != nil || !reflect.DeepEqual(saved, replayed.Questions[0]) {
			t.Fatalf("same original question conflicts with its normalized persisted content: saved=%+v replay=%+v error=%v", saved, replayed.Questions, err)
		}
	})
	message := Message{ID: "question-original", FromHandle: "dispatch:ctx-original", Type: "question", Body: "Fallback question", Payload: json.RawMessage(`{"taskId":"task-original","dispatchId":"ctx-original","question":"Which path?","options":["Direct"]}`)}
	fact, matched, err := DecodeMessage(message, "task-original", "ctx-original", "term-original", fixed)
	if err != nil || !matched || len(fact.Questions) != 1 || fact.Questions[0].Question != "Which path?" || fact.Questions[0].ID != message.ID || !fact.Questions[0].AllowFreeText || fact.QuestionRoute == nil || fact.QuestionRoute.QuestionID != message.ID || len(fact.Questions[0].Options) != 1 || fact.Questions[0].Options[0].ID != "option-1" {
		t.Fatalf("structured question was lost: %+v / %v", fact, err)
	}
	route, err := DecodeQuestionRoute(*fact.QuestionRoute, fixed, "task-original", "ctx-original", "term-original")
	if err != nil || route.RunID != "run-original" || route.TaskID != "task-original" || route.DispatchID != "ctx-original" || route.MessageID != message.ID {
		t.Fatalf("question route lost its original recipient: %+v / %v", route, err)
	}
	unknownCodec := fact.QuestionRoute.Clone()
	unknownCodec.CodecVersion++
	if _, err := DecodeQuestionRoute(unknownCodec, fixed, "task-original", "ctx-original", "term-original"); err == nil {
		t.Fatal("unknown inbox route codec version was accepted")
	}
	message.Payload = json.RawMessage(`{"taskId":"task-original","dispatchId":"ctx-original"}`)
	fact, matched, err = DecodeMessage(message, "task-original", "ctx-original", "term-original", fixed)
	if err != nil || !matched || len(fact.Questions) != 1 || fact.Questions[0].Question != "Fallback question" {
		t.Fatalf("question fallback was lost: %+v / %v", fact, err)
	}
}

func testCD62OrcaLateDispatch(t *testing.T, fixed executionbackend.SessionContext) {
	const payload = `{"taskId":"task-original","dispatchId":"ctx-original","outcome":"failed"}`
	encoded, _ := json.Marshal(payload)
	for _, raw := range []json.RawMessage{json.RawMessage(payload), encoded} {
		message := Message{ID: "message-original", FromHandle: "term-original", Type: "worker_done", Payload: raw}
		fact, matched, err := DecodeMessage(message, "task-original", "ctx-original", "term-original", fixed)
		if err != nil || !matched || fact.SourceEventID != message.ID || fact.Outcome != "failed" {
			t.Fatalf("original Dispatch lost its outcome: %+v / %v", fact, err)
		}
		if _, matched, err := DecodeMessage(message, "task-next", "ctx-next", "term-original", fixed); err != nil || matched {
			t.Fatalf("old Dispatch advanced another execution: matched=%v / %v", matched, err)
		}
		if _, matched, err := DecodeMessage(message, "task-original", "ctx-original", "term-replacement", fixed); err != nil || matched {
			t.Fatalf("unowned terminal supplied evidence: matched=%v / %v", matched, err)
		}
	}
}
