package steerline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func minimalConfig(repo, runDir string) Config {
	return Config{
		Repository:      repo,
		Remote:          "origin",
		Branch:          "main",
		RunDir:          runDir,
		RuntimeFile:     "runtime.json",
		ExecutableTypes: []string{"task"},
		TaskPoll:        "10s",
		PRPoll:          "10s",
		Heartbeat:       "5s",
		CommandTimeout:  "30s",
		ShutdownGrace:   "10s",
		Sandbox:         "workspace-write",
	}
}

func writeProjectConfig(t *testing.T, name string, c Config) {
	t.Helper()
	b, e := json.Marshal(c)
	if e != nil {
		t.Fatal(e)
	}
	if e := os.MkdirAll(SteerlineEtcDir(), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(SteerlineEtcDir(), name+".json"), b, 0600); e != nil {
		t.Fatal(e)
	}
}

func TestListAndResolveProjects(t *testing.T) {
	t.Setenv("STEERLINE_HOME", t.TempDir())
	writeProjectConfig(t, "zeta", minimalConfig(filepath.Join(t.TempDir(), "z"), filepath.Join(t.TempDir(), "zstate")))
	writeProjectConfig(t, "alpha", minimalConfig(filepath.Join(t.TempDir(), "a"), filepath.Join(t.TempDir(), "astate")))

	projects, e := ListProjects()
	if e != nil {
		t.Fatal(e)
	}
	if len(projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(projects))
	}
	if projects[0].Name != "alpha" || projects[1].Name != "zeta" {
		t.Fatalf("projects not sorted by name: %+v", projects)
	}
	if projects[0].Err != nil {
		t.Fatalf("config should load: %v", projects[0].Err)
	}

	byNumber, e := ResolveProject(projects, "2")
	if e != nil || byNumber.Name != "zeta" {
		t.Fatalf("resolve by number: %+v %v", byNumber, e)
	}
	byName, e := ResolveProject(projects, "ALPHA")
	if e != nil || byName.Name != "alpha" {
		t.Fatalf("resolve by name: %+v %v", byName, e)
	}
	if _, e := ResolveProject(projects, "nope"); e == nil {
		t.Fatal("expected error for unknown project")
	}
}

func TestInstallLaunchersWritesCRLF(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STEERLINE_HOME", home)
	written, e := InstallLaunchers()
	if e != nil {
		t.Fatal(e)
	}
	must := []string{"projects.bat", "viewer.bat", "run.bat", "stop.bat", "home.bat", "README.txt"}
	got := map[string]bool{}
	for _, p := range written {
		got[filepath.Base(p)] = true
	}
	for _, name := range must {
		if !got[name] {
			t.Fatalf("launcher %s not written (wrote %v)", name, written)
		}
	}
	b, e := os.ReadFile(filepath.Join(home, "projects.bat"))
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(string(b), "bin\\steerline.exe") || !strings.Contains(string(b), "\r\n") {
		t.Fatalf("projects.bat not CRLF/expected content:\n%s", b)
	}
}
