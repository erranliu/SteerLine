package viewer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/erranliu/SteerLine/internal/steerline"
)

// Source is the read-mostly data the viewer needs. Only Answer writes, and it
// writes only the immutable decision answer file; runtime.json is never touched.
type Source interface {
	Snapshot() steerline.ObservationSnapshot
	ReadLog(logPath string, cursor steerline.JournalCursor, raw bool) (LogBatch, error)
	TaskDetail(taskID string) (steerline.TaskDetail, error)
	Answer(run *steerline.Run, optionID, note, by string) error
}

type LogBatch struct {
	Cursor      steerline.JournalCursor
	Reset       bool
	Activity    []steerline.ActivityEvent
	Raw         []string
	Diagnostics []string
}

// FileSource reads the same journal framing used by `steerline watch`.
type FileSource struct {
	cfg steerline.Config

	mu          sync.Mutex
	last        *steerline.ObservationSnapshot
	normalizers map[string]steerline.Normalizer
}

func NewFileSource(c steerline.Config) *FileSource {
	return &FileSource{cfg: c, normalizers: make(map[string]steerline.Normalizer)}
}

func (f *FileSource) Snapshot() steerline.ObservationSnapshot {
	obs := steerline.ReadObservationSnapshot(filepath.Join(f.cfg.RunDir, f.cfg.RuntimeFile), f.cfg.Heartbeat, nil)
	f.mu.Lock()
	defer f.mu.Unlock()
	if obs.Runtime != nil && (obs.State == steerline.SnapshotValid || obs.State == steerline.SnapshotStale) {
		copy := cloneRuntime(obs.Runtime)
		obs.Runtime = copy
		stored := obs
		f.last = &stored
		return obs
	}
	if f.last != nil && (obs.State == steerline.SnapshotReadFailed || obs.State == steerline.SnapshotUnknownVersion || obs.State == steerline.SnapshotNone) {
		observedAt := obs.SampledAt
		if obs.State == steerline.SnapshotNone {
			obs.State = steerline.SnapshotReadFailed
			obs.Error = "runtime snapshot disappeared; showing the last successful snapshot"
		}
		obs.Runtime = cloneRuntime(f.last.Runtime)
		obs.SampledAt = f.last.SampledAt
		if obs.Runtime != nil && !obs.Runtime.HeartbeatAt.IsZero() {
			obs.HeartbeatAge = observedAt.Sub(obs.Runtime.HeartbeatAt.UTC())
			obs.ClockSkew = obs.HeartbeatAge < 0
			if obs.ClockSkew {
				obs.HeartbeatAge = 0
			}
		}
	}
	return obs
}

func cloneRuntime(rt *steerline.Runtime) *steerline.Runtime {
	if rt == nil {
		return nil
	}
	b, err := json.Marshal(rt)
	if err != nil {
		return rt
	}
	var copy steerline.Runtime
	if json.Unmarshal(b, &copy) != nil {
		return rt
	}
	return &copy
}

func (f *FileSource) TaskDetail(taskID string) (steerline.TaskDetail, error) {
	return steerline.LoadTaskDetail(context.Background(), f.cfg, taskID)
}

func (f *FileSource) ReadLog(logPath string, cursor steerline.JournalCursor, raw bool) (LogBatch, error) {
	if strings.TrimSpace(logPath) == "" {
		return LogBatch{}, fmt.Errorf("empty run log path")
	}
	path := logPath
	activityFile := false
	if !raw {
		candidate := steerline.ActivityPath(logPath)
		if _, err := os.Stat(candidate); err == nil {
			path, activityFile = candidate, true
		}
	}
	framed, err := steerline.ReadJournalBatch(path, cursor, steerline.JournalOptions{
		FromStart: !cursor.Initialized,
		Activity:  activityFile,
		KeepCR:    raw,
	})
	if err != nil {
		return LogBatch{}, err
	}
	batch := LogBatch{Cursor: framed.Cursor, Reset: framed.Reset, Diagnostics: append([]string(nil), framed.Diagnostics...)}
	if raw {
		for _, line := range framed.Lines {
			batch.Raw = append(batch.Raw, string(line))
		}
		return batch, nil
	}
	if activityFile {
		for _, line := range framed.Lines {
			ev, err := steerline.DecodeActivityLine(line)
			if err != nil {
				batch.Diagnostics = append(batch.Diagnostics, fmt.Sprintf("invalid activity JSON: %v", err))
				continue
			}
			batch.Activity = append(batch.Activity, ev)
		}
		return batch, nil
	}
	var firstLine []byte
	if len(framed.Lines) > 0 {
		firstLine = framed.Lines[0]
	}
	normalizer := f.normalizerFor(path, framed.Cursor, firstLine, framed.Reset)
	for _, line := range framed.Lines {
		if normalizer == nil {
			normalizer = steerline.DetectNormalizer(line)
			f.storeNormalizer(path, framed.Cursor, normalizer)
		}
		batch.Activity = append(batch.Activity, steerline.NormalizeFallbackLine(normalizer, line, &framed.Cursor)...)
	}
	batch.Cursor = framed.Cursor
	return batch, nil
}

func (f *FileSource) normalizerFor(path string, cursor steerline.JournalCursor, firstRecord []byte, reset bool) steerline.Normalizer {
	key := fmt.Sprintf("%s|%s|%d", path, cursor.FileID, cursor.Generation)
	f.mu.Lock()
	defer f.mu.Unlock()
	if reset {
		delete(f.normalizers, key)
	}
	if n := f.normalizers[key]; n != nil {
		return n
	}
	first, err := steerline.FirstJournalLine(path)
	if err == nil && len(first) == 0 {
		first = firstRecord
	}
	if err != nil && len(firstRecord) > 0 {
		first, err = firstRecord, nil
	}
	if err != nil {
		first = nil
	}
	if len(first) == 0 {
		return nil
	}
	n := steerline.DetectNormalizer(first)
	if len(f.normalizers) >= 64 {
		f.normalizers = make(map[string]steerline.Normalizer)
	}
	f.normalizers[key] = n
	return n
}

func (f *FileSource) storeNormalizer(path string, cursor steerline.JournalCursor, normalizer steerline.Normalizer) {
	if normalizer == nil {
		return
	}
	key := fmt.Sprintf("%s|%s|%d", path, cursor.FileID, cursor.Generation)
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.normalizers) >= 64 {
		f.normalizers = make(map[string]steerline.Normalizer)
	}
	f.normalizers[key] = normalizer
}

func (f *FileSource) Answer(run *steerline.Run, optionID, note, by string) error {
	if by == "" {
		by = steerline.AnswererName()
	}
	return steerline.WriteAnswer(run, steerline.DecisionAnswer{OptionID: optionID, Note: note, AnsweredBy: by})
}
