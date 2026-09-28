package viewer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline"
)

// Source is the read-mostly data the viewer needs. Answers go through the
// versioned command queue for the current database format.
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
	// LoadConfig intentionally performs no repository probing. Resolve the
	// read-only identity here so the normal viewer entry can use SQLite too.
	if c.Identity() == "" && c.Repository != "" {
		_ = c.ResolveIdentity(context.Background())
	}
	return &FileSource{cfg: c, normalizers: make(map[string]steerline.Normalizer)}
}

func (f *FileSource) Snapshot() steerline.ObservationSnapshot {
	return f.SnapshotFromObservation(steerline.ReadObservationSnapshot(filepath.Join(f.cfg.RunDir, f.cfg.RuntimeFile), f.cfg.Heartbeat, nil))
}

// SnapshotFromObservation reuses a runtime file snapshot already read by a
// caller while retaining the committed SQLite fallback and freshness checks.
func (f *FileSource) SnapshotFromObservation(obs steerline.ObservationSnapshot) steerline.ObservationSnapshot {
	if f.cfg.Identity() != "" {
		if databaseObs, err := f.readDatabaseProjection(); err == nil && databaseObs.Runtime != nil {
			if obs.Runtime == nil || databaseObs.Runtime.Revision > obs.Runtime.Revision || (databaseObs.Runtime.Revision == obs.Runtime.Revision && databaseObs.Runtime.HeartbeatAt.After(obs.Runtime.HeartbeatAt)) {
				databaseObs.Runtime.ProjectionError = "runtime.json is missing or stale; reading the committed SQLite projection"
				obs = databaseObs
			}
		}
	}
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

func (f *FileSource) readDatabaseProjection() (steerline.ObservationSnapshot, error) {
	path := filepath.Join(f.cfg.RunDir, "steerline.db")
	if _, err := os.Stat(path); err != nil {
		return steerline.ObservationSnapshot{}, err
	}
	store, err := steerline.OpenSQLiteReader(path)
	if err != nil {
		return steerline.ObservationSnapshot{}, err
	}
	defer store.Close()
	payload, _, err := store.LoadProjection(context.Background(), f.cfg.Identity())
	if err != nil {
		return steerline.ObservationSnapshot{}, err
	}
	if len(payload) == 0 {
		return steerline.ObservationSnapshot{}, errors.New("SQLite projection is missing")
	}
	var runtime steerline.Runtime
	if err = json.Unmarshal(payload, &runtime); err != nil {
		return steerline.ObservationSnapshot{}, err
	}
	if runtime.SchemaVersion != 5 {
		return steerline.ObservationSnapshot{}, fmt.Errorf("unsupported database projection schema %d", runtime.SchemaVersion)
	}
	if runtime.Repository.Identity != f.cfg.Identity() {
		return steerline.ObservationSnapshot{}, errors.New("database projection repository identity mismatch")
	}
	now := time.Now().UTC()
	limit := 15 * time.Second
	if duration, parseErr := time.ParseDuration(f.cfg.Heartbeat); parseErr == nil && duration > 0 && 3*duration > limit {
		limit = 3 * duration
	}
	obs := steerline.ObservationSnapshot{Runtime: &runtime, State: steerline.SnapshotValid, SampledAt: now, FreshnessLimit: limit}
	if runtime.HeartbeatAt.IsZero() {
		obs.State = steerline.SnapshotStale
		return obs, nil
	}
	obs.HeartbeatAge = now.Sub(runtime.HeartbeatAt.UTC())
	if obs.HeartbeatAge < 0 {
		obs.ClockSkew = true
		obs.State = steerline.SnapshotStale
		return obs, nil
	}
	if obs.HeartbeatAge > limit {
		obs.State = steerline.SnapshotStale
	}
	return obs, nil
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
	if run == nil {
		return fmt.Errorf("run is required")
	}
	if by == "" {
		by = steerline.AnswererName()
	}
	decisionID := ""
	var version uint64
	if run.Decision != nil {
		decisionID = run.Decision.ID
		version = run.Decision.Version
	}
	return steerline.AnswerDecision(f.cfg, steerline.AnswerOptions{RunID: run.RunID, DecisionID: decisionID, ExpectedVersion: version, Option: optionID, Note: note, By: by})
}
