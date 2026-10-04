package steerline

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
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
type ActivityEvent = contracts.ActivityEvent

func activityPath(logPath string) string {
	return filepath.Join(filepath.Dir(logPath), "activity.jsonl")
}

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
	items  map[string]uint64
}

// OpenActivity prepares the journal for append: it repairs a partial trailing
// line and recovers the last seq so ordering stays monotonic across lazy reopens
// and service restarts. No handle is retained after this call.
func OpenActivity(path, runID, taskID string) (*ActivityWriter, error) {
	seq, items, e := recoverActivityState(path)
	if e != nil {
		return nil, e
	}
	return &ActivityWriter{path: path, seq: seq, runID: runID, taskID: taskID, items: items}, nil
}

func recoverActivityState(path string) (uint64, map[string]uint64, error) {
	f, e := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return 0, nil, e
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil {
		return 0, nil, e
	}
	if info.Size() == 0 {
		return 0, map[string]uint64{}, nil
	}
	b, e := io.ReadAll(io.LimitReader(f, 64<<20))
	if e != nil {
		return 0, nil, e
	}
	if len(b) > 0 && b[len(b)-1] != '\n' {
		if i := bytes.LastIndexByte(b, '\n'); i >= 0 {
			b = b[:i+1]
		} else {
			b = nil
		}
		if e := f.Truncate(int64(len(b))); e != nil {
			return 0, nil, e
		}
	}
	var last uint64
	items := map[string]uint64{}
	for len(b) > 0 {
		i := bytes.IndexByte(b, '\n')
		if i < 0 {
			break
		}
		var ev ActivityEvent
		if json.Unmarshal(b[:i], &ev) == nil {
			if ev.Seq > last {
				last = ev.Seq
			}
			if ev.ItemID != "" {
				items[ev.ItemID] = ev.Seq
			}
		}
		b = b[i+1:]
	}
	return last, items, nil
}

func (w *ActivityWriter) Write(ev ActivityEvent) error {
	_, err := w.WriteEvent(ev)
	return err
}

func (w *ActivityWriter) WriteEvent(ev ActivityEvent) (ActivityEvent, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if ev.ItemID != "" {
		if seq, exists := w.items[ev.ItemID]; exists {
			ev.Seq = seq
			return ev, nil
		}
	}
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
		return ActivityEvent{}, e
	}
	f, e := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if e != nil {
		return ActivityEvent{}, e
	}
	_, e = f.Write(append(b, '\n'))
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return ActivityEvent{}, e
	}
	if ev.ItemID != "" {
		if w.items == nil {
			w.items = map[string]uint64{}
		}
		w.items[ev.ItemID] = ev.Seq
	}
	return ev, nil
}

// Close is a no-op: writes never retain a handle.
func (w *ActivityWriter) Close() error { return nil }
