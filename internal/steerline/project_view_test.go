package steerline

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
	productquery "github.com/erranliu/SteerLine/internal/product/query"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

func TestProjectViewDoesNotOwnServiceRuntime(t *testing.T) {
	ctx := context.Background()
	service := controlReviewService(t)
	view, err := (productquery.NewService(fixtureStoreFor(t, service), service.Config.Identity())).ReadProjectView(ctx, "", 500)
	require(t, err)
	requireProjectView(t, view)
	connection := ConnectService(ctx, service.Config)
	if connection.State != ConnectionConnected || connection.Snapshot.Runtime == nil {
		t.Fatalf("host failed to read its service observation: %+v", connection)
	}
	// The product query result is independent from subsequent service checkpoint writes.
	service.State.Service.Ready = false
	require(t, publishFixtureCheckpoint(service))
	if current := ConnectService(ctx, service.Config); current.State != ConnectionInitializing {
		t.Fatal("service observer ignored the latest service checkpoint")
	}
	unknown := connectServiceFromSnapshot(ctx, service.Config, connection.Snapshot, func(*executionbackend.ProcessIdentity) productapi.ProcessProbe {
		return productapi.ProcessProbe{State: ProcessUnknown, Reason: "unavailable"}
	})
	if unknown.State != ConnectionUnknown || unknown.CanControl {
		t.Fatal("sampled runtime replaced the independent process check")
	}
}

func TestProjectViewFreshnessChecksUseHostSnapshot(t *testing.T) {
	ctx := context.Background()
	service := controlReviewService(t)
	view, err := (productquery.NewService(fixtureStoreFor(t, service), service.Config.Identity())).ReadProjectView(ctx, "", 500)
	require(t, err)
	requireProjectView(t, view)
	runtime, err := LoadDatabaseRuntime(service.Config)
	require(t, err)
	for _, test := range []struct {
		name      string
		heartbeat time.Time
		sampledAt time.Time
		want      productapi.ServiceConnectionState
		clockSkew bool
	}{
		{name: "expired since sampling", heartbeat: now().Add(-time.Hour), sampledAt: now().Add(-time.Hour).Add(time.Second), want: ConnectionStale},
		{name: "heartbeat after sampling began", heartbeat: now(), sampledAt: now().Add(-time.Second), want: ConnectionConnected},
		{name: "future heartbeat", heartbeat: now().Add(time.Hour), sampledAt: now(), want: ConnectionStale, clockSkew: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			copy := *runtime
			copy.HeartbeatAt = test.heartbeat
			observation := ObserveRuntimeSnapshot(service.Config, &copy, nil, test.sampledAt)
			connection := ConnectServiceFromSnapshot(ctx, service.Config, observation)
			if connection.State != test.want || connection.Snapshot.ClockSkew != test.clockSkew ||
				!connection.Snapshot.SampledAt.Equal(test.sampledAt) || connection.Snapshot.Runtime != &copy ||
				connection.CanControl != (test.want == ConnectionConnected) {
				t.Fatalf("connection trusted an outdated freshness classification: %+v", connection)
			}
		})
	}
	observation := ObserveRuntimeSnapshot(service.Config, runtime, nil, view.SampledAt)
	invalid := service.Config
	invalid.Git.Executable = filepath.Join(t.TempDir(), "missing-git")
	connection := ConnectServiceFromSnapshot(ctx, invalid, observation)
	if connection.State != ConnectionIdentityError || connection.CanControl || connection.CanStart || connection.Snapshot.Runtime != observation.Runtime {
		t.Fatal("failed identity check discarded the supplied read-only snapshot")
	}
}

func requireProjectView(t *testing.T, view storageports.ProjectView) {
	t.Helper()
	for _, err := range []error{view.TaskError, view.SelectedTaskError, view.HistoryError} {
		require(t, err)
	}
	if view.Tasks == nil || view.Ready == nil || view.History == nil {
		t.Fatal("project view omitted a successful query section")
	}
	if view.Tasks.DatabaseID != view.Ready.DatabaseID || view.Tasks.DatabaseRevision != view.Ready.DatabaseRevision ||
		view.Tasks.AuthorityVersion != view.Ready.AuthorityVersion || view.Tasks.GraphRevision != view.Ready.GraphRevision ||
		!view.Tasks.SampledAt.Equal(view.SampledAt) || !view.Ready.SampledAt.Equal(view.SampledAt) {
		t.Fatal("project sections have different sampling versions or times")
	}
}
