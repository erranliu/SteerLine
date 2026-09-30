package codexworker

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
)

const activityTitleMax = 240

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

type Normalizer struct {
	started map[string]time.Time
}

func NewNormalizer() *Normalizer { return &Normalizer{started: map[string]time.Time{}} }

func (n *Normalizer) Name() string { return "codex" }

func (n *Normalizer) Detect(line []byte) bool {
	var probe struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(line, &probe) != nil {
		return false
	}
	return probe.Type == "thread.started"
}

func (n *Normalizer) Normalize(line []byte) []contracts.ActivityEvent {
	var e codexExecEvent
	if json.Unmarshal(line, &e) != nil {
		return []contracts.ActivityEvent{{Source: "codex", Kind: "unknown", Title: "unparsed raw event", Raw: cloneRaw(line)}}
	}
	raw := cloneRaw(line)
	switch e.Type {
	case "thread.started":
		return []contracts.ActivityEvent{{Source: "codex", Kind: "session", Status: "info", Title: "codex session", Meta: map[string]any{"thread_id": e.ThreadID}}}
	case "turn.started":
		return []contracts.ActivityEvent{{Source: "codex", Kind: "turn", Status: "started"}}
	case "turn.completed":
		return []contracts.ActivityEvent{{Source: "codex", Kind: "usage", Status: "completed", Meta: map[string]any{"usage": e.Usage}}}
	case "turn.failed":
		msg := ""
		if e.Error != nil {
			msg = e.Error.Message
		}
		return []contracts.ActivityEvent{{Source: "codex", Kind: "error", Status: "failed", Title: "turn failed", Detail: msg, Raw: raw}}
	case "error":
		return []contracts.ActivityEvent{{Source: "codex", Kind: "error", Status: "failed", Detail: e.Message, Raw: raw}}
	case "item.started", "item.updated", "item.completed":
		return []contracts.ActivityEvent{n.item(e, raw)}
	}
	return []contracts.ActivityEvent{{Source: "codex", Kind: "unknown", Title: e.Type, Raw: raw}}
}

func (n *Normalizer) item(e codexExecEvent, raw json.RawMessage) contracts.ActivityEvent {
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
	ev := contracts.ActivityEvent{Source: "codex", ItemID: it.ID, Status: status, ElapsedMS: elapsed}
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

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

type ExecLineFact struct {
	SessionID string
	Type      string
	Summary   string
}

func DecodeExecLine(line []byte) (ExecLineFact, error) {
	var event struct {
		Type     string `json:"type"`
		ThreadID string `json:"thread_id"`
		Item     struct {
			Type string `json:"type"`
		} `json:"item"`
	}
	if err := json.Unmarshal(line, &event); err != nil {
		return ExecLineFact{}, err
	}
	fact := ExecLineFact{Type: event.Type, Summary: clip(event.Type+" "+event.Item.Type, 512)}
	if event.Type == "thread.started" {
		fact.SessionID = event.ThreadID
	}
	return fact, nil
}
