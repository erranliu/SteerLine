package codexworker

import (
	"bytes"
	"encoding/json"
	"fmt"
	executionvalues "github.com/erranliu/SteerLine/internal/execution/model"
	"os"
	"path/filepath"
	"strings"
	"testing"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func TestConversationMapperKeepsTurnAndPhaseEvidenceExplicit(t *testing.T) {
	mapper := &ConversationMapper{}
	base := executionvalues.ConversationEvent{ProjectID: "project", ExecutionID: "execution", AttemptID: "attempt", SourceSegment: "local-jsonl-v1", SourceEventQuality: executionvalues.IdentityDerived, SessionQuality: executionvalues.IdentityUnknown, TurnQuality: executionvalues.IdentityUnknown}
	mapLine := func(seq uint64, start int64, line string) executionvalues.ConversationEvent {
		event := base
		event.SourceSeq = seq
		event.SourceEventID = fmt.Sprintf("bytes:%d:%d", start, start+int64(len(line))+1)
		return mapper.MapCodexJSONL(event, []byte(line+"\n")).Event
	}
	session := mapLine(1, 0, `{"type":"thread.started","thread_id":"thread-native"}`)
	if session.Type != executionvalues.ConversationSession || session.SessionID != "thread-native" || session.SessionQuality != executionvalues.IdentityNative {
		t.Fatalf("session identity was not preserved: %+v", session)
	}
	started := mapLine(2, 54, `{"type":"turn.started"}`)
	if started.Type != executionvalues.ConversationTurnStarted || started.TurnID == "" || started.TurnQuality != executionvalues.IdentityDerived || started.Evidence["turn_derivation"] != "codex-jsonl-turn-boundary-v1" {
		t.Fatalf("explicit boundary was not assigned a derived turn identity: %+v", started)
	}
	message := mapLine(3, 78, `{"type":"item.completed","item":{"id":"same-item","type":"agent_message","text":"hello"}}`)
	if message.Type != executionvalues.ConversationMessageComplete || message.Phase != executionvalues.MessagePhaseUnknown || message.FinalAvailability == executionvalues.FinalAvailable {
		t.Fatalf("message completion was promoted to a final phase: %+v", message)
	}
	ended := mapLine(4, 180, `{"type":"turn.completed"}`)
	if ended.Type != executionvalues.ConversationTurnCompleted || ended.TurnID != started.TurnID || ended.FinalAvailability != executionvalues.FinalUnknown {
		t.Fatalf("turn boundary did not remain distinct from message phase: %+v", ended)
	}
	secondTurn := mapLine(5, 205, `{"type":"turn.started"}`)
	secondMessage := mapLine(6, 229, `{"type":"item.completed","item":{"id":"same-item","type":"agent_message","phase":"final_answer","text":"done"}}`)
	if secondMessage.TurnID != secondTurn.TurnID || secondMessage.ItemKey == message.ItemKey || secondMessage.Phase != executionvalues.MessagePhaseFinalAnswer || secondMessage.FinalAvailability != executionvalues.FinalAvailable {
		t.Fatalf("cross-turn item identity or verified phase was lost: first=%+v second=%+v", message, secondMessage)
	}
}

func TestConversationMapperPreservesMissingSnapshotAndExplicitEmptyFinal(t *testing.T) {
	if _, present := stringFieldPresent(map[string]json.RawMessage{"text": json.RawMessage(" null ")}, "text"); present {
		t.Fatal("JSON null was treated as explicit empty message text")
	}
	if text, present := stringFieldPresent(map[string]json.RawMessage{"text": json.RawMessage(`""`)}, "text"); !present || text != "" {
		t.Fatal("explicit empty message text was treated as missing")
	}
	mapper := &ConversationMapper{}
	lines := []string{
		`{"type":"turn.started"}`,
		`{"type":"item.updated","item":{"id":"message","type":"agent_message","text":"kept"}}`,
		`{"type":"item.updated","item":{"id":"message","type":"agent_message","status":"in_progress"}}`,
		`{"type":"item.completed","item":{"id":"message","type":"agent_message","phase":"final_answer"}}`,
		`{"type":"turn.completed"}`,
		`{"type":"turn.started"}`,
		`{"type":"item.completed","item":{"id":"empty","type":"agent_message","phase":"final_answer","text":""}}`,
	}
	writes := make([]executionvalues.ConversationWrite, 0, len(lines))
	for i, line := range lines {
		base := executionvalues.ConversationEvent{Version: executionvalues.ConversationVersion, ProjectID: "project", WorkflowID: "workflow", ExecutionID: "execution", AttemptID: "attempt",
			SourceSegment: "segment", SourceEventID: fmt.Sprintf("seq:%d", i+1), SourceSeq: uint64(i + 1)}
		writes = append(writes, mapper.MapCodexJSONL(base, []byte(line+"\n")))
	}
	if string(writes[1].Body) != "kept" || writes[1].BodyMode != "snapshot" || writes[2].BodyMode != "" || writes[3].BodyMode != "" ||
		writes[3].Event.FinalAvailability != executionvalues.FinalUnknown || writes[4].Event.FinalAvailability != executionvalues.FinalUnknown {
		t.Fatalf("missing text erased a snapshot or claimed a final: %+v", writes)
	}
	if writes[6].Body == nil || len(writes[6].Body) != 0 || writes[6].Event.FinalAvailability != executionvalues.FinalAvailable || writes[6].BodyMode != "complete" {
		t.Fatalf("explicit empty final was not preserved as complete: %+v", writes[6])
	}
}

func TestConversationMapperNativeTwoTurnFixture(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "docs", "validation", "pr-04b-local-codex-native-2026-09-25.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	mapper := &ConversationMapper{}
	var starts, completions, messages []executionvalues.ConversationEvent
	var offset int64
	for seq, line := range bytes.Split(bytes.TrimSuffix(raw, []byte("\n")), []byte("\n")) {
		event := executionvalues.ConversationEvent{Version: executionvalues.ConversationVersion, ProjectID: "project", ExecutionID: "execution", AttemptID: "attempt",
			SourceSegment: "native-fixture", SourceEventID: fmt.Sprintf("bytes:%d:%d", offset, offset+int64(len(line))+1), SourceSeq: uint64(seq + 1)}
		mapped := mapper.MapCodexJSONL(event, append(append([]byte(nil), line...), '\n')).Event
		switch mapped.Type {
		case executionvalues.ConversationTurnStarted:
			starts = append(starts, mapped)
		case executionvalues.ConversationTurnCompleted:
			completions = append(completions, mapped)
		case executionvalues.ConversationMessageComplete:
			messages = append(messages, mapped)
		}
		offset += int64(len(line)) + 1
	}
	if len(starts) != 2 || len(completions) != 2 || len(messages) != 3 {
		t.Fatalf("native fixture lost events: starts=%d completions=%d messages=%d", len(starts), len(completions), len(messages))
	}
	if starts[0].TurnID == starts[1].TurnID || starts[0].TurnQuality != executionvalues.IdentityDerived || starts[1].TurnQuality != executionvalues.IdentityDerived {
		t.Fatalf("native turn boundaries were not assigned distinct derived identities: %+v", starts)
	}
	for i := range completions {
		if completions[i].TurnID != starts[i].TurnID || completions[i].FinalAvailability != executionvalues.FinalUnknown {
			t.Fatalf("native completion was misattributed or promoted to a final answer: %+v", completions[i])
		}
	}
	if messages[0].ItemID != "item_0" || messages[2].ItemID != "item_1" || messages[1].ItemKey == messages[2].ItemKey {
		t.Fatalf("native item identity was not scoped by turn: %+v", messages)
	}
	for _, message := range messages {
		if message.Phase != executionvalues.MessagePhaseUnknown || message.SessionID != "native-session-redacted" || message.SessionQuality != executionvalues.IdentityNative {
			t.Fatalf("native fixture lacks phase and has a stable session ID: %+v", message)
		}
	}
}

func TestHistoryTerminalOutcomeFixturesKeepFailureAndCancellationDistinct(t *testing.T) {
	read := func(name string) HistoryThread {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "docs", "validation", name))
		if err != nil {
			t.Fatal(err)
		}
		var fixture struct {
			Thread HistoryThread `json:"thread"`
		}
		if err = json.Unmarshal(raw, &fixture); err != nil {
			t.Fatal(err)
		}
		return fixture.Thread
	}
	base := executionvalues.ConversationEvent{ProjectID: "project", ExecutionID: "execution", AttemptID: "attempt", Backend: executionbackend.BackendLocal,
		Protocol: executionbackend.ProtocolLocalProcess, Agent: "codex", SourceSegment: "native", SourceIdentity: "native"}
	cancelled := read("pr-04b-native-cancelled-2026-09-25.json")
	if len(cancelled.Turns) != 1 || cancelled.Turns[0].CompletedAt == nil {
		t.Fatal("cancelled fixture lacks native completion evidence")
	}
	writes, err := MapHistoryTurn(base, cancelled.Turns[0], 1)
	if err != nil || len(writes) != 2 || writes[0].Event.Type != executionvalues.ConversationTurnStarted || writes[1].Event.Type != executionvalues.ConversationTurnCancelled || writes[1].Event.FinalAvailability != executionvalues.FinalAbsent {
		t.Fatalf("cancelled turn was not kept distinct from a failed turn or final: writes=%+v err=%v", writes, err)
	}
	failed := read("pr-04b-native-failed-2026-09-25.json")
	if len(failed.Turns) != 1 || failed.Turns[0].CompletedAt == nil || failed.Turns[0].Status != "failed" {
		t.Fatal("failed fixture lacks native terminal evidence")
	}
	writes, err = MapHistoryTurn(base, failed.Turns[0], 1)
	if err != nil || len(writes) != 3 || writes[0].Event.Type != executionvalues.ConversationTurnStarted || writes[1].Event.Type != executionvalues.ConversationObservation || writes[2].Event.Type != executionvalues.ConversationTurnFailed || writes[2].Event.FinalAvailability != executionvalues.FinalAbsent {
		t.Fatalf("failed turn was not kept distinct from cancellation or final: writes=%+v err=%v", writes, err)
	}
	if len(writes[2].Raw) == 0 || !strings.Contains(string(writes[2].Raw), "401 Unauthorized") {
		t.Fatal("failed turn lost its original native error evidence")
	}
}
