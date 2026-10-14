package controlled

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWebHistoryReturnsBoundedOriginalTurnPages(t *testing.T) {
	file := filepath.Join(t.TempDir(), "state.json")
	state := State{Session: "original", Cwd: "original-workspace", Turns: []map[string]any{{"id": "first", "status": "completed", "completedAt": 1, "items": []any{map[string]any{"type": "agentMessage", "id": "first-final", "phase": "final_answer", "text": "中文最终回复"}}}, {"id": "second", "status": "completed", "completedAt": 2, "items": []any{map[string]any{"type": "agentMessage", "id": "second-final", "phase": "final_answer", "text": "second"}}}}}
	if err := Write(file, state); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(file)
	for _, cursor := range []string{"", "1"} {
		input, _ := json.Marshal(map[string]any{"id": 7, "method": "thread/turns/list", "params": map[string]any{"threadId": "original", "cursor": cursor, "limit": 1, "itemsView": "full", "sortDirection": "asc"}})
		var receipt struct {
			Result struct {
				Data      []map[string]any `json:"data"`
				Next      *string          `json:"nextCursor"`
				Backwards *string          `json:"backwardsCursor"`
			} `json:"result"`
		}
		err := handleRPC(file, input, func(value any) error { data, _ := json.Marshal(value); return json.Unmarshal(data, &receipt) })
		if err != nil {
			t.Fatal(err)
		}
		if len(receipt.Result.Data) != 1 || receipt.Result.Data[0]["itemsView"] != "full" || receipt.Result.Backwards == nil {
			t.Fatalf("unbounded history: %+v", receipt)
		}
		if cursor == "" && (receipt.Result.Data[0]["id"] != "first" || receipt.Result.Next == nil || *receipt.Result.Next != "1") {
			t.Fatal("first page identity/cursor changed")
		}
		if cursor == "1" && (receipt.Result.Data[0]["id"] != "second" || receipt.Result.Next != nil) {
			t.Fatal("second page changed")
		}
	}
	after, _ := os.ReadFile(file)
	if string(after) != string(before) {
		t.Fatal("history read mutated the controlled native thread")
	}
}
