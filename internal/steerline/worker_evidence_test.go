package steerline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/steerline/codexapp"
	"github.com/erranliu/SteerLine/internal/steerline/workerflow"
)

func TestWorkerProtocolFactsPreserveOriginalNativeIdentity(t *testing.T) {
	thread := "00000000-0000-0000-0000-000000000001"
	turn := "00000000-0000-0000-0000-000000000002"
	execution := &ExecutionRecord{SessionID: thread, CurrentTurnID: turn, AppServerGeneration: "generation-1"}
	started := WorkerEvent{Session: thread, AppServer: &codexapp.Envelope{Method: "turn/started", Params: mustJSON(map[string]any{"threadId": thread, "turn": map[string]string{"id": turn}})}}
	fact, err := decodeWorkerFlowEvent(workerflow.LocalCodex, started, execution, false)
	if err != nil || fact.StartedTurnID != turn {
		t.Fatalf("original turn start was lost: %+v %v", fact, err)
	}
	started.AppServer.Params = mustJSON(map[string]any{"threadId": "00000000-0000-0000-0000-000000000003", "turn": map[string]string{"id": turn}})
	if _, err := decodeWorkerFlowEvent(workerflow.LocalCodex, started, execution, false); err == nil {
		t.Fatal("replacement app-server thread was accepted")
	}
	completed := WorkerEvent{Session: thread, AppServer: &codexapp.Envelope{Method: "item/completed", Params: mustJSON(map[string]any{"threadId": thread, "turnId": turn, "item": map[string]any{"id": "call-1", "type": "dynamicToolCall", "status": "completed", "success": true, "contentItems": []any{map[string]string{"type": "inputText", "text": `{"answers":{"choice":["A"]}}`}}}})}}
	fact, err = decodeWorkerFlowEvent(workerflow.LocalCodex, completed, execution, true)
	if err != nil || fact.ToolReply == nil || !fact.ToolReply.MatchesAnswers(map[string][]string{"choice": {"A"}}) || fact.ToolReply.MatchesAnswers(map[string][]string{"choice": {"B"}}) {
		t.Fatalf("reply fact accepted another answer: %+v %v", fact, err)
	}
	completed.AppServer.Params = mustJSON(map[string]any{"threadId": thread, "turnId": "00000000-0000-0000-0000-000000000004", "item": map[string]string{"id": "call-1", "type": "dynamicToolCall"}})
	if _, err := decodeWorkerFlowEvent(workerflow.LocalCodex, completed, execution, true); err == nil {
		t.Fatal("old-turn reply was accepted")
	}
	if _, err := decodeWorkerFlowEvent(workerflow.NativeOrca, completed, execution, true); err == nil {
		t.Fatal("native Orca message bypassed its Dispatch adapter")
	}
	if _, err := decodeWorkerFlowEvent(workerflow.SupervisedOrca, completed, execution, true); err == nil {
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

func TestSupervisedTurnEvidenceRetainsQuestionAndEarlierPR(t *testing.T) {
	root := t.TempDir()
	runID, turnID := strings.Repeat("1", 32), strings.Repeat("2", 32)
	path := filepath.Join(root, "runs", runID, "turns", turnID, "events.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	journal := `{"type":"thread.started","thread_id":"00000000-0000-0000-0000-000000000008"}
{"type":"turn.started"}
{"type":"item.completed","item":{"type":"agent_message","text":"Created https://git.example/o/r/pulls/8."}}
{"type":"item.completed","item":{"type":"agent_message","text":"Continue?"}}
{"type":"turn.completed"}
`
	if err := os.WriteFile(path, []byte(journal), 0600); err != nil {
		t.Fatal(err)
	}
	fixed := workerFlowContext{run: &Run{RunID: runID}, execution: &ExecutionRecord{CurrentTurnID: turnID, SessionID: "00000000-0000-0000-0000-000000000008"}, config: Config{RunDir: root}}
	fact, err := readWorkerTurnEvidence(fixed, workerflow.SupervisedOrca)
	if err != nil || !fact.Completed || !fact.Succeeded || !fact.QuestionHint || len(fact.PRCandidates) != 1 || fact.PRCandidates[0] != "https://git.example/o/r/pulls/8" {
		t.Fatalf("supervised turn fact lost terminal evidence: %+v %v", fact, err)
	}
}
