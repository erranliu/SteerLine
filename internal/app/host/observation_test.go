package host

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	execution "github.com/erranliu/SteerLine/internal/execution"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/execution/reconstruction"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

func TestTM08Heartbeat(t *testing.T) {
	c, _ := nativeTaskFixture(t)
	c.Heartbeat = "2s"
	reader, err := openHostFixtureNativeProject(context.Background(), &c)
	if err != nil {
		t.Fatal(err)
	}
	if err = reader.Close(); err != nil {
		t.Fatal(err)
	}
	hostConfig := hostConfigForTest(c)
	db, err := openHostFixtureNativeCommandStore(context.Background(), &c)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	nowTime := time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)
	clock := func() time.Time { return nowTime }
	if got := ReadObservationSnapshot(hostConfig, clock); got.State != SnapshotNone {
		t.Fatalf("missing state=%s", got.State)
	}
	operation := executionmodel.OperationRecord{ProjectIdentity: c.Identity(), OperationID: newID(), IdempotencyKey: newID(), Kind: "test", Status: "pending", PayloadDigest: strings.Repeat("a", 64), Request: []byte(`{}`), CreatedAt: nowTime, UpdatedAt: nowTime}
	if err = transactFixture(db, context.Background(), func(tx *fixtureTransaction) error {
		_, _, putErr := tx.PutOperation(operation)
		return putErr
	}); err != nil {
		t.Fatal(err)
	}
	state := reconstruction.Runtime{SchemaVersion: storageports.WorkerFlowRuntimeSchema, Repository: reconstruction.Repository{Identity: c.Identity(), Path: c.Repository, Remote: c.Remote, Branch: c.Branch}, HeartbeatAt: nowTime.Add(-5 * time.Second), Runs: []*reconstruction.RunView{}}
	save := func() {
		t.Helper()
		if _, err := db.SaveCheckpoint(context.Background(), reconstruction.Checkpoint(state)); err != nil {
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
	loaded, err := LoadDatabaseRuntime(hostConfig)
	if err != nil || len(loaded.Operations) != 1 || loaded.Operations[0].OperationID != operation.OperationID {
		t.Fatalf("domain operation was not joined into runtime: %+v / %v", loaded, err)
	}
	if got := ReadObservationSnapshot(hostConfig, clock); got.State != SnapshotValid || got.FreshnessLimit != 15*time.Second || len(got.Runtime.Operations) != 1 {
		t.Fatalf("fresh heartbeat classification: %+v", got)
	}
	if err = (&execution.Store{Domain: db, Project: c.Identity()}).Heartbeat(); err != nil {
		t.Fatal(err)
	}
	updated, revision, err := db.LoadProjection(context.Background(), c.Identity())
	if err != nil {
		t.Fatal(err)
	}
	var beat storageports.ServiceCheckpoint
	if err = json.Unmarshal(updated, &beat); err != nil || !beat.HeartbeatAt.After(state.HeartbeatAt) || beat.Revision != revision {
		t.Fatalf("SQLite heartbeat did not update the same checkpoint: %+v / %v", beat, err)
	}
	if _, err = os.Stat(filepath.Join(c.RunDir, "runtime.json")); !os.IsNotExist(err) {
		t.Fatalf("heartbeat wrote a second runtime file: %v", err)
	}
	state.HeartbeatAt = nowTime.Add(-16 * time.Second)
	save()
	if got := ReadObservationSnapshot(hostConfig, clock); got.State != SnapshotStale {
		t.Fatalf("stale heartbeat classification: %+v", got)
	}
	state.HeartbeatAt = nowTime.Add(time.Second)
	save()
	if got := ReadObservationSnapshot(hostConfig, clock); got.State != SnapshotStale || !got.ClockSkew {
		t.Fatalf("future heartbeat classification: %+v", got)
	}
	state.SchemaVersion = 4
	checkpointValue := reconstruction.Checkpoint(state)
	checkpointValue.SchemaVersion = 4
	body, err := json.Marshal(checkpointValue)
	if err != nil {
		t.Fatal(err)
	}
	if err = transactFixture(db, context.Background(), func(tx *fixtureTransaction) error { return tx.PutProjection(c.Identity(), 99, body) }); err != nil {
		t.Fatal(err)
	}
	if got := ReadObservationSnapshot(hostConfig, clock); got.State != SnapshotUnknownVersion {
		t.Fatalf("unsupported SQLite projection classification: %+v", got)
	}
}
