package orcaworker

import (
	"encoding/json"
	"testing"
)

func TestDecodeMessageBindsPayloadVariantsToOriginalDispatch(t *testing.T) {
	const payload = `{"taskId":"task-original","dispatchId":"ctx-original","outcome":"failed"}`
	encoded, _ := json.Marshal(payload)
	for _, raw := range []json.RawMessage{json.RawMessage(payload), encoded} {
		message := Message{ID: "message-original", FromHandle: "term-original", Type: "worker_done", Payload: raw}
		fact, matched, err := DecodeMessage(message, "task-original", "ctx-original", "term-original")
		if err != nil || !matched || fact.MessageID != message.ID || fact.Outcome != "failed" {
			t.Fatalf("original Dispatch lost its outcome: %+v / %v", fact, err)
		}
		if _, matched, err := DecodeMessage(message, "task-next", "ctx-next", "term-original"); err != nil || matched {
			t.Fatalf("old Dispatch advanced another execution: matched=%v / %v", matched, err)
		}
		if _, matched, err := DecodeMessage(message, "task-original", "ctx-original", "term-replacement"); err != nil || matched {
			t.Fatalf("unowned terminal supplied evidence: matched=%v / %v", matched, err)
		}
	}
	message := Message{ID: "question-original", FromHandle: "dispatch:ctx-original", Type: "question", Body: "Fallback question", Payload: json.RawMessage(`{"taskId":"task-original","dispatchId":"ctx-original","question":"Which path?","options":["Direct"]}`)}
	fact, matched, err := DecodeMessage(message, "task-original", "ctx-original", "term-original")
	if err != nil || !matched || fact.Question == nil || fact.Question.Text != "Which path?" || len(fact.Question.Options) != 1 || fact.Question.Options[0].ID != "option-1" {
		t.Fatalf("structured question was lost: %+v / %v", fact, err)
	}
	message.Payload = json.RawMessage(`{"taskId":"task-original","dispatchId":"ctx-original"}`)
	fact, matched, err = DecodeMessage(message, "task-original", "ctx-original", "term-original")
	if err != nil || !matched || fact.Question == nil || fact.Question.Text != "Fallback question" {
		t.Fatalf("question fallback was lost: %+v / %v", fact, err)
	}
}
