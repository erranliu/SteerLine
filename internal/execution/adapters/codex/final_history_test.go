package codexworker

import (
	"encoding/json"
	"testing"

	model "github.com/erranliu/SteerLine/internal/execution/model"
)

func TestHistoryMixedTurnRetainsOnlyCompleteFinal(t *testing.T) {
	completed := int64(1)
	turn := HistoryTurn{ID: "original-turn", Status: "completed", CompletedAt: &completed, Items: []json.RawMessage{
		json.RawMessage(`{"id":"tool","type":"commandExecution","aggregatedOutput":"private tool output"}`),
		json.RawMessage(`{"id":"comment","type":"agentMessage","phase":"commentary","text":"working"}`),
		json.RawMessage(`{"type":"futureTool","private":"unrecognized process body"}`),
		json.RawMessage(`{"id":"final","type":"agentMessage","phase":"final_answer","text":"完成了🙂"}`),
	}}
	base := model.ConversationEvent{ProjectID: "project", ExecutionID: "execution", AttemptID: "attempt", SessionID: "original-session", SourceSegment: "original-source"}
	writes, err := MapHistoryTurn(base, turn, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(writes) != 3 || writes[0].Event.Type != model.ConversationTurnStarted || writes[2].Event.Type != model.ConversationTurnCompleted {
		t.Fatalf("process events retained or turn facts lost: %+v", writes)
	}
	final := writes[1]
	if final.Event.SessionID != base.SessionID || final.Event.TurnID != turn.ID || final.Event.ItemID != "final" || string(final.Body) != "完成了🙂" || final.BodyMode != "complete" || final.Event.FinalAvailability != model.FinalAvailable {
		t.Fatalf("complete final or native identity lost: %+v", final)
	}
	turn.CompletedAt = nil
	active, err := MapHistoryTurn(base, turn, 10)
	if err != nil || len(active) != 1 || len(active[0].Body) != 0 {
		t.Fatalf("active turn stored a partial final: %+v %v", active, err)
	}
	turn.CompletedAt = &completed
	turn.Items = []json.RawMessage{json.RawMessage(`{"id":"missing","type":"agentMessage","phase":"final_answer"}`)}
	missing, err := MapHistoryTurn(base, turn, 20)
	if err != nil || len(missing) != 3 || missing[2].Event.Type != model.ConversationGap || missing[2].Event.FinalAvailability != model.FinalUnknown {
		t.Fatalf("missing final was not explicit: %+v %v", missing, err)
	}
	for _, write := range missing {
		if write.BodyMode != "" {
			t.Fatal("missing text became a body")
		}
	}
}
