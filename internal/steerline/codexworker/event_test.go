package codexworker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	"github.com/erranliu/SteerLine/internal/steerline/codexapp"
)

func TestDecodeQuestionsKeepsCenterActionsOutOfNativeContent(t *testing.T) {
	for _, actionFields := range []string{
		`"receiver":"center","action":"open_terminal"`,
		`"receiver":42,"action":{"unexpected":true}`,
	} {
		payload := json.RawMessage(`{"questions":[{"id":"choice","header":"Choice","question":"Continue?","allow_free_text":true,"options":[{"id":"yes","label":"Yes","detail":"Keep working",` + actionFields + `}]}]}`)
		questions, err := DecodeQuestions(payload)
		if err != nil || len(questions) != 1 || len(questions[0].Options) != 1 {
			t.Fatalf("unknown native fields changed question decoding: %+v / %v", questions, err)
		}
		option := questions[0].Options[0]
		if option.ID != "yes" || option.Label != "Yes" || option.Detail != "Keep working" || option.Receiver != "" || option.Action != "" {
			t.Fatalf("native content acquired center action authority: %+v", option)
		}
	}
}

func TestCD62NativeFactsAndQuestionRoute(t *testing.T) {
	thread, turn := "00000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000002"
	fixed := executionbackend.SessionContext{ProjectID: "project", WorkflowRunID: "workflow", ExecutionID: "execution", AttemptID: "attempt", SessionID: thread, Generation: "generation", CurrentTurnID: turn}
	params, _ := json.Marshal(map[string]any{"threadId": thread, "turnId": turn, "callId": "call-original", "tool": codexapp.AskToolName, "arguments": map[string]any{"questions": []any{
		map[string]any{"id": "choice", "header": "Choice", "question": "Which path?", "allow_free_text": true, "options": []any{map[string]any{"id": "direct", "label": "Direct"}}},
		map[string]any{"id": "scope", "header": "Scope", "question": "What scope?", "allow_free_text": true},
	}}})
	fact, err := DecodeEvent(codexapp.Envelope{ID: json.RawMessage(`17`), Method: "item/tool/call", Params: params}, fixed, false)
	if err != nil || len(fact.Questions) != 2 || fact.Questions[0].ExecutionID != fixed.ExecutionID || fact.Questions[0].TurnID != turn || fact.QuestionRoute == nil || fact.QuestionRoute.QuestionID != "call-original" || fact.Questions[1].ID != "scope" || len(fact.Questions[0].Options) != 1 {
		t.Fatalf("ask lost its original request: %+v / %v", fact, err)
	}
	route, err := DecodeQuestionRoute(*fact.QuestionRoute, fixed)
	if err != nil || route.ThreadID != thread || route.TurnID != turn || route.CallID != "call-original" || string(route.RequestID) != "17" {
		t.Fatalf("strict question route lost its native request binding: %+v / %v", route, err)
	}
	for _, tc := range []struct {
		name, threadID, turnID string
		valid                  bool
	}{
		{"original", thread, turn, true},
		{"another_thread", "00000000-0000-0000-0000-000000000003", turn, false},
		{"another_turn", thread, "00000000-0000-0000-0000-000000000004", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			params, _ := json.Marshal(map[string]any{"threadId": tc.threadID, "turnId": tc.turnID, "item": map[string]any{"id": "call-original", "type": "dynamicToolCall", "status": "completed", "success": true, "contentItems": []any{map[string]string{"type": "inputText", "text": `{"answers":{"choice":["A"]}}`}}}})
			fact, err := DecodeEvent(codexapp.Envelope{Method: "item/completed", Params: params}, fixed, true)
			if !tc.valid {
				if err == nil || fact.ToolReply != nil {
					t.Fatalf("foreign reply became a verified fact: %+v / %v", fact, err)
				}
				return
			}
			if err != nil || fact.ToolReply == nil || fact.ToolReply.CallID != "call-original" || !fact.ToolReply.MatchesAnswers(map[string][]string{"choice": {"A"}}) || fact.ToolReply.MatchesAnswers(map[string][]string{"choice": {"B"}}) {
				t.Fatalf("original answer evidence was lost: %+v / %v", fact, err)
			}
		})
	}
	changed := fact.QuestionRoute.Clone()
	changed.Payload[0] ^= 1
	if _, err := DecodeQuestionRoute(changed, fixed); err == nil {
		t.Fatal("tampered route payload passed its digest")
	}
	unknownCodec := fact.QuestionRoute.Clone()
	unknownCodec.CodecVersion++
	if _, err := DecodeQuestionRoute(unknownCodec, fixed); err == nil {
		t.Fatal("unknown route codec version was accepted")
	}
	unknownPayload, _ := json.Marshal(map[string]any{"request_id": 17, "thread_id": thread, "turn_id": turn, "call_id": "call-original", "recipient_override": "untrusted"})
	unknownRoute, err := executionbackend.NewQuestionRoute(QuestionRouteImplementationKey, 1, executionbackend.QuestionRouteIdentity{
		ProjectID: fixed.ProjectID, WorkflowRunID: fixed.WorkflowRunID, ExecutionID: fixed.ExecutionID,
		AttemptID: fixed.AttemptID, Generation: fixed.Generation, TurnID: turn, QuestionID: "call-original",
	}, unknownPayload)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeQuestionRoute(unknownRoute, fixed); err == nil {
		t.Fatal("unknown route payload field was accepted")
	}
	finished, err := DecodeEvent(codexapp.Envelope{Method: "turn/completed", Params: json.RawMessage(`{"threadId":"` + thread + `","turn":{"id":"` + turn + `","status":"completed"}}`)}, fixed, true)
	if err != nil || finished.Kind != executionbackend.EventCompleted || !finished.Completed {
		t.Fatalf("current turn completion shape was not represented as a generic fact: %+v / %v", finished, err)
	}
	wrongCompletion, err := DecodeEvent(codexapp.Envelope{Method: "turn/completed", Params: json.RawMessage(`{"threadId":"` + thread + `","turn":{"id":"00000000-0000-0000-0000-000000000003"}}`)}, fixed, true)
	if err == nil || wrongCompletion.Completed {
		t.Fatalf("completion from an old turn was accepted: %+v / %v", wrongCompletion, err)
	}
	t.Run("TestAppServerRecoveryReplaysOnlyUnresolvedQuestion", func(t *testing.T) {
		testCD62JournalReplay(t, thread, turn)
	})
	t.Run("TestAppServerAnswerEvidenceRequiresMatchingNativeContent", func(t *testing.T) {
		testCD62AnswerEvidence(t, thread, turn)
	})
}

func testCD62JournalReplay(t *testing.T, thread, turn string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "appserver.jsonl")
	makeCall := func(id string) codexapp.Envelope {
		return codexapp.Envelope{ID: json.RawMessage(`0`), Method: "item/tool/call", Params: mustJSONTest(map[string]any{"threadId": thread, "turnId": turn, "callId": id, "tool": codexapp.AskToolName, "arguments": map[string]any{"questions": []any{map[string]any{"id": "choice", "header": "Choice", "question": "Continue?", "options": []any{map[string]any{"id": "yes", "label": "Yes"}}, "allow_free_text": false}}}})}
	}
	resolved := makeCall("resolved")
	completedResolved := codexapp.Envelope{Method: "item/completed", Params: mustJSONTest(map[string]any{"threadId": thread, "turnId": turn, "item": map[string]any{"type": "dynamicToolCall", "id": "resolved"}})}
	pending := makeCall("pending")
	var journal []byte
	for _, event := range []codexapp.Envelope{resolved, completedResolved, pending} {
		journal = append(journal, append(mustJSONTest(event), '\n')...)
	}
	if err := os.WriteFile(path, journal, 0600); err != nil {
		t.Fatal(err)
	}
	replay, err := PendingAskEvents(path, thread, turn, nil)
	if err != nil || len(replay) != 1 {
		t.Fatalf("unresolved request was not isolated for replay: %d / %v", len(replay), err)
	}
	call, err := ParseAskCall(replay[0], thread, turn)
	if err != nil || call.Route.CallID != "pending" {
		t.Fatalf("wrong request was replayed: %+v / %v", call, err)
	}
}

func testCD62AnswerEvidence(t *testing.T, thread, turn string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "appserver.jsonl")
	completed := codexapp.Envelope{Method: "item/completed", Params: mustJSONTest(map[string]any{"threadId": thread, "turnId": turn, "item": map[string]any{"id": "pending", "type": "dynamicToolCall", "status": "completed", "success": true, "contentItems": []any{map[string]string{"type": "inputText", "text": `{"answers":{"choice":["A"]}}`}}}})}
	if err := os.WriteFile(path, append(mustJSONTest(completed), '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	if matched, err := AnswerEvidence(path, thread, turn, "pending", map[string][]string{"choice": {"A"}}); err != nil || !matched {
		t.Fatalf("matching reply receipt was lost: %v / %v", matched, err)
	}
	if matched, err := AnswerEvidence(path, thread, turn, "pending", map[string][]string{"choice": {"B"}}); err != nil || matched {
		t.Fatalf("different answer was treated as the receipt: %v / %v", matched, err)
	}
}

func mustJSONTest(value any) []byte {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return data
}
