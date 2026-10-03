package orcaworker

import (
	"context"
	"encoding/json"
	"errors"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"testing"
	"time"
)

func TestOriginalTerminalRequiresAgentDetectionBeforeQualifiedProcess(t *testing.T) {
	detected := false
	calls := 0
	cfg := Config{CommandTimeout: "2s", Output: func(context.Context, time.Duration, ...string) ([]byte, error) {
		calls++
		agent := ""
		if detected {
			agent = "codex"
		}
		return json.Marshal(map[string]any{"ok": true, "_meta": map[string]string{"runtimeId": "fixed-runtime"}, "result": map[string]any{"terminal": Terminal{Handle: "original-terminal", WorktreeID: "original-worktree", IncarnationID: "original-incarnation", Connected: true, AgentIdentity: agent}}})
	}}
	port := ExecutionLifecycle{client: New(cfg), orca: ProfileConfig{Executable: "fixture-host"}}
	worktree := Worktree{ID: "original-worktree"}
	process, bound, err := port.processForTerminal(context.Background(), "original-terminal", worktree, true)
	if !errors.Is(err, ErrAgentStarting) || bound || process != (executionapi.ProcessIdentity{}) {
		t.Fatal("unidentified terminal manufactured a qualified agent process")
	}
	detected = true
	process, bound, err = port.processForTerminal(context.Background(), "original-terminal", worktree, true)
	if err != nil || !bound || process.Terminal != "original-terminal" || process.Incarnation != "original-incarnation" || process.RuntimeID != "fixed-runtime" || calls != 2 {
		t.Fatalf("agent detection changed original terminal or failed: %+v %t %v calls=%d", process, bound, err, calls)
	}
}
