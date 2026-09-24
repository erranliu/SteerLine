package steerline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeRuntimeForObservation(t *testing.T, path string, version int, heartbeat time.Time) {
	t.Helper()
	b, err := json.Marshal(Runtime{SchemaVersion: version, HeartbeatAt: heartbeat, Service: ServiceState{Status: "running"}, Runs: []*Run{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestObservationClassifiesMissingFreshStaleFutureAndUnknownRuntime(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.json")
	now := time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	if got := ReadObservationSnapshot(path, "2s", clock); got.State != SnapshotNone {
		t.Fatalf("missing state=%s", got.State)
	}
	writeRuntimeForObservation(t, path, 5, now.Add(-5*time.Second))
	if got := ReadObservationSnapshot(path, "2s", clock); got.State != SnapshotValid || got.FreshnessLimit != 15*time.Second {
		t.Fatalf("fresh heartbeat classification: %+v", got)
	}
	writeRuntimeForObservation(t, path, 5, now.Add(-16*time.Second))
	if got := ReadObservationSnapshot(path, "2s", clock); got.State != SnapshotStale {
		t.Fatalf("stale heartbeat classification: %+v", got)
	}
	writeRuntimeForObservation(t, path, 5, now.Add(time.Second))
	got := ReadObservationSnapshot(path, "2s", clock)
	if got.State != SnapshotStale || !got.ClockSkew {
		t.Fatalf("future timestamp should report clock skew and stale: %+v", got)
	}
	writeRuntimeForObservation(t, path, 4, now)
	if got := ReadObservationSnapshot(path, "2s", clock); got.State != SnapshotUnknownVersion {
		t.Fatalf("old version classification: %+v", got)
	}
	writeRuntimeForObservation(t, path, 99, now)
	if got := ReadObservationSnapshot(path, "2s", clock); got.State != SnapshotUnknownVersion {
		t.Fatalf("unknown version classification: %+v", got)
	}
	writeRuntimeForObservation(t, path, 4, now)
	if got := ReadObservationSnapshot(path, "2s", clock); got.State != SnapshotUnknownVersion {
		t.Fatalf("v4 runtime should be rejected: %+v", got)
	}
}
