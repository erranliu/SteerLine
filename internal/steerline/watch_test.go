package steerline

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestWatchRawStdoutIsOnlyCompleteRawLines(t *testing.T) {
	c := writeWatchFixture(t, false)
	var buf bytes.Buffer
	if e := Watch(context.Background(), c, "", WatchOptions{FromStart: true, Raw: true, Out: &buf}); e != nil {
		t.Fatal(e)
	}
	want := `{"type":"thread.started","thread_id":"t"}` + "\n" + `{"type":"item.completed","item":{"id":"i1","type":"agent_message","text":"hello"}}` + "\n"
	if buf.String() != want {
		t.Fatalf("raw stdout included status/diagnostic or changed records: %q", buf.String())
	}
}

func TestWatchRawPreservesMalformedContentWithoutJSONFiltering(t *testing.T) {
	c := writeWatchFixture(t, false)
	path := filepath.Join(c.RunDir, "runs", "abc", "events.jsonl")
	if err := os.WriteFile(path, []byte("not json\n{"+"broken\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Watch(context.Background(), c, "", WatchOptions{FromStart: true, Raw: true, Out: &out}); err != nil {
		t.Fatal(err)
	}
	if out.String() != "not json\n{broken\n" {
		t.Fatalf("raw mode parsed or changed complete lines: %q", out.String())
	}
}

func TestWatchJSONFallbackEmitsUnknownForMalformedRawRecord(t *testing.T) {
	c := writeWatchFixture(t, false)
	path := filepath.Join(c.RunDir, "runs", "abc", "events.jsonl")
	if err := os.WriteFile(path, []byte("not json\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Watch(context.Background(), c, "", WatchOptions{FromStart: true, JSON: true, Out: &out}); err != nil {
		t.Fatal(err)
	}
	var ev ActivityEvent
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &ev); err != nil {
		t.Fatalf("invalid JSON stdout %q: %v", out.String(), err)
	}
	if ev.V != ActivityVersion || ev.Seq != 1 || ev.TS.IsZero() || ev.Kind != "unknown" || ev.Source != "codex" || string(ev.Raw) != `"not json"` {
		t.Fatalf("malformed raw record not represented as unknown event: %+v raw=%s", ev, ev.Raw)
	}
}

func TestWatchFromStartFalseDetectsPiNormalizer(t *testing.T) {
	c := writeWatchFixture(t, false)
	runDir := filepath.Join(c.RunDir, "runs", "abc")
	eventsPath := filepath.Join(runDir, "events.jsonl")
	if err := os.WriteFile(eventsPath, []byte(`{"type":"session","version":1,"session_id":"pi-session"}`+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		time.Sleep(40 * time.Millisecond)
		f, err := os.OpenFile(eventsPath, os.O_APPEND|os.O_WRONLY, 0600)
		if err == nil {
			_, _ = f.WriteString(`{"type":"message_start","message":{"role":"assistant","content":[{"type":"text","text":"hello"}]}}` + "\n")
			_ = f.Close()
		}
		time.Sleep(320 * time.Millisecond)
		cancel()
	}()
	var buf bytes.Buffer
	if err := Watch(ctx, c, "", WatchOptions{FromStart: false, Follow: true, JSON: true, Out: &buf}); err != nil {
		t.Fatal(err)
	}
	var events []ActivityEvent
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var ev ActivityEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatal(err)
		}
		events = append(events, ev)
	}
	if len(events) != 1 || events[0].Source != "pi" || events[0].Kind != "message" || !strings.Contains(events[0].Detail, "hello") {
		t.Fatalf("from-start=false lost Pi detection: %+v stdout=%q", events, buf.String())
	}
}

func TestWatchFromStartFalseDetectsHostWhenFileStartsEmpty(t *testing.T) {
	c := writeWatchFixture(t, false)
	eventsPath := filepath.Join(c.RunDir, "runs", "abc", "events.jsonl")
	if err := os.WriteFile(eventsPath, nil, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		time.Sleep(40 * time.Millisecond)
		f, err := os.OpenFile(eventsPath, os.O_APPEND|os.O_WRONLY, 0600)
		if err == nil {
			_, _ = f.WriteString(`{"type":"session","version":1,"session_id":"pi-session"}` + "\n" + `{"type":"message_start","message":{"role":"assistant","content":[{"type":"text","text":"after open"}]}}` + "\n")
			_ = f.Close()
		}
		time.Sleep(320 * time.Millisecond)
		cancel()
	}()
	var out bytes.Buffer
	if err := Watch(ctx, c, "", WatchOptions{FromStart: false, Follow: true, JSON: true, Out: &out}); err != nil {
		t.Fatal(err)
	}
	var events []ActivityEvent
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if line == "" {
			continue
		}
		var ev ActivityEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatal(err)
		}
		events = append(events, ev)
	}
	if len(events) != 2 || events[0].Source != "pi" || events[1].Source != "pi" || events[1].Kind != "message" {
		t.Fatalf("empty-at-open stream lost host detection: %+v stdout=%q", events, out.String())
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
