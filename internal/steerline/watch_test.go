package steerline

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeWatchFixture(t *testing.T, withActivity bool) Config {
	t.Helper()
	dir := t.TempDir()
	runDir := filepath.Join(dir, "runs", "abc")
	if e := os.MkdirAll(runDir, 0700); e != nil {
		t.Fatal(e)
	}
	events := filepath.Join(runDir, "events.jsonl")
	result := filepath.Join(runDir, "result.json")
	rawEvents := `{"type":"thread.started","thread_id":"t"}` + "\n" +
		`{"type":"item.completed","item":{"id":"i1","type":"agent_message","text":"hello"}}` + "\n"
	if e := os.WriteFile(events, []byte(rawEvents), 0600); e != nil {
		t.Fatal(e)
	}
	if withActivity {
		activity := `{"v":1,"seq":1,"ts":"2026-01-01T00:00:00Z","source":"steer","run_id":"abc","task_id":"t1","kind":"claim"}` + "\n" +
			`{"v":1,"seq":2,"ts":"2026-01-01T00:00:01Z","source":"codex","run_id":"abc","task_id":"t1","kind":"message","detail":"hi"}` + "\n"
		if e := os.WriteFile(activityPath(events), []byte(activity), 0600); e != nil {
			t.Fatal(e)
		}
	}
	rt := Runtime{SchemaVersion: 2, Runs: []*Run{{TaskID: "t1", RunID: "abc", Title: "x", ExecutionStatus: "running", FinishTaskStatus: "pending", LogPath: &events, ResultPath: &result}}}
	b, e := json.Marshal(rt)
	if e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(dir, "runtime.json"), b, 0600); e != nil {
		t.Fatal(e)
	}
	return Config{RunDir: dir, RuntimeFile: "runtime.json"}
}

func TestWatchReplaysActivityJSON(t *testing.T) {
	c := writeWatchFixture(t, true)
	var buf bytes.Buffer
	if e := Watch(context.Background(), c, "", WatchOptions{FromStart: true, JSON: true, Out: &buf}); e != nil {
		t.Fatal(e)
	}
	rows := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2: %q", len(rows), buf.String())
	}
	var ev ActivityEvent
	if e := json.Unmarshal([]byte(rows[0]), &ev); e != nil {
		t.Fatal(e)
	}
	if ev.Kind != "claim" || ev.RunID != "abc" {
		t.Fatalf("unexpected first row: %#v", ev)
	}
}

func TestWatchFallsBackToRawEvents(t *testing.T) {
	c := writeWatchFixture(t, false)
	var buf bytes.Buffer
	if e := Watch(context.Background(), c, "", WatchOptions{FromStart: true, JSON: true, Out: &buf}); e != nil {
		t.Fatal(e)
	}
	rows := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2: %q", len(rows), buf.String())
	}
	var ev ActivityEvent
	if e := json.Unmarshal([]byte(rows[0]), &ev); e != nil {
		t.Fatal(e)
	}
	if ev.Kind != "session" || ev.Source != "codex" {
		t.Fatalf("unexpected normalized row: %#v", ev)
	}
}

func TestWatchResolvesRunByID(t *testing.T) {
	c := writeWatchFixture(t, true)
	var buf bytes.Buffer
	if e := Watch(context.Background(), c, "t1", WatchOptions{FromStart: true, JSON: true, Out: &buf}); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(buf.String(), `"kind":"claim"`) {
		t.Fatalf("run not resolved by task id: %q", buf.String())
	}
	if e := Watch(context.Background(), c, "missing", WatchOptions{JSON: true, Out: &buf}); e == nil {
		t.Fatal("expected error for unknown run")
	}
}
