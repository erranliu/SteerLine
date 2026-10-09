package controlled

import (
	"encoding/json"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
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

func TestNewControlledThreadPreservesOriginalReadAndDispatchCounters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	initial := State{Session: "original", Cwd: "original-workspace", Sessions: 1, Dispatches: 1, Turns: []map[string]any{{"id": "old-turn", "status": "completed"}}}
	if err := Write(path, initial); err != nil {
		t.Fatal(err)
	}
	var receipt map[string]any
	send := func(v any) error {
		raw, err := json.Marshal(v)
		if err != nil {
			return err
		}
		return json.Unmarshal(raw, &receipt)
	}
	if err := handleRPC(path, []byte(`{"id":1,"method":"thread/start","params":{"cwd":"other-workspace"}}`), send); err != nil {
		t.Fatal(err)
	}
	current, err := Read(path)
	if err != nil || current.Sessions != 2 || current.Dispatches != 1 || current.Session == initial.Session || len(current.Turns) != 0 {
		t.Fatal("new thread lost aggregate identity/counts", err)
	}
	if err := handleRPC(path, []byte(`{"id":2,"method":"thread/read","params":{"threadId":"original"}}`), send); err != nil {
		t.Fatal(err)
	}
	thread := receipt["result"].(map[string]any)["thread"].(map[string]any)
	if thread["id"] != "original" || thread["cwd"] != "original-workspace" || len(thread["turns"].([]any)) != 1 {
		t.Fatal("original thread history changed", thread)
	}
	if err := handleRPC(path, []byte(`{"id":3,"method":"turn/start","params":{"threadId":"original","input":[{"text":"duplicate"}]}}`), send); err == nil {
		t.Fatal("an old thread accepted duplicate business input")
	}
}

func TestControlledProviderKeepsOriginalPRWhenAnotherTaskPublishes(t *testing.T) {
	first := executionmodel.PRInfo{URL: "https://github.com/e2e/project/pull/1", HeadBranch: "first", State: "open"}
	second := executionmodel.PRInfo{URL: "https://github.com/e2e/project/pull/2", HeadBranch: "second", State: "open"}
	s := State{PR: second, PRs: map[string]executionmodel.PRInfo{first.URL: first, second.URL: second}}
	for _, expected := range []executionmodel.PRInfo{first, second} {
		got, err := ProviderPR(s, expected.URL)
		if err != nil || got.URL != expected.URL || got.HeadBranch != expected.HeadBranch {
			t.Fatal("provider retargeted a fixed PR", got, err)
		}
	}
	if _, err := ProviderPR(s, "https://github.com/e2e/project/pull/3"); err == nil {
		t.Fatal("unknown PR URL reused the latest result")
	}
}
