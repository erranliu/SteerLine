package steerline

import (
	"encoding/json"
	"testing"

	"github.com/erranliu/SteerLine/internal/steerline/codexapp"
	"github.com/erranliu/SteerLine/internal/steerline/workerflow"
)

func TestWorkerProtocolFactsPreserveOriginalNativeIdentity(t *testing.T) {
	thread := "00000000-0000-0000-0000-000000000001"
	turn := "00000000-0000-0000-0000-000000000002"
	execution := &ExecutionRecord{SessionID: thread, CurrentTurnID: turn, AppServerGeneration: "generation-1"}
	started := WorkerEvent{Source: "codex-app-server", SourceEventID: "event-1", SourceIdentity: "journal-1", SourceCursor: "cursor-1", SourceSegment: "segment-1", SourceOffset: 12, SourceLength: 34, Session: thread, AppServer: &codexapp.Envelope{Method: "turn/started", Params: mustJSON(map[string]any{"threadId": thread, "turn": map[string]string{"id": turn}})}}
	fact, err := decodeWorkerFlowEvent(workerflow.LocalCodex, started, workerSessionContext(nil, execution), false)
	if err != nil || fact.StartedTurnID != turn {
		t.Fatalf("original turn start was lost: %+v %v", fact, err)
	}
	if fact.Source != started.Source || fact.SourceEventID != started.SourceEventID || fact.SourceIdentity != started.SourceIdentity || fact.SourceCursor != started.SourceCursor || fact.SourceSegment != started.SourceSegment || fact.SourceOffset != started.SourceOffset || fact.SourceLength != started.SourceLength {
		t.Fatal("common event lost its original evidence reference")
	}
	accepted := WorkerEvent{Session: thread, Activity: ActivityEvent{Kind: "turn", Status: "input_accepted"}}
	for _, mode := range []workerflow.Mode{workerflow.LocalCodex} {
		fact, err := decodeWorkerFlowEvent(mode, accepted, workerSessionContext(nil, execution), false)
		if err != nil || fact.StartedTurnID != "" || fact.Question != nil || fact.ToolReply != nil {
			t.Fatalf("input acceptance became a verified turn start: %+v / %v", fact, err)
		}
	}
	question := started
	question.AppServer = &codexapp.Envelope{ID: json.RawMessage(`17`), Method: "item/tool/call", Params: mustJSON(map[string]any{"threadId": thread, "turnId": turn, "callId": "ask-original", "tool": codexAskToolName, "arguments": map[string]any{"questions": []any{map[string]any{"id": "choice", "header": "Choice", "question": "Continue?", "allow_free_text": true, "options": []any{map[string]string{"id": "yes", "label": "Yes"}}}}}})}
	fact, err = decodeWorkerFlowEvent(workerflow.LocalCodex, question, workerSessionContext(nil, execution), false)
	if err != nil || fact.Question == nil || fact.Question.Generation != "generation-1" || fact.Question.ThreadID != thread || fact.Question.TurnID != turn || string(fact.Question.RequestID) != "17" || len(fact.Question.Questions) != 1 || !fact.Question.Questions[0].AllowFreeText || len(fact.Question.Questions[0].Options) != 1 || fact.SourceEventID != started.SourceEventID {
		t.Fatalf("common question lost its source or original answer route: %+v / %v", fact, err)
	}
	started.AppServer.Params = mustJSON(map[string]any{"threadId": "00000000-0000-0000-0000-000000000003", "turn": map[string]string{"id": turn}})
	if _, err := decodeWorkerFlowEvent(workerflow.LocalCodex, started, workerSessionContext(nil, execution), false); err == nil {
		t.Fatal("replacement app-server thread was accepted")
	}
	completed := WorkerEvent{Session: thread, AppServer: &codexapp.Envelope{Method: "item/completed", Params: mustJSON(map[string]any{"threadId": thread, "turnId": turn, "item": map[string]any{"id": "call-1", "type": "dynamicToolCall", "status": "completed", "success": true, "contentItems": []any{map[string]string{"type": "inputText", "text": `{"answers":{"choice":["A"]}}`}}}})}}
	fact, err = decodeWorkerFlowEvent(workerflow.LocalCodex, completed, workerSessionContext(nil, execution), true)
	if err != nil || fact.ToolReply == nil || !fact.ToolReply.MatchesAnswers(map[string][]string{"choice": {"A"}}) || fact.ToolReply.MatchesAnswers(map[string][]string{"choice": {"B"}}) {
		t.Fatalf("reply fact accepted another answer: %+v %v", fact, err)
	}
	completed.AppServer.Params = mustJSON(map[string]any{"threadId": thread, "turnId": "00000000-0000-0000-0000-000000000004", "item": map[string]string{"id": "call-1", "type": "dynamicToolCall"}})
	if _, err := decodeWorkerFlowEvent(workerflow.LocalCodex, completed, workerSessionContext(nil, execution), true); err == nil {
		t.Fatal("old-turn reply was accepted")
	}
	if _, err := decodeWorkerFlowEvent(workerflow.NativeOrca, completed, workerSessionContext(nil, execution), true); err == nil {
		t.Fatal("native Orca message bypassed its Dispatch adapter")
	}
	if _, err := decodeWorkerFlowEvent(workerflow.SupervisedOrca, completed, workerSessionContext(nil, execution), true); err == nil {
		t.Fatal("supervised Orca event accepted the local app-server protocol")
	}
}

func TestNativeOrcaFactsRejectOldDispatchAndKeepCurrentOutcome(t *testing.T) {
	turn := OrcaNativeTurn{TaskID: "task-current", DispatchID: "ctx-current"}
	message := OrcaInboxMessage{ID: "msg-done", FromHandle: "term-agent", Type: "worker_done"}
	message.Payload, _ = json.Marshal(`{"taskId":"task-old","dispatchId":"ctx-old","outcome":"succeeded"}`)
	if _, matched, err := decodeNativeOrcaMessage(message, "run-current", turn, "term-agent"); err != nil || matched {
		t.Fatalf("old Dispatch was attributed to current turn: %v %v", matched, err)
	}
	message.Payload, _ = json.Marshal(`{"taskId":"task-current","dispatchId":"ctx-current","outcome":"failed"}`)
	message.Body = "See https://git.example/o/r/pull/9"
	fact, matched, err := decodeNativeOrcaMessage(message, "run-current", turn, "term-agent")
	if err != nil || !matched || fact.Outcome != "failed" || fact.MessageID != "msg-done" || len(fact.PRCandidates) != 1 {
		t.Fatalf("current Dispatch evidence was lost: %+v %v", fact, err)
	}
}
