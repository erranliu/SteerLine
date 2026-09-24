package steerline

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ActivityVersion is the stable schema version of activity.jsonl. Consumers must
// tolerate unknown fields and treat unknown kinds as opaque. The raw agent stream
// stays in events.jsonl; activity.jsonl is a normalized derivative.
const ActivityVersion = 1

const (
	activityDetailMax = 4096
	activityTitleMax  = 240
)

// ActivityEvent is one normalized record. Field semantics are documented in
// docs/activity.md and are intentionally independent of the underlying runner.
type ActivityEvent struct {
	V         int             `json:"v"`
	Seq       uint64          `json:"seq"`
	TS        time.Time       `json:"ts"`
	Source    string          `json:"source"`
	RunID     string          `json:"run_id,omitempty"`
	TaskID    string          `json:"task_id,omitempty"`
	ItemID    string          `json:"item_id,omitempty"`
	Kind      string          `json:"kind"`
	Status    string          `json:"status,omitempty"`
	Title     string          `json:"title,omitempty"`
	Detail    string          `json:"detail,omitempty"`
	ElapsedMS int64           `json:"elapsed_ms,omitempty"`
	Meta      map[string]any  `json:"meta,omitempty"`
	Raw       json.RawMessage `json:"raw,omitempty"`
}

// DecodeActivityLine validates one normalized journal object while tolerating
// unknown fields and unknown kind values for forward compatibility.
func DecodeActivityLine(line []byte) (ActivityEvent, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(line, &fields); err != nil {
		return ActivityEvent{}, err
	}
	if fields == nil {
		return ActivityEvent{}, errors.New("activity record must be a JSON object")
	}
	var ev ActivityEvent
	if err := json.Unmarshal(line, &ev); err != nil {
		return ActivityEvent{}, err
	}
	if ev.Seq == 0 {
		return ActivityEvent{}, errors.New("activity record is missing a positive seq")
	}
	if ev.Kind == "" {
		return ActivityEvent{}, errors.New("activity record is missing kind")
	}
	return ev, nil
}

// Normalizer converts one raw agent stdout line into zero or more activity
// records. Implementations must be tolerant: an unrecognized line becomes a
// kind=unknown record with Raw preserved, never an error.
type Normalizer interface {
	Name() string
	Detect(firstLine []byte) bool
	Normalize(line []byte) []ActivityEvent
}

// DetectNormalizer picks a normalizer from the first line of a stream. Codex
// starts with thread.started; pi starts with a versioned session header.
func DetectNormalizer(firstLine []byte) Normalizer {
	if NewPiNormalizer().Detect(firstLine) {
		return NewPiNormalizer()
	}
	return NewCodexNormalizer()
}

func activityPath(logPath string) string {
	return filepath.Join(filepath.Dir(logPath), "activity.jsonl")
}

// ActivityPath is the exported form of activityPath for the viewer and other
// read-only consumers that hold only a run's events.jsonl path.
func ActivityPath(logPath string) string { return activityPath(logPath) }

// ActivityWriter appends normalized records as JSONL, one open/append/close per
// record, so no file handle outlives a write and Windows directory cleanup and
// service restarts are never blocked. It keeps only the last seq in memory. The
// service is the single writer.
type ActivityWriter struct {
	mu     sync.Mutex
	path   string
	seq    uint64
	runID  string
	taskID string
}

// OpenActivity prepares the journal for append: it repairs a partial trailing
// line and recovers the last seq so ordering stays monotonic across lazy reopens
// and service restarts. No handle is retained after this call.
func OpenActivity(path, runID, taskID string) (*ActivityWriter, error) {
	seq, e := recoverActivitySeq(path)
	if e != nil {
		return nil, e
	}
	return &ActivityWriter{path: path, seq: seq, runID: runID, taskID: taskID}, nil
}

// recoverActivitySeq repairs a partial trailing line and returns the highest seq
// already present. The journal is a derivative file: a read failure costs only
// ordering, never truth.
func recoverActivitySeq(path string) (uint64, error) {
	f, e := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return 0, e
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil {
		return 0, e
	}
	if info.Size() == 0 {
		return 0, nil
	}
	b, e := io.ReadAll(io.LimitReader(f, 64<<20))
	if e != nil {
		return 0, e
	}
	if len(b) > 0 && b[len(b)-1] != '\n' {
		if i := bytes.LastIndexByte(b, '\n'); i >= 0 {
			b = b[:i+1]
		} else {
			b = nil
		}
		if e := f.Truncate(int64(len(b))); e != nil {
			return 0, e
		}
	}
	var last uint64
	for len(b) > 0 {
		i := bytes.IndexByte(b, '\n')
		if i < 0 {
			break
		}
		var ev ActivityEvent
		if json.Unmarshal(b[:i], &ev) == nil && ev.Seq > last {
			last = ev.Seq
		}
		b = b[i+1:]
	}
	return last, nil
}

func (w *ActivityWriter) Write(ev ActivityEvent) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.seq++
	ev.V = ActivityVersion
	ev.Seq = w.seq
	if ev.TS.IsZero() {
		ev.TS = now()
	} else {
		ev.TS = ev.TS.UTC()
	}
	if ev.RunID == "" {
		ev.RunID = w.runID
	}
	if ev.TaskID == "" {
		ev.TaskID = w.taskID
	}
	if ev.Detail != "" {
		ev.Detail = clip(ev.Detail, activityDetailMax)
	}
	if ev.Title != "" {
		ev.Title = clip(ev.Title, activityTitleMax)
	}
	b, e := json.Marshal(ev)
	if e != nil {
		return e
	}
	f, e := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if e != nil {
		return e
	}
	_, e = f.Write(append(b, '\n'))
	ce := f.Close()
	if e == nil {
		e = ce
	}
	return e
}

// Close is a no-op: writes never retain a handle.
func (w *ActivityWriter) Close() error { return nil }

func cloneRaw(line []byte) json.RawMessage {
	if len(line) == 0 {
		return nil
	}
	if !json.Valid(line) {
		b, _ := json.Marshal(string(line))
		return b
	}
	return append(json.RawMessage(nil), line...)
}

func firstLine(s string, n int) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return clip(strings.TrimSpace(s), n)
}

func rawToText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return string(raw)
}

// ---- Codex ----

type codexExecEvent struct {
	Type     string `json:"type"`
	ThreadID string `json:"thread_id"`
	Message  string `json:"message"`
	Error    *struct {
		Message string `json:"message"`
	} `json:"error"`
	Usage map[string]any `json:"usage"`
	Item  struct {
		ID               string `json:"id"`
		Type             string `json:"type"`
		Text             string `json:"text"`
		Message          string `json:"message"`
		Command          string `json:"command"`
		AggregatedOutput string `json:"aggregated_output"`
		ExitCode         *int   `json:"exit_code"`
		Status           string `json:"status"`
		Changes          []struct {
			Path string `json:"path"`
			Kind string `json:"kind"`
		} `json:"changes"`
	} `json:"item"`
}

type codexNormalizer struct {
	started map[string]time.Time
}

func NewCodexNormalizer() Normalizer { return &codexNormalizer{started: map[string]time.Time{}} }

func (n *codexNormalizer) Name() string { return "codex" }

func (n *codexNormalizer) Detect(line []byte) bool {
	var probe struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(line, &probe) != nil {
		return false
	}
	return probe.Type == "thread.started"
}

func (n *codexNormalizer) Normalize(line []byte) []ActivityEvent {
	var e codexExecEvent
	if json.Unmarshal(line, &e) != nil {
		return []ActivityEvent{{Source: "codex", Kind: "unknown", Title: "unparsed raw event", Raw: cloneRaw(line)}}
	}
	raw := cloneRaw(line)
	switch e.Type {
	case "thread.started":
		return []ActivityEvent{{Source: "codex", Kind: "session", Status: "info", Title: "codex session", Meta: map[string]any{"thread_id": e.ThreadID}}}
	case "turn.started":
		return []ActivityEvent{{Source: "codex", Kind: "turn", Status: "started"}}
	case "turn.completed":
		return []ActivityEvent{{Source: "codex", Kind: "usage", Status: "completed", Meta: map[string]any{"usage": e.Usage}}}
	case "turn.failed":
		msg := ""
		if e.Error != nil {
			msg = e.Error.Message
		}
		return []ActivityEvent{{Source: "codex", Kind: "error", Status: "failed", Title: "turn failed", Detail: msg, Raw: raw}}
	case "error":
		return []ActivityEvent{{Source: "codex", Kind: "error", Status: "failed", Detail: e.Message, Raw: raw}}
	case "item.started", "item.updated", "item.completed":
		return []ActivityEvent{n.item(e, raw)}
	}
	return []ActivityEvent{{Source: "codex", Kind: "unknown", Title: e.Type, Raw: raw}}
}

func (n *codexNormalizer) item(e codexExecEvent, raw json.RawMessage) ActivityEvent {
	it := e.Item
	status := it.Status
	if status == "" {
		switch e.Type {
		case "item.started":
			status = "started"
		case "item.completed":
			status = "completed"
		default:
			status = "running"
		}
	}
	var elapsed int64
	if it.ID != "" {
		switch e.Type {
		case "item.started":
			n.started[it.ID] = time.Now()
		case "item.completed":
			if t, ok := n.started[it.ID]; ok {
				elapsed = time.Since(t).Milliseconds()
				delete(n.started, it.ID)
			}
		}
	}
	ev := ActivityEvent{Source: "codex", ItemID: it.ID, Status: status, ElapsedMS: elapsed}
	switch it.Type {
	case "agent_message":
		ev.Kind = "message"
		ev.Detail = it.Text
	case "reasoning":
		ev.Kind = "reasoning"
		ev.Status = "info"
		ev.Detail = it.Text
	case "command_execution":
		ev.Kind = "command"
		ev.Title = firstLine(it.Command, activityTitleMax)
		ev.Detail = it.AggregatedOutput
		if it.ExitCode != nil {
			ev.Meta = map[string]any{"exit_code": *it.ExitCode}
			if *it.ExitCode != 0 && e.Type == "item.completed" {
				ev.Status = "failed"
			}
		}
	case "file_change":
		ev.Kind = "file_change"
		paths := make([]string, 0, len(it.Changes))
		for _, c := range it.Changes {
			paths = append(paths, strings.TrimSpace(c.Kind+" "+c.Path))
		}
		ev.Title = strings.Join(paths, ", ")
		ev.Meta = map[string]any{"changes": it.Changes}
	case "error":
		ev.Kind = "error"
		ev.Status = "failed"
		ev.Detail = it.Message
		ev.Raw = raw
	default:
		ev.Kind = "unknown"
		ev.Title = it.Type
		ev.Raw = raw
	}
	return ev
}

// ---- pi ----

type piEvent struct {
	Type                  string          `json:"type"`
	Version               int             `json:"version"`
	Message               json.RawMessage `json:"message"`
	Usage                 map[string]any  `json:"usage"`
	AssistantMessageEvent *struct {
		Type         string `json:"type"`
		ContentIndex int    `json:"contentIndex"`
		Delta        string `json:"delta"`
		ID           string `json:"id"`
		ToolName     string `json:"toolName"`
	} `json:"assistantMessageEvent"`
	ToolCallID    string          `json:"toolCallId"`
	ToolName      string          `json:"toolName"`
	Args          json.RawMessage `json:"args"`
	PartialResult json.RawMessage `json:"partialResult"`
	Result        json.RawMessage `json:"result"`
	IsError       bool            `json:"isError"`
	MessageText   string          `json:"messageText"`
}

type piContent struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Thinking  string          `json:"thinking"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type piMessage struct {
	Role    string      `json:"role"`
	Content []piContent `json:"content"`
}

type piNormalizer struct {
	started map[string]time.Time
}

func NewPiNormalizer() Normalizer { return &piNormalizer{started: map[string]time.Time{}} }

func (n *piNormalizer) Name() string { return "pi" }

func (n *piNormalizer) Detect(line []byte) bool {
	var probe struct {
		Type    string `json:"type"`
		Version int    `json:"version"`
	}
	if json.Unmarshal(line, &probe) != nil {
		return false
	}
	return probe.Type == "session" && probe.Version > 0
}

func (n *piNormalizer) Normalize(line []byte) []ActivityEvent {
	var e piEvent
	if json.Unmarshal(line, &e) != nil {
		return []ActivityEvent{{Source: "pi", Kind: "unknown", Title: "unparsed raw event", Raw: cloneRaw(line)}}
	}
	raw := cloneRaw(line)
	switch e.Type {
	case "session":
		return []ActivityEvent{{Source: "pi", Kind: "session", Status: "info", Title: "pi session", Meta: map[string]any{"version": e.Version}}}
	case "agent_start", "turn_start":
		return []ActivityEvent{{Source: "pi", Kind: "turn", Status: "started", Title: e.Type}}
	case "agent_end", "turn_end":
		return []ActivityEvent{{Source: "pi", Kind: "turn", Status: "completed", Title: e.Type, Meta: map[string]any{"usage": e.Usage}}}
	case "message_start":
		return []ActivityEvent{{Source: "pi", Kind: "message", Status: "started", Detail: piMessageText(e.Message)}}
	case "message_end":
		return []ActivityEvent{{Source: "pi", Kind: "message", Status: "completed", Detail: piMessageText(e.Message), Meta: map[string]any{"usage": e.Usage}}}
	case "message_update":
		return n.update(e)
	case "tool_execution_start":
		ev := ActivityEvent{Source: "pi", Kind: "command", Status: "started", ItemID: e.ToolCallID, Title: e.ToolName}
		if len(e.Args) > 0 {
			ev.Detail = rawToText(e.Args)
		}
		if e.ToolCallID != "" {
			n.started[e.ToolCallID] = time.Now()
		}
		return []ActivityEvent{ev}
	case "tool_execution_update":
		ev := ActivityEvent{Source: "pi", Kind: "command", Status: "running", ItemID: e.ToolCallID, Title: e.ToolName}
		ev.Detail = rawToText(e.PartialResult)
		return []ActivityEvent{ev}
	case "tool_execution_end":
		status := "completed"
		if e.IsError {
			status = "failed"
		}
		ev := ActivityEvent{Source: "pi", Kind: "command", Status: status, ItemID: e.ToolCallID, Title: e.ToolName}
		if e.ToolCallID != "" {
			if t, ok := n.started[e.ToolCallID]; ok {
				ev.ElapsedMS = time.Since(t).Milliseconds()
				delete(n.started, e.ToolCallID)
			}
		}
		ev.Detail = rawToText(e.Result)
		ev.Meta = map[string]any{"is_error": e.IsError}
		return []ActivityEvent{ev}
	case "compaction_start":
		return []ActivityEvent{{Source: "pi", Kind: "note", Status: "started", Title: "compaction started"}}
	case "compaction_end":
		return []ActivityEvent{{Source: "pi", Kind: "note", Status: "completed", Title: "compaction finished"}}
	case "queue_update":
		return []ActivityEvent{{Source: "pi", Kind: "note", Status: "info", Title: "queue updated"}}
	}
	return []ActivityEvent{{Source: "pi", Kind: "unknown", Title: e.Type, Raw: raw}}
}

func (n *piNormalizer) update(e piEvent) []ActivityEvent {
	ame := e.AssistantMessageEvent
	if ame == nil {
		return nil
	}
	itemID := ame.ID
	if itemID == "" {
		itemID = fmt.Sprintf("msg:%d", ame.ContentIndex)
	}
	switch ame.Type {
	case "text_delta":
		return []ActivityEvent{{Source: "pi", Kind: "message", Status: "running", ItemID: itemID, Detail: ame.Delta}}
	case "thinking_delta":
		return []ActivityEvent{{Source: "pi", Kind: "reasoning", Status: "running", ItemID: itemID, Detail: ame.Delta}}
	case "toolcall_start":
		return []ActivityEvent{{Source: "pi", Kind: "command", Status: "running", ItemID: itemID, Title: ame.ToolName}}
	case "toolcall_delta":
		return []ActivityEvent{{Source: "pi", Kind: "command", Status: "running", ItemID: itemID, Title: ame.ToolName, Detail: ame.Delta}}
	}
	return nil
}

func piMessageText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var m piMessage
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	var parts []string
	for _, c := range m.Content {
		switch c.Type {
		case "text":
			if c.Text != "" {
				parts = append(parts, c.Text)
			}
		case "thinking":
			if c.Thinking != "" {
				parts = append(parts, c.Thinking)
			}
		case "toolCall":
			parts = append(parts, "tool call: "+c.Name)
		}
	}
	return strings.Join(parts, "\n")
}
