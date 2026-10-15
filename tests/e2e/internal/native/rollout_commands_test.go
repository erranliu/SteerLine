package native

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeRolloutCommandBinding(t *testing.T) {
	for _, tc := range []struct {
		name, thread, cwd   string
		complete, wantError bool
	}{
		{"original", "original", "C:/owned", true, false},
		{"wrong_thread", "other", "C:/owned", true, true},
		{"wrong_cwd", "original", "C:/sibling", true, true},
		{"not_complete", "original", "C:/owned", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lines := []any{
				map[string]any{"type": "session_meta", "payload": map[string]any{"id": "original", "cwd": "C:/owned", "source": "cli"}},
				map[string]any{"type": "event_msg", "payload": map[string]any{"type": "item_completed", "thread_id": "original", "turn_id": "other", "item": map[string]any{"type": "CommandExecution", "cwd": "C:/sibling", "command": []string{"not selected"}}}},
				map[string]any{"type": "event_msg", "payload": map[string]any{"type": "item_completed", "thread_id": tc.thread, "turn_id": "selected", "item": map[string]any{"type": "CommandExecution", "cwd": "file:///" + tc.cwd, "command": []string{"powershell.exe", "-File", "./sandbox-check.ps1"}, "status": "completed", "exit_code": 0, "aggregated_output": "native proof"}}},
			}
			if tc.complete {
				lines = append(lines, map[string]any{"type": "event_msg", "payload": map[string]any{"type": "task_complete", "turn_id": "selected"}})
			}
			var raw []byte
			for _, line := range lines {
				b, _ := json.Marshal(line)
				raw = append(append(raw, b...), '\n')
			}
			commands, err := RolloutTurnCommands(raw, "original", "selected", "C:/owned")
			if (err != nil) != tc.wantError {
				t.Fatalf("commands=%+v err=%v", commands, err)
			}
			if !tc.wantError && (len(commands) != 1 || commands[0].ExitCode == nil || *commands[0].ExitCode != 0 || commands[0].Output != "native proof") {
				t.Fatalf("lost selected native facts: %+v", commands)
			}
		})
	}
}

func TestReadNativeRolloutRejectsMismatchedHeader(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "sessions")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "rollout-original.jsonl")
	for _, id := range []string{"other", "original"} {
		raw := `{"type":"session_meta","payload":{"id":"` + id + `","cwd":"C:/owned","source":"cli"}}` + "\n"
		if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		got, err := ReadBusinessRollout(home, "original", "C:/owned")
		if (err != nil) != (id != "original") || err == nil && !strings.Contains(string(got), `"original"`) {
			t.Fatalf("id=%s err=%v", id, err)
		}
	}
}
