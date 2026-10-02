package controlled

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestEmptyThreadCannotResumeUntilItHasATurn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	state := State{Session: "original", Cwd: "workspace"}
	for _, hasTurn := range []bool{false, true} {
		if hasTurn {
			state.Turns = []map[string]any{{"id": "turn", "status": "completed"}}
		}
		if err := Write(path, state); err != nil {
			t.Fatal(err)
		}
		var receipt map[string]any
		err := handleRPC(path, []byte(`{"id":1,"method":"thread/resume","params":{"threadId":"original"}}`), func(v any) error {
			raw, err := json.Marshal(v)
			if err != nil {
				return err
			}
			return json.Unmarshal(raw, &receipt)
		})
		if err != nil || (receipt["error"] == nil) != hasTurn {
			t.Fatalf("hasTurn=%t: receipt=%v err=%v", hasTurn, receipt, err)
		}
		got, err := Read(path)
		if err != nil || got.Session != state.Session || got.Dispatches != 0 || got.Sessions != 0 {
			t.Fatal("resume changed the original controlled session")
		}
	}
}
