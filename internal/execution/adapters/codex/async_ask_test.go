package codexworker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/erranliu/SteerLine/internal/execution/adapters/protocol/codexapp"
	b "github.com/erranliu/SteerLine/internal/execution/backend"
)

func TestAsyncQuestionNativeIdentityOptionsReplayAndFinal(t *testing.T) {
	thread, turn := "00000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000002"
	params := mustJSONTest(map[string]any{"threadId": thread, "turnId": turn, "item": map[string]any{"type": "agentMessage", "id": "native-item", "phase": "final_answer", "delivery": "async", "text": "choose", "questions": []any{map[string]any{"title": "Publisher?", "options": []string{"worker", "host_service"}}, map[string]any{"title": "Scope?"}}}})
	event := codexapp.Envelope{Method: "item/completed", Params: params}
	fixed := b.SessionContext{ProjectID: "project", WorkflowRunID: "run", ExecutionID: "exec", AttemptID: "attempt", Generation: "generation", SessionID: thread, CurrentTurnID: turn, CurrentCommandID: "00000000000000000000000000000001"}
	fact, err := DecodeEvent(event, fixed, true)
	if err != nil || fact.InteractionKind != "async" || len(fact.Questions) != 2 || fact.Questions[0].Options[1].Label != "host_service" || fact.QuestionRoute.QuestionID != "native-item" {
		t.Fatalf("lost native question: %+v %v", fact, err)
	}
	route, err := DecodeQuestionRoute(*fact.QuestionRoute, fixed)
	if err != nil || route.Delivery != "async" || len(route.RequestID) != 0 {
		t.Fatalf("async route used tool reply: %+v %v", route, err)
	}
	fixed.CurrentTurnID = thread
	if _, err := DecodeEvent(event, fixed, true); err == nil {
		t.Fatal("foreign turn accepted")
	}
	path := filepath.Join(t.TempDir(), "appserver.jsonl")
	raw, _ := json.Marshal(event)
	if err := os.WriteFile(path, append(raw, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	pending, err := PendingAskEvents(path, thread, turn, nil)
	if err != nil || len(pending) != 1 {
		t.Fatalf("question was not replayed: %v %v", pending, err)
	}
	pending, err = PendingAskEvents(path, thread, turn, []string{"native-item"})
	if err != nil || len(pending) != 0 {
		t.Fatal("known question replayed")
	}
	outcome, err := ReadAppServerOutcome(path, thread, turn)
	if err != nil || outcome.FinalVerified || outcome.LastMessage != "" {
		t.Fatalf("async question became a final result: %+v %v", outcome, err)
	}
}

func TestAsyncQuestionObserverUsesNewInputCommand(t *testing.T) {
	snapshot := b.LifecycleSnapshot{Session: b.SessionContext{CurrentCommandID: "old-command", PendingCommandID: "new-command"}}
	fixed := lifecycleSession(snapshot, "thread", "new-turn")
	if fixed.CurrentCommandID != "new-command" || fixed.CurrentTurnID != "new-turn" {
		t.Fatal("question journal bound to previous input")
	}
}
