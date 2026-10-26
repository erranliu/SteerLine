package app

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/erranliu/SteerLine/internal/app/host"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	"github.com/erranliu/SteerLine/internal/projectconnection"
	"github.com/erranliu/SteerLine/internal/runnerclient"
)

type startupCountingHost struct {
	executionbackend.ManagedProcessHost
	calls int
}

func (h *startupCountingHost) StartManaged(cmd *exec.Cmd) (executionbackend.StopHandle, error) {
	h.calls++
	return h.ManagedProcessHost.StartManaged(cmd)
}

func TestStartupQueriesRevalidateOwnershipAndDoNotEscapeToLiveCommands(t *testing.T) {
	c, _ := nativeTaskFixture(t)
	h := &startupCountingHost{ManagedProcessHost: c.ProcessHost}
	c.ProcessHost = h
	check := func() {
		t.Helper()
		require(t, c.ResolveIdentity(context.Background()))
		require(t, c.ResolveIdentity(context.Background()))
		_, err := c.TaskRepositoryTarget(context.Background())
		require(t, err)
		_, err = c.TaskRepositoryTarget(context.Background())
		require(t, err)
		require(t, ensureRunDirOutsideRepository(context.Background(), c))
		require(t, ensureRunDirOutsideRepository(context.Background(), c))
	}
	c.startup = newStartupQueries(context.Background())
	check()
	if h.calls != 4 {
		t.Fatalf("repeated startup checks launched %d Git processes, want 4", h.calls)
	}
	c.startup = newStartupQueries(context.Background())
	check()
	if h.calls != 8 {
		t.Fatalf("ownership boundary reused stale evidence: %d", h.calls)
	}
	c.startup = nil
	require(t, c.ResolveIdentity(context.Background()))
	require(t, c.ResolveIdentity(context.Background()))
	if h.calls != 12 {
		t.Fatalf("live command queries were cached: %d", h.calls)
	}
	// Host callbacks copy Config by value during preparation. Finishing the
	// phase must retire their shared query scope as well as the owner's pointer.
	ctx, cancel := context.WithCancel(context.Background())
	c.startup = newStartupQueries(ctx)
	require(t, c.ResolveIdentity(ctx))
	borrowed := c
	c.finishStartupQueries()
	cancel()
	require(t, borrowed.ResolveIdentity(context.Background()))
	if h.calls != 16 {
		t.Fatalf("copied config retained startup queries: %d", h.calls)
	}
}

func TestStartupQueriesHonorCancellationAndChangedRepository(t *testing.T) {
	c, _ := nativeTaskFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	c.startup = newStartupQueries(ctx)
	require(t, c.ResolveIdentity(ctx))
	cancel()
	if err := c.ResolveIdentity(context.Background()); !errors.Is(err, context.Canceled) {
		t.Fatal("cached identity ignored cancellation", err)
	}
	if _, err := c.startupGitOutput(context.Background(), "config", "--get", "remote.origin.url"); !errors.Is(err, context.Canceled) {
		t.Fatal("query ignored startup cancellation", err)
	}
	c.startup = newStartupQueries(context.Background())
	require(t, c.ResolveIdentity(context.Background()))
	c.Repository = projectRepo(t)
	if c.ResolveIdentity(context.Background()) == nil {
		t.Fatal("changed repository reused original identity")
	}
}

func TestStartupQueriesKeepRemoteAndDatabaseAdmissionChecks(t *testing.T) {
	c, _ := nativeTaskFixture(t)
	store, err := OpenNativeTaskProject(context.Background(), &c)
	require(t, err)
	require(t, store.Close())
	c.startup = newStartupQueries(context.Background())
	reader, err := OpenNativeTaskReader(context.Background(), &c)
	require(t, err)
	require(t, reader.Close())
	_, err = c.TaskRepositoryTarget(context.Background())
	require(t, err)
	c.Remote = "missing"
	if _, err = c.TaskRepositoryTarget(context.Background()); err == nil {
		t.Fatal("different remote reused cached target")
	}
	c.Remote = "origin"
	alias := filepath.Join(t.TempDir(), "linked.db")
	if err = os.Link(filepath.Join(c.RunDir, "steerline.db"), alias); err != nil {
		t.Skipf("hard links unavailable: %v", err)
	}
	reader, err = OpenNativeTaskReader(context.Background(), &c)
	if reader != nil {
		reader.Close()
	}
	if err == nil {
		t.Fatal("cached Git evidence bypassed database hard-link admission")
	}
}

func TestStartupParallelRecoveryFailureReleasesAllProjectLocks(t *testing.T) {
	c, _ := nativeTaskFixture(t)
	for _, name := range []string{"one", "two"} {
		other := c
		other.Repository = filepath.Join(t.TempDir(), name)
		fixtureGit(t, HomeDir(), "clone", "--quiet", "--no-hardlinks", c.Repository, other.Repository)
		fixtureGit(t, other.Repository, "remote", "set-url", "origin", "https://github.com/owner/repo.git")
		other.RunDir, other.identity = "", ""
		writeProjectConfig(t, name, other)
		loaded, err := LoadConfig(filepath.Join(EtcDir(), name+".json"))
		require(t, err)
		store, err := OpenNativeTaskProject(context.Background(), &loaded)
		require(t, err)
		require(t, store.Close())
		if name == "two" {
			other.PR.Repository = "foreign/repository"
			writeProjectConfig(t, name, other)
		}
	}
	d := NewApplicationDirectory(HomeDir())
	for _, entry := range d.entries {
		require(t, projectconnection.ResolveSnapshot(context.Background(), &entry.ref, entry.data))
	}
	if _, err := d.prepareRunnerStops(context.Background(), &runnerclient.Client{}); err == nil {
		t.Fatal("invalid project did not block Runner admission")
	}
	for _, entry := range d.entries {
		for _, path := range []string{entry.ref.Identity, entry.ref.RunDir} {
			lock, err := host.LockRepository(path)
			require(t, err)
			lock.Close()
		}
		if entry.runtime != nil {
			t.Fatal("failed recovery started project runtime")
		}
	}
}
