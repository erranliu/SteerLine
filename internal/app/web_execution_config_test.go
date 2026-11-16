package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	cfg "github.com/erranliu/SteerLine/internal/executionconfig"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

func TestWebExecutionConfigLoadedSnapshot(t *testing.T) {
	home := t.TempDir()
	original := []byte(`{"version":3,"saved":"original"}`)
	path := filepath.Join(home, "execution.json")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	d := NewApplicationDirectory(home)
	first, err := d.Catalog(context.Background())
	if err != nil || first.LoadedExecutionDigest != cfg.Digest(original) {
		t.Fatal(first, err)
	}
	if err = os.WriteFile(path, []byte(`{"saved":"changed"}`), 0600); err != nil {
		t.Fatal(err)
	}
	next, err := d.Catalog(context.Background())
	if err != nil || next.LoadedExecutionDigest != first.LoadedExecutionDigest {
		t.Fatal("query reloaded config", next, err)
	}
	d2 := NewApplicationDirectory(home)
	fresh, err := d2.Catalog(context.Background())
	if err != nil || fresh.LoadedExecutionDigest == first.LoadedExecutionDigest {
		t.Fatal("new consumer did not read new snapshot", fresh, err)
	}
}

func TestWebExecutionConfigReloadsProfilesWithoutRestart(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	projectPath, path := filepath.Join(home, "project.json"), filepath.Join(home, "execution.json")
	project := []byte(`{"repository":"fixture"}`)
	require(t, os.WriteFile(projectPath, project, 0600))
	plain := RunnerProfile{ID: "plain", Name: "Plain", Execution: runnercontract.ExecutionInput{
		Adapter: "codex-app-server", Target: runnercontract.ExecutionTarget{Kind: "local"}, Settings: json.RawMessage(`{}`),
	}, Workflow: runnercontract.Workflow{ID: "development", Version: 2}}
	initial := RunnerExecutionConfig{Version: 3, DefaultProfile: plain.ID, Profiles: []RunnerProfile{plain}}
	write := func(config RunnerExecutionConfig) string {
		raw := mustJSON(config)
		require(t, os.WriteFile(path, raw, 0600))
		return cfg.Digest(raw)
	}
	write(initial)
	d := NewApplicationDirectory(home)
	defer d.Close()
	d.Start()
	<-d.done // No projects and no resident Runner are needed to adopt the catalog.
	c := Config{RunnerExecution: initial, runtimeProfiles: d.profiles}
	c.bindApplicationFiles(projectPath, path, loadedConfigDigest(project, initial.startupSettings()), true)
	original := c.withProfiles(ctx)

	added := plain
	added.ID, added.Name = "added", "New profile"
	added.Execution.Settings = json.RawMessage(`{"model":"new"}`)
	next := initial
	next.Profiles = []RunnerProfile{added}
	next.DefaultProfile = added.ID
	digest := write(next)
	catalog, err := d.Catalog(ctx)
	require(t, err)
	if catalog.LoadedExecutionDigest != digest || catalog.ExecutionRestartRequired || catalog.ExecutionReloadError != "" {
		t.Fatalf("updated profile catalog was not loaded: %+v", catalog)
	}
	current := c.withProfiles(ctx)
	if got := c.executionConfig(current); got.DefaultProfile != added.ID || len(got.Profiles) != 1 || got.Profiles[0].Digest() != added.Digest() {
		t.Fatalf("new admission does not see the saved profiles: %+v", got)
	}
	if got := c.executionConfig(original); got.DefaultProfile != plain.ID {
		t.Fatalf("original admission changed: %+v", got)
	}
	reserved := false
	reserve := func() error { reserved = true; return nil }
	if err := c.reserveCurrentProfiles(original, reserve); err == nil || reserved {
		t.Fatalf("superseded configuration reached the writer: %v", err)
	}
	require(t, c.reserveCurrentProfiles(current, reserve))
	if !reserved {
		t.Fatal("current configuration did not reach the writer")
	}

	invalid := next
	invalid.DefaultProfile = "missing"
	write(invalid)
	catalog, err = d.Catalog(ctx)
	require(t, err)
	if catalog.LoadedExecutionDigest != digest || catalog.ExecutionReloadError == "" || catalog.ExecutionRestartRequired {
		t.Fatalf("invalid profile publication was not reported: %+v", catalog)
	}

	limit := 7
	next.MaxGenerateConcurrency = &limit
	write(next)
	catalog, err = d.Catalog(ctx)
	require(t, err)
	if catalog.LoadedExecutionDigest != digest || !catalog.ExecutionRestartRequired {
		t.Fatalf("startup settings change was accepted live: %+v", catalog)
	}

	initialDigest := write(initial)
	require(t, os.WriteFile(filepath.Join(home, "manifest.json"), []byte(`{}`), 0600))
	frozen := NewApplicationDirectory(home)
	defer frozen.Close()
	frozen.Start()
	<-frozen.done
	write(next)
	catalog, err = frozen.Catalog(ctx)
	require(t, err)
	if catalog.LoadedExecutionDigest != initialDigest || catalog.ExecutionReloadError == "" {
		t.Fatalf("frozen package accepted a configuration change: %+v", catalog)
	}
}
