package console

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

type LogBatch struct {
	Cursor      steerline.JournalCursor
	Reset       bool
	Activity    []steerline.ActivityEvent
	Diagnostics []string
}

// fileSource reads the console's committed runtime projection and activity journal.
type fileSource struct {
	cfg steerline.Config

	mu          sync.Mutex
	normalizers map[string]steerline.Normalizer
}

func newFileSource(c steerline.Config) *fileSource {
	return &fileSource{cfg: c, normalizers: make(map[string]steerline.Normalizer)}
}

// SnapshotFromObservation reuses a runtime file snapshot already read by a
// caller while retaining the committed SQLite fallback and freshness checks.
func (f *fileSource) SnapshotFromObservation(obs steerline.ObservationSnapshot) steerline.ObservationSnapshot {
	if f.cfg.Identity() != "" {
		if databaseObs, err := f.readDatabaseProjection(); err == nil && databaseObs.Runtime != nil {
			if obs.Runtime == nil || databaseObs.Runtime.Revision > obs.Runtime.Revision || (databaseObs.Runtime.Revision == obs.Runtime.Revision && databaseObs.Runtime.HeartbeatAt.After(obs.Runtime.HeartbeatAt)) {
				databaseObs.Runtime.ProjectionError = "runtime.json is missing or stale; reading the committed SQLite projection"
				obs = databaseObs
			}
		}
	}
	return obs
}

func (f *fileSource) readDatabaseProjection() (steerline.ObservationSnapshot, error) {
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

func (f *fileSource) ReadLog(logPath string, cursor steerline.JournalCursor) (LogBatch, error) {
	if strings.TrimSpace(logPath) == "" {
		return LogBatch{}, fmt.Errorf("empty run log path")
	}
	path := logPath
	activityFile := false
	candidate := steerline.ActivityPath(logPath)
	if _, err := os.Stat(candidate); err == nil {
		path, activityFile = candidate, true
	}
	framed, err := steerline.ReadJournalBatch(path, cursor, steerline.JournalOptions{
		FromStart: !cursor.Initialized,
		Activity:  activityFile,
	})
	if err != nil {
		return LogBatch{}, err
	}
	batch := LogBatch{Cursor: framed.Cursor, Reset: framed.Reset, Diagnostics: append([]string(nil), framed.Diagnostics...)}
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

func (f *fileSource) normalizerFor(path string, cursor steerline.JournalCursor, firstRecord []byte, reset bool) steerline.Normalizer {
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

func (f *fileSource) storeNormalizer(path string, cursor steerline.JournalCursor, normalizer steerline.Normalizer) {
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
