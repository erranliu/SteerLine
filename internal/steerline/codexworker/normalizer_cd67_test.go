package codexworker

import (
	"testing"
)

func TestCodexNormalizer(t *testing.T) {
	lines := []string{
		`{"type":"thread.started","thread_id":"t1"}`,
		`{"type":"turn.started"}`,
		`{"type":"item.started","item":{"id":"i1","type":"command_execution","command":"go test ./...","aggregated_output":"","exit_code":null,"status":"in_progress"}}`,
		`{"type":"item.completed","item":{"id":"i1","type":"command_execution","command":"go test ./...","aggregated_output":"ok\n","exit_code":0,"status":"completed"}}`,
		`{"type":"item.completed","item":{"id":"i2","type":"agent_message","text":"hello"}}`,
		`{"type":"item.completed","item":{"id":"i3","type":"file_change","changes":[{"path":"a.go","kind":"update"}]}}`,
		`{"type":"item.completed","item":{"id":"i4","type":"reasoning","text":"thinking"}}`,
		`{"type":"turn.completed","usage":{"input_tokens":10,"output_tokens":2}}`,
		`{"type":"turn.failed","error":{"message":"boom"}}`,
		`{"type":"item.completed","item":{"id":"i5","type":"brand_new_item","payload":1}}`,
	}
	want := []string{"session", "turn", "command", "command", "message", "file_change", "reasoning", "usage", "error", "unknown"}
	n := NewNormalizer()
	for i, line := range lines {
		got := n.Normalize([]byte(line))
		if len(got) != 1 {
			t.Fatalf("line %d: got %d events, want 1", i, len(got))
		}
		if got[0].Kind != want[i] {
			t.Fatalf("line %d: kind=%q want %q", i, got[0].Kind, want[i])
		}
		if got[0].Source != "codex" {
			t.Fatalf("line %d: source=%q", i, got[0].Source)
		}
	}
	// A completed command carries status and elapsed time from its started pair.
	cmd := n.Normalize([]byte(`{"type":"item.completed","item":{"id":"z","type":"command_execution","command":"x","exit_code":7,"status":"completed"}}`))
	if cmd[0].Status != "failed" {
		t.Fatalf("nonzero exit should map to failed, got %q", cmd[0].Status)
	}
	if code, ok := cmd[0].Meta["exit_code"].(int); !ok || code != 7 {
		t.Fatalf("exit_code meta missing: %#v", cmd[0].Meta)
	}
	// Unknown item types preserve the raw line for forward compatibility.
	unknown := n.Normalize([]byte(`{"type":"item.completed","item":{"id":"i9","type":"brand_new_item"}}`))
	if len(unknown[0].Raw) == 0 {
		t.Fatal("unknown item must preserve raw")
	}
}
