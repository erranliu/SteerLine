package steerline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDetectNormalizer(t *testing.T) {
	if got := DetectNormalizer([]byte(`{"type":"session","version":3,"id":"x"}`)); got.Name() != "pi" {
		t.Fatalf("pi session header detected as %s", got.Name())
	}
	if got := DetectNormalizer([]byte(`{"type":"thread.started","thread_id":"t"}`)); got.Name() != "codex" {
		t.Fatalf("codex stream detected as %s", got.Name())
	}
}

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
	n := NewCodexNormalizer()
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

func TestPiNormalizer(t *testing.T) {
	lines := map[string]string{
		"session":   `{"type":"session","version":3,"id":"s"}`,
		"turn":      `{"type":"turn_start"}`,
		"message":   `{"type":"message_update","usage":{"input_tokens":1},"assistantMessageEvent":{"type":"text_delta","contentIndex":0,"delta":"Hi"}}`,
		"reasoning": `{"type":"message_update","assistantMessageEvent":{"type":"thinking_delta","contentIndex":0,"delta":"hmm"}}`,
		"command":   `{"type":"tool_execution_start","toolCallId":"tc1","toolName":"bash","args":{"command":"ls"}}`,
		"note":      `{"type":"compaction_start"}`,
	}
	want := map[string]string{
		"session":   "session",
		"turn":      "turn",
		"message":   "message",
		"reasoning": "reasoning",
		"command":   "command",
		"note":      "note",
	}
	n := NewPiNormalizer()
	for name, line := range lines {
		got := n.Normalize([]byte(line))
		if len(got) == 0 {
			t.Fatalf("%s: no event", name)
		}
		if got[0].Kind != want[name] {
			t.Fatalf("%s: kind=%q want %q", name, got[0].Kind, want[name])
		}
	}
	end := n.Normalize([]byte(`{"type":"tool_execution_end","toolCallId":"tc1","toolName":"bash","result":"ok","isError":false}`))
	if end[0].Status != "completed" {
		t.Fatalf("tool end status=%q", end[0].Status)
	}
}

func TestActivityWriterRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "activity.jsonl")
	w, e := OpenActivity(path, "run-1", "task-1")
	if e != nil {
		t.Fatal(e)
	}
	if e = w.Write(ActivityEvent{Source: "codex", Kind: "message", Detail: "one"}); e != nil {
		t.Fatal(e)
	}
	if e = w.Write(ActivityEvent{Source: "steer", Kind: "claim", Status: "completed"}); e != nil {
		t.Fatal(e)
	}
	if e = w.Close(); e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	rows := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	for i, row := range rows {
		var ev ActivityEvent
		if e := json.Unmarshal([]byte(row), &ev); e != nil {
			t.Fatalf("row %d not JSON: %v", i, e)
		}
		if ev.V != ActivityVersion {
			t.Fatalf("row %d v=%d", i, ev.V)
		}
		if ev.Seq != uint64(i+1) {
			t.Fatalf("row %d seq=%d", i, ev.Seq)
		}
		if ev.RunID != "run-1" || ev.TaskID != "task-1" {
			t.Fatalf("row %d identity=%q/%q", i, ev.RunID, ev.TaskID)
		}
		if ev.TS.IsZero() {
			t.Fatalf("row %d missing timestamp", i)
		}
	}
}

func TestOpenActivityCreatesRunDir(t *testing.T) {
	// Regression: dispatch opens the activity writer before the run directory
	// exists, so the writer must create it.
	dir := t.TempDir()
	logPath := filepath.Join(dir, "runs", "abc", "events.jsonl")
	r := &Run{RunID: "abc", TaskID: "t1", LogPath: &logPath}
	s := &Service{activities: map[string]*ActivityWriter{}}
	w := s.activityWriter(r)
	if w == nil {
		t.Fatal("activity writer was not opened")
	}
	if e := w.Write(ActivityEvent{Source: "steer", Kind: "claim"}); e != nil {
		t.Fatal(e)
	}
	s.closeActivity(r)
	if !fileExists(activityPath(logPath)) {
		t.Fatal("activity.jsonl was not created")
	}
}

func TestActivityWriterAppendRecoversSeq(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "activity.jsonl")
	w, e := OpenActivity(path, "r", "t")
	if e != nil {
		t.Fatal(e)
	}
	if e := w.Write(ActivityEvent{Source: "steer", Kind: "claim"}); e != nil {
		t.Fatal(e)
	}
	if e := w.Close(); e != nil {
		t.Fatal(e)
	}
	// A crash can leave a partial trailing line; reopening must repair it and
	// continue the sequence instead of truncating the journal.
	f, e := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		t.Fatal(e)
	}
	f.WriteString(`{"v":1,"seq":99`)
	f.Close()
	w2, e := OpenActivity(path, "r", "t")
	if e != nil {
		t.Fatal(e)
	}
	if e := w2.Write(ActivityEvent{Source: "steer", Kind: "completed"}); e != nil {
		t.Fatal(e)
	}
	w2.Close()
	b, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 2 {
		t.Fatalf("partial line was not repaired: %q", b)
	}
	var ev ActivityEvent
	if e := json.Unmarshal([]byte(lines[1]), &ev); e != nil {
		t.Fatal(e)
	}
	if ev.Seq != 2 {
		t.Fatalf("seq not recovered: %d", ev.Seq)
	}
}

func TestActivityJournalIsPerRun(t *testing.T) {
	dir := t.TempDir()
	s := &Service{activities: map[string]*ActivityWriter{}}
	log1 := filepath.Join(dir, "r1", "events.jsonl")
	log2 := filepath.Join(dir, "r2", "events.jsonl")
	r1 := &Run{RunID: "r1", TaskID: "t1", LogPath: &log1}
	r2 := &Run{RunID: "r2", TaskID: "t2", LogPath: &log2}
	s.note(r1, "steer", "pr", "info", "one", "", nil)
	s.note(r2, "steer", "completion", "completed", "two", "", nil)
	s.closeAllActivities()
	b1, e := os.ReadFile(activityPath(log1))
	if e != nil {
		t.Fatal(e)
	}
	b2, e := os.ReadFile(activityPath(log2))
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(string(b1), "one") || strings.Contains(string(b1), "two") {
		t.Fatalf("run r1 journal leaked another run: %s", b1)
	}
	if !strings.Contains(string(b2), "two") || strings.Contains(string(b2), "one") {
		t.Fatalf("run r2 journal leaked another run: %s", b2)
	}
}

func TestRenderActivity(t *testing.T) {
	ev := ActivityEvent{TS: time.Now(), Source: "codex", Kind: "command", Status: "completed", Title: "go test", ElapsedMS: 13200, Meta: map[string]any{"exit_code": 0}}
	s := RenderActivity(ev, RenderOptions{})
	for _, want := range []string{"command", "go test", "13.2s", "exit 0"} {
		if !strings.Contains(s, want) {
			t.Fatalf("render %q missing %q", s, want)
		}
	}
}
