package app

import (
	"context"
	"encoding/json"
	"github.com/erranliu/SteerLine/internal/localservice"
	contract "github.com/erranliu/SteerLine/internal/product/applicationcontract"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
	deadline := time.Now().Add(10 * time.Second)
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
	t.Fatal("catalog did not settle")
	return contract.Catalog{}
}
func TestWebSwitchCatalogReadOnlyAndConflicts(t *testing.T) {
	home := t.TempDir()
	d := NewApplicationDirectory(home)
	defer d.Close()
	ctx := context.Background()
	c, err := d.Catalog(ctx)
	if err != nil || len(c.Projects) != 0 {
		t.Fatalf("zero config: %+v %v", c, err)
	}
	repo := switchRepo(t, home, "repo")
	switchConfig(t, home, "one", repo)
	switchConfig(t, home, "two", repo)
	os.WriteFile(filepath.Join(home, "etc", "broken.json"), []byte("{"), 0600)
	c = switchCatalog(t, d, func(c contract.Catalog) bool {
		return len(c.Projects) == 3 && c.Projects[1].Conflict != "" && c.Projects[0].Diagnostic != ""
	})
	for _, p := range c.Projects {
		if p.ConfigID == "broken.json" {
			if p.ProjectID != "" {
				t.Fatal("invented identity for bad config")
			}
		} else {
			if p.Online || p.Conflict == "" {
				t.Fatal("ambiguous repository is writable")
			}
			if _, err = d.Connect(ctx, p.ProjectID); err == nil || !strings.Contains(err.Error(), "conflict") {
				t.Fatal("conflict route connected", err)
			}
		}
	}
	if _, err = os.Stat(filepath.Join(home, "state")); !os.IsNotExist(err) {
		t.Fatal("catalog initialized project state")
	}
	os.Remove(filepath.Join(home, "etc", "two.json"))
	c = switchCatalog(t, d, func(c contract.Catalog) bool { return len(c.Projects) == 2 && c.Projects[1].Conflict == "" })
	old := c.Projects[1].ProjectID
	repo2 := switchRepo(t, home, "repo2")
	switchConfig(t, home, "one", repo2)
	if _, err = d.Connect(ctx, old); err == nil {
		t.Fatal("old mapping survived config replacement")
	}
	switchCatalog(t, d, func(c contract.Catalog) bool { return c.Projects[1].ProjectID != "" && c.Projects[1].ProjectID != old })
}
func TestWebSwitchCatalogBoundedSlowConfig(t *testing.T) {
	home := t.TempDir()
	repo := switchRepo(t, home, "repo")
	switchConfig(t, home, "good", repo)
	// This child test executable provides a portable slow Git command.
	data, _ := json.Marshal(map[string]any{"repository": repo, "task_authority": "native", "workflow_protocol": "worker-flow-v2", "git": map[string]any{"executable": os.Args[0], "args": []string{"-test.run=^TestWebSwitchSlowGitChild$"}}})
	os.WriteFile(filepath.Join(home, "etc", "slow.json"), data, 0600)
	t.Setenv("STEERLINE_SLOW_GIT_CHILD", "1")
	d := NewApplicationDirectory(home)
	defer d.Close()
	start := time.Now()
	if _, err := d.Catalog(context.Background()); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Fatal("directory blocked on slow Git")
	}
	switchCatalog(t, d, func(c contract.Catalog) bool { return c.Projects[0].ProjectID != "" })
	if _, err := os.Stat(filepath.Join(home, "state")); !os.IsNotExist(err) {
		t.Fatal("offline catalog wrote state")
	}
}
func TestWebSwitchSlowGitChild(t *testing.T) {
	if os.Getenv("STEERLINE_SLOW_GIT_CHILD") == "1" {
		time.Sleep(10 * time.Second)
		os.Exit(0)
	}
}
func TestWebSwitchApplicationZeroProjectsLifecycle(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STEERLINE_HOME", home)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- ServeApplication(ctx) }()
	var c *localservice.Client
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var err error
		c, err = localservice.Open(ctx, home, filepath.Join(home, "application-api"), contract.Protocol, "/v1/application/identity")
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
	if err := c.Request(ctx, "GET", "/v1/application/projects", nil, &catalog); err != nil || len(catalog.Projects) != 0 {
		t.Fatal(catalog, err)
	}
	if _, err := localservice.Read(t.TempDir(), filepath.Join(home, "application-api"), contract.Protocol); err == nil {
		t.Fatal("cross home discovery accepted")
	}
	if err := c.Request(ctx, "POST", "/v1/application/shutdown", nil, nil); err != nil {
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
