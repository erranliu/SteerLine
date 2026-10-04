package steerlineclient

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
	api "github.com/erranliu/SteerLine/internal/steerline/clientcontracts"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func TestProjectSnapshotReuseRequiresSameLiveIdentityAndDatabaseVersion(t *testing.T) {
	now := time.Now()
	ref := api.ProjectRef{Identity: "project", ConfigPath: "config", RunDir: "state"}
	process := &executionbackend.ProcessIdentity{PID: 42}
	runtime := &contracts.Runtime{Repository: contracts.Repository{Identity: ref.Identity}, Service: contracts.ServiceState{ProcessIdentity: process}, HeartbeatAt: now}
	previous := api.ProjectSnapshot{
		SelectedTaskID: "14", FullReadAt: now,
		Connection:  api.ServiceConnection{State: productapi.ConnectionConnected, Snapshot: api.ObservationSnapshot{Runtime: runtime}},
		Observation: api.ObservationSnapshot{Runtime: runtime, FreshnessLimit: 15 * time.Second},
		NativeTasks: &contracts.NativeTaskPage{ProjectID: ref.Identity, DatabaseID: "database", DatabaseRevision: 7},
		Tasks:       &contracts.TaskSnapshot{}, Domain: &contracts.DomainSnapshot{},
	}
	probe := func(*executionbackend.ProcessIdentity) productapi.ProcessProbe {
		return productapi.ProcessProbe{State: productapi.ProcessMatches}
	}
	version := func(context.Context, string) (string, uint64, error) { return "database", 7, nil }
	if !ReusableProjectSnapshot(context.Background(), ref, "14", previous, probe, version) {
		t.Fatal("unchanged verified project was fully reloaded")
	}
	for _, changed := range []struct {
		name     string
		selected string
		snapshot api.ProjectSnapshot
		probe    func(*executionbackend.ProcessIdentity) productapi.ProcessProbe
		version  func(context.Context, string) (string, uint64, error)
	}{
		{name: "selection changed", selected: "15", snapshot: previous, probe: probe, version: version},
		{name: "database revision changed", selected: "14", snapshot: previous, probe: probe, version: func(context.Context, string) (string, uint64, error) { return "database", 8, nil }},
		{name: "process exited", selected: "14", snapshot: previous, probe: func(*executionbackend.ProcessIdentity) productapi.ProcessProbe {
			return productapi.ProcessProbe{State: productapi.ProcessAbsent}
		}, version: version},
		{name: "database probe failed", selected: "14", snapshot: previous, probe: probe, version: func(context.Context, string) (string, uint64, error) { return "", 0, errors.New("unavailable") }},
		{name: "full check expired", selected: "14", snapshot: func() api.ProjectSnapshot { old := previous; old.FullReadAt = now.Add(-6 * time.Second); return old }(), probe: probe, version: version},
	} {
		t.Run(changed.name, func(t *testing.T) {
			if ReusableProjectSnapshot(context.Background(), ref, changed.selected, changed.snapshot, changed.probe, changed.version) {
				t.Fatal("stale or mismatched project snapshot was reused")
			}
		})
	}
}

func TestNativeCandidateSnapshotKeepsDispatchOrderBeyondDetailPage(t *testing.T) {
	now := time.Now().UTC()
	connection := api.ServiceConnection{State: productapi.ConnectionConnected}
	ready := contracts.NativeReadyTaskSnapshot{SampledAt: now}
	for i := 501; i >= 1; i-- {
		ready.Tasks = append(ready.Tasks, taskmodel.TaskRecord{TaskID: fmt.Sprint(i), Title: "ready", Type: "task", Status: "open"})
	}
	snapshot := NativeCandidates("project", []string{"task"}, connection, ready)
	if len(snapshot.Tasks) != 501 || snapshot.Tasks[0].TaskID != "501" || snapshot.Tasks[500].TaskID != "1" {
		t.Fatalf("ready candidates were truncated or reordered by task ID: %d", len(snapshot.Tasks))
	}
}
