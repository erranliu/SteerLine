//go:build startup_timing

package app

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/projectconnection"
	"github.com/erranliu/SteerLine/internal/runnerclient"
)

// This opt-in diagnostic uses three independent repositories and real stores.
// The same test can be built with an overlay of the original implementation.
func TestStartupPreparationTiming(t *testing.T) {
	d := startupTimingDirectory(t)
	measureStartupPreparation(t, d)
}

func startupTimingDirectory(t *testing.T) *ApplicationDirectory {
	c, _ := nativeTaskFixture(t)
	for _, name := range []string{"one", "two", "three"} {
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
	}
	d := NewApplicationDirectory(HomeDir())
	for _, entry := range d.entries {
		require(t, projectconnection.ResolveSnapshot(context.Background(), &entry.ref, entry.data))
	}
	return d
}

func measureStartupPreparation(t *testing.T, d *ApplicationDirectory) {
	started := time.Now()
	var wg sync.WaitGroup
	for _, entry := range d.entries {
		wg.Add(1)
		go func(e *catalogEntry) {
			defer wg.Done()
			r := &ProjectRuntime{}
			defer r.close()
			now := time.Now()
			err := r.prepare(context.Background(), d.home, e, d.global)
			t.Logf("startup_timing project=%s elapsed=%s error=%v", e.view.ConfigID, time.Since(now), err)
			if err != nil {
				t.Error(err)
			}
		}(entry)
	}
	wg.Wait()
	t.Logf("startup_timing three_projects_elapsed=%s", time.Since(started))
}

func TestStartupRecoveryTiming(t *testing.T) {
	d := startupTimingDirectory(t)
	started := time.Now()
	_, err := d.prepareRunnerStops(context.Background(), &runnerclient.Client{})
	require(t, err)
	t.Logf("startup_timing recovery_elapsed=%s", time.Since(started))
	measureStartupPreparation(t, d)
	t.Logf("startup_timing recovery_and_preparation_elapsed=%s", time.Since(started))
}
