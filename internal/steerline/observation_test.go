package steerline

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestObservationReadsOneSQLiteRuntimeAndClassifiesHeartbeat(t *testing.T) {
	c, _ := nativeTaskFixture(t)
	c.Heartbeat = "2s"
	reader, err := OpenNativeTaskProject(context.Background(), &c)
	if err != nil {
		t.Fatal(err)
	}
	if err = reader.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := OpenNativeTaskCommandStore(context.Background(), &c)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	nowTime := time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)
	clock := func() time.Time { return nowTime }
	if got := ReadObservationSnapshot(c, clock); got.State != SnapshotNone {
		t.Fatalf("missing state=%s", got.State)
	}
	operation := OperationRecord{ProjectIdentity: c.Identity(), OperationID: newID(), IdempotencyKey: newID(), Kind: "test", Status: "pending", PayloadDigest: strings.Repeat("a", 64), Request: []byte(`{}`), CreatedAt: nowTime, UpdatedAt: nowTime}
	if err = db.transactSQLite(context.Background(), func(tx *sqliteTx) error {
		_, _, putErr := tx.PutOperation(operation)
		return putErr
	}); err != nil {
		t.Fatal(err)
	}
	state := Runtime{SchemaVersion: WorkerFlowRuntimeSchema, Repository: Repository{Identity: c.Identity(), Path: c.Repository, Remote: c.Remote, Branch: c.Branch}, HeartbeatAt: nowTime.Add(-5 * time.Second), Runs: []*Run{}}
	save := func() {
		t.Helper()
		if _, err := db.SaveCheckpoint(context.Background(), runtimeCheckpoint(state)); err != nil {
			t.Fatal(err)
		}
	}
	save()
	payload, _, err := db.LoadProjection(context.Background(), c.Identity())
	if err != nil {
		t.Fatal(err)
	}
	var checkpoint map[string]json.RawMessage
	if err = json.Unmarshal(payload, &checkpoint); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"runs", "current_task", "slot_occupied", "control", "tasks", "workflow_runs", "executions", "attempts", "decisions", "operations"} {
		if _, duplicated := checkpoint[key]; duplicated {
			t.Fatalf("SQLite checkpoint duplicates %s domain rows", key)
		}
	}
	loaded, err := LoadDatabaseRuntime(c)
	if err != nil || len(loaded.Operations) != 1 || loaded.Operations[0].OperationID != operation.OperationID {
		t.Fatalf("domain operation was not joined into runtime: %+v / %v", loaded, err)
	}
	if got := ReadObservationSnapshot(c, clock); got.State != SnapshotValid || got.FreshnessLimit != 15*time.Second || len(got.Runtime.Operations) != 1 {
		t.Fatalf("fresh heartbeat classification: %+v", got)
	}
	if err = (&Store{Domain: db, Project: c.Identity()}).Heartbeat(); err != nil {
		t.Fatal(err)
	}
	updated, revision, err := db.LoadProjection(context.Background(), c.Identity())
	if err != nil {
		t.Fatal(err)
	}
	var beat ServiceCheckpoint
	if err = json.Unmarshal(updated, &beat); err != nil || !beat.HeartbeatAt.After(state.HeartbeatAt) || beat.Revision != revision {
		t.Fatalf("SQLite heartbeat did not update the same checkpoint: %+v / %v", beat, err)
	}
	if _, err = os.Stat(filepath.Join(c.RunDir, "runtime.json")); !os.IsNotExist(err) {
		t.Fatalf("heartbeat wrote a second runtime file: %v", err)
	}
	state.HeartbeatAt = nowTime.Add(-16 * time.Second)
	save()
	if got := ReadObservationSnapshot(c, clock); got.State != SnapshotStale {
		t.Fatalf("stale heartbeat classification: %+v", got)
	}
	state.HeartbeatAt = nowTime.Add(time.Second)
	save()
	if got := ReadObservationSnapshot(c, clock); got.State != SnapshotStale || !got.ClockSkew {
		t.Fatalf("future heartbeat classification: %+v", got)
	}
	state.SchemaVersion = 4
	checkpointValue := runtimeCheckpoint(state)
	checkpointValue.SchemaVersion = 4
	body, err := json.Marshal(checkpointValue)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.transactSQLite(context.Background(), func(tx *sqliteTx) error { return tx.PutProjection(c.Identity(), 99, body) }); err != nil {
		t.Fatal(err)
	}
	if got := ReadObservationSnapshot(c, clock); got.State != SnapshotUnknownVersion {
		t.Fatalf("unsupported SQLite projection classification: %+v", got)
	}
}
