package native

import (
	"encoding/json"
	"testing"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func TestRemoteInventoryDistinguishesWorkerShellAndResumedDuplicate(t *testing.T) {
	identity := executionbackend.ProcessIdentity{Terminal: "original", Incarnation: "epoch", WorktreeID: "repo::tree"}
	worker := map[string]any{"handle": "original", "incarnationId": "epoch", "worktreeId": "repo::tree", "agentIdentity": "codex", "connected": true, "orphaned": true}
	shell := map[string]any{"handle": "shell", "connected": true}
	duplicate := map[string]any{"handle": "resumed", "incarnationId": "other", "worktreeId": "repo::tree", "agentIdentity": "codex", "connected": true}
	for _, test := range []struct {
		name             string
		terminals        []any
		truncated, valid bool
	}{
		{"headless_worker_and_shell", []any{worker, shell}, false, true},
		{"resumed_duplicate", []any{worker, shell, duplicate}, false, false},
		{"only_resumed", []any{duplicate}, false, false},
		{"no_worker", []any{shell}, false, false},
		{"incomplete_inventory", []any{worker}, true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			raw, err := json.Marshal(map[string]any{"terminals": test.terminals, "truncated": test.truncated})
			if err != nil {
				t.Fatal(err)
			}
			if err := checkWorkerTerminalInventory(raw, identity, "codex"); (err == nil) != test.valid {
				t.Fatalf("valid=%v: %v", test.valid, err)
			}
		})
	}
}
