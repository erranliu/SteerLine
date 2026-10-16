package app

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/localservice"
	contract "github.com/erranliu/SteerLine/internal/product/applicationcontract"
)

func switchRepo(t *testing.T, home, name string) string {
	t.Helper()
	repo := filepath.Join(home, name)
	if err := os.MkdirAll(repo, 0700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "init", "--quiet", repo)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	return repo
}
func switchConfig(t *testing.T, home, name, repo string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(home, "etc"), 0700); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(map[string]any{"repository": repo, "task_authority": "native", "workflow_protocol": "worker-flow-v2"})
	if err := os.WriteFile(filepath.Join(home, "etc", name+".json"), data, 0600); err != nil {
		t.Fatal(err)
	}
}
func switchCatalog(t *testing.T, d *ApplicationDirectory, ready func(contract.Catalog) bool) contract.Catalog {
	t.Helper()
	deadline := time.Now().Add(75 * time.Second)
	for time.Now().Before(deadline) {
		c, err := d.Catalog(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if ready(c) {
			return c
		}
		time.Sleep(20 * time.Millisecond)
	}
	c, _ := d.Catalog(context.Background())
	t.Fatal("catalog did not settle", c)
	return contract.Catalog{}
}
func TestUnifiedStartupFrozenCatalogConflictsBeforeDatabase(t *testing.T) {
	home := t.TempDir()
	repo := switchRepo(t, home, "repo")
	switchConfig(t, home, "one", repo)
	switchConfig(t, home, "two", repo)
	os.WriteFile(filepath.Join(home, "etc", "broken.json"), []byte("{"), 0600)
	d := NewApplicationDirectory(home)
	defer d.Close()
	before, err := d.Catalog(context.Background())
	if err != nil || len(before.Projects) != 3 {
		t.Fatal(before, err)
	}
	if _, err = os.Stat(filepath.Join(home, "state")); !os.IsNotExist(err) {
		t.Fatal("catalog initialized storage")
	}
	d.Start()
	c := switchCatalog(t, d, func(c contract.Catalog) bool { return c.Projects[1].Conflict != "" && c.Projects[2].Conflict != "" })
	for _, p := range c.Projects {
		if p.ConfigID == "broken.json" {
			if p.ProjectID != "" {
				t.Fatal("invented project ID")
			}
			continue
		}
		if _, err = d.Connect(context.Background(), p.ProjectID); err == nil || !strings.Contains(err.Error(), "conflict") {
			t.Fatal("conflicted project connected", err)
		}
	}
	os.Remove(filepath.Join(home, "etc", "two.json"))
	switchConfig(t, home, "three", switchRepo(t, home, "other"))
	c, err = d.Catalog(context.Background())
	if err != nil || len(c.Projects) != 3 || c.Projects[2].ConfigID != "two.json" {
		t.Fatal("configuration snapshot changed", c, err)
	}
	if _, err = os.Stat(filepath.Join(home, "state")); !os.IsNotExist(err) {
		t.Fatal("conflict created database")
	}
}
func TestUnifiedStartupOverlappingRunDirs(t *testing.T) {
	home := t.TempDir()
	for i, name := range []string{"one", "two"} {
		repo := switchRepo(t, home, name)
		switchConfig(t, home, name, repo)
		path := filepath.Join(home, "etc", name+".json")
		var input map[string]any
		raw, _ := os.ReadFile(path)
		json.Unmarshal(raw, &input)
		dir := filepath.Join(home, "state", "parent")
		if i == 1 {
			dir = filepath.Join(dir, "child")
		}
		input["run_dir"] = dir
		raw, _ = json.Marshal(input)
		os.WriteFile(path, raw, 0600)
	}
	d := NewApplicationDirectory(home)
	defer d.Close()
	d.Start()
	switchCatalog(t, d, func(c contract.Catalog) bool {
		return len(c.Projects) == 2 && c.Projects[0].Conflict != "" && c.Projects[1].Conflict != ""
	})
	if _, err := os.Stat(filepath.Join(home, "state")); !os.IsNotExist(err) {
		t.Fatal("overlap opened project storage")
	}
}
func TestUnifiedStartupZeroProjectsLifecycle(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STEERLINE_HOME", home)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- ServeApplication(ctx) }()
	var c *localservice.Client
	deadline := time.Now().Add(75 * time.Second)
	for time.Now().Before(deadline) {
		var err error
		c, err = localservice.Open(ctx, home, filepath.Join(home, "application-api"), contract.Protocol, "/v2/application/identity")
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if c == nil {
		t.Fatal("application not ready")
	}
	defer c.Close()
	var catalog contract.Catalog
	if err := c.Request(ctx, "GET", "/v2/application/projects", nil, &catalog); err != nil || len(catalog.Projects) != 0 {
		t.Fatal(catalog, err)
	}
	if _, err := localservice.Read(t.TempDir(), filepath.Join(home, "application-api"), contract.Protocol); err == nil {
		t.Fatal("cross home discovery accepted")
	}
	if err := c.Request(ctx, "POST", "/v2/application/shutdown", nil, nil); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("normal stop failed")
	}
	if _, err := os.Stat(filepath.Join(home, "state")); !os.IsNotExist(err) {
		t.Fatal("application created project state")
	}
}
func TestUnifiedStartupCloseBeforeStartAndPendingResults(t *testing.T) {
	home := t.TempDir()
	repo := switchRepo(t, home, "repo")
	switchConfig(t, home, "one", repo)
	d := NewApplicationDirectory(home)
	require(t, d.Close())
	d.Start()
	require(t, d.Close())
	if d.started || d.entries[0].runtime != nil {
		t.Fatal("Close implicitly started a runtime")
	}
	if _, err := os.Stat(filepath.Join(home, "state")); !os.IsNotExist(err) {
		t.Fatal("Close created state", err)
	}
	completed := d.ShutdownResults()[0]
	if !completed.LocalClosed || !completed.ResourcesSettled || !completed.LocksReleased {
		t.Fatal("never-started project acquired obligations", completed)
	}
	// A scheduled owner's absent result must never use the no-resource default.
	d.mu.Lock()
	d.entries[0].resultReady = false
	d.entries[0].result = contract.ProjectShutdown{ConfigID: "one.json"}
	d.mu.Unlock()
	pending := d.ShutdownResults()[0]
	if pending.LocalClosed || pending.ResourcesSettled || pending.LocksReleased || pending.Diagnostic == "" {
		t.Fatal("pending result fabricated clean shutdown", pending)
	}
}

func TestUnifiedStartupLateReadyCannotReopenStoppedDirectory(t *testing.T) {
	home := t.TempDir()
	switchConfig(t, home, "one", switchRepo(t, home, "repo"))
	d := NewApplicationDirectory(home)
	defer d.Close()
	d.BeginStop()
	port := &runtimeProjectPort{instance: "late"}
	d.publish(d.entries[0], port)
	if !port.stopping.Load() || d.entries[0].view.Online || d.entries[0].view.RuntimeState != "stopping" {
		t.Fatal("late ready reopened project", d.entries[0].view)
	}
	if _, err := port.Read(context.Background(), "/identity", nil); err == nil {
		t.Fatal("late ready allowed borrowing")
	}
}
