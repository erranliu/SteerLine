package steerline

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
)

func TestProjectViewConnectionUsesSampledRuntime(t *testing.T) {
	ctx := context.Background()
	service := controlReviewService(t)
	view, err := (NewQueryService(fixtureStoreFor(t, service), service.Config.Identity())).ReadProjectView(ctx, "", 500)
	require(t, err)
	requireProjectView(t, view)
	observation := ObserveRuntimeSnapshot(service.Config, view.Runtime, view.RuntimeError, view.SampledAt)
	// A subsequent checkpoint must not be read by connection classification.
	service.State.Service.Ready = false
	require(t, publishFixtureCheckpoint(service))
	connection := ConnectServiceFromSnapshot(ctx, service.Config, observation)
	if connection.State != ConnectionConnected || connection.Snapshot.Runtime != view.Runtime || !connection.Snapshot.SampledAt.Equal(view.SampledAt) {
		t.Fatalf("connection re-read the database instead of using the project sample: %+v", connection)
	}
	if current := ConnectService(ctx, service.Config); current.State != ConnectionInitializing {
		t.Fatal("fixture did not publish a different service checkpoint")
	}
	unknown := connectServiceFromSnapshot(ctx, service.Config, observation, func(*executionbackend.ProcessIdentity) productapi.ProcessProbe {
		return productapi.ProcessProbe{State: ProcessUnknown, Reason: "unavailable"}
	})
	if unknown.State != ConnectionUnknown || unknown.CanControl {
		t.Fatal("sampled runtime replaced the independent process check")
	}
}

func TestProjectViewConnectionRechecksHeartbeatAtUse(t *testing.T) {
	ctx := context.Background()
	service := controlReviewService(t)
	view, err := (NewQueryService(fixtureStoreFor(t, service), service.Config.Identity())).ReadProjectView(ctx, "", 500)
	require(t, err)
	requireProjectView(t, view)
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
			runtime := *view.Runtime
			runtime.HeartbeatAt = test.heartbeat
			observation := ObserveRuntimeSnapshot(service.Config, &runtime, nil, test.sampledAt)
			connection := ConnectServiceFromSnapshot(ctx, service.Config, observation)
			if connection.State != test.want || connection.Snapshot.ClockSkew != test.clockSkew ||
				!connection.Snapshot.SampledAt.Equal(test.sampledAt) || connection.Snapshot.Runtime != &runtime ||
				connection.CanControl != (test.want == ConnectionConnected) {
				t.Fatalf("connection trusted an outdated freshness classification: %+v", connection)
			}
		})
	}
	observation := ObserveRuntimeSnapshot(service.Config, view.Runtime, nil, view.SampledAt)
	invalid := service.Config
	invalid.Git.Executable = filepath.Join(t.TempDir(), "missing-git")
	connection := ConnectServiceFromSnapshot(ctx, invalid, observation)
	if connection.State != ConnectionIdentityError || connection.CanControl || connection.CanStart || connection.Snapshot.Runtime != observation.Runtime {
		t.Fatal("failed identity check discarded the supplied read-only snapshot")
	}
}

func requireProjectView(t *testing.T, view ProjectView) {
	t.Helper()
	for _, err := range []error{view.TaskError, view.SelectedTaskError, view.DomainError, view.RuntimeError} {
		require(t, err)
	}
	if view.Tasks == nil || view.Ready == nil || view.Domain == nil {
		t.Fatal("project view omitted a successful query section")
	}
	if view.Tasks.DatabaseID != view.Ready.DatabaseID || view.Tasks.DatabaseRevision != view.Ready.DatabaseRevision ||
		view.Tasks.AuthorityVersion != view.Ready.AuthorityVersion || view.Tasks.GraphRevision != view.Ready.GraphRevision ||
		!view.Tasks.SampledAt.Equal(view.SampledAt) || !view.Ready.SampledAt.Equal(view.SampledAt) {
		t.Fatal("project sections have different sampling versions or times")
	}
}
