package codexworker

import (
	"encoding/json"
	"testing"

	"github.com/erranliu/SteerLine/internal/steerline/codexapp"
)

func TestDecodeEventKeepsAskAndReplyOnOriginalTurn(t *testing.T) {
	thread, turn := "00000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000002"
	params, _ := json.Marshal(map[string]any{"threadId": thread, "turnId": turn, "callId": "call-original", "tool": codexapp.AskToolName, "arguments": map[string]any{"questions": []any{map[string]string{"id": "choice", "question": "Which path?"}}}})
	fact, err := DecodeEvent(codexapp.Envelope{ID: json.RawMessage(`17`), Method: "item/tool/call", Params: params}, thread, turn, false)
	if err != nil || fact.Question == nil || fact.Question.ThreadID != thread || fact.Question.TurnID != turn || fact.Question.CallID != "call-original" || string(fact.Question.RequestID) != "17" {
		t.Fatalf("ask lost its original request: %+v / %v", fact, err)
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
			fact, err := DecodeEvent(codexapp.Envelope{Method: "item/completed", Params: params}, thread, turn, true)
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
}
