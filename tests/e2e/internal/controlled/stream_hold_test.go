package controlled

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

func TestWebSwitchHeldStreamRetainsOriginalTurn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	original := State{Session: "original-thread", Recovery: &RecoveryFixture{HoldStep: "implement.execute", HoldStream: true}}
	events := make(chan map[string]any, 8)
	done := make(chan error, 1)
	go func() {
		done <- recoveryStart(path, original, json.RawMessage(`1`), original.Session, fixtureSteps[0], func(value any) error {
			data, _ := json.Marshal(value)
			var event map[string]any
			json.Unmarshal(data, &event)
			events <- event
			return nil
		})
	}()
	deadline := time.After(5 * time.Second)
	for i := 0; i < 2; i++ {
		select {
		case <-events:
		case <-deadline:
			t.Fatal("native turn start was not observed")
		}
	}
	held, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	turn := held.Turns[0]["id"]
	select {
	case err := <-done:
		t.Fatal("active native stream disconnected before release", err)
	default:
	}
	held.Recovery.HoldStep = ""
	held.Turns[0]["status"] = "completed"
	held.Turns[0]["items"] = []any{map[string]any{"id": "original-final", "type": "agentMessage", "phase": "final_answer", "text": "original completion"}}
	if err = Write(path, held); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-deadline:
		t.Fatal("held stream did not finish on release")
	}
	final := <-events
	if final["method"] != "item/completed" {
		t.Fatal(final)
	}
	complete := <-events
	params := complete["params"].(map[string]any)
	if complete["method"] != "turn/completed" || params["threadId"] != original.Session || params["turn"].(map[string]any)["id"] != turn {
		t.Fatal("released another native turn", complete)
	}
	after, err := Read(path)
	if err != nil || after.Dispatches != 1 || len(after.Turns) != 1 {
		t.Fatal("release added native input", after, err)
	}
}
