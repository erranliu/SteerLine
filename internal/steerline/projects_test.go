package steerline

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func minimalConfig(repo string) Config {
	return Config{
		PublicationPolicy: executionPublicationFixture(), PR: hostPublicationProviderFixture(), ExecutionPolicy: workspacePolicyFixture(), TaskAuthority: "native",
		WorkflowProtocol: WorkerFlowVersion,
		Repository:       repo,
		Git:              Command{Executable: "git"},
		Remote:           "origin",
		Branch:           "main",
		ExecutableTypes:  []string{"task"},
		TaskPoll:         "10s",
		PRPoll:           "10s",
		Heartbeat:        "5s",
		CommandTimeout:   "30s",
		ShutdownGrace:    "10s",
	}
}

func projectRepo(t *testing.T) string {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "repo")
	for _, args := range [][]string{{"init", "--quiet", "-b", "main", repo}, {"-C", repo, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "--quiet", "--allow-empty", "-m", "fixture"}} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	return repo
}

func writeProjectConfig(t *testing.T, name string, c Config) {
	t.Helper()
	b := projectConfigJSON(t, c)
	if e := os.MkdirAll(SteerlineEtcDir(), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(SteerlineEtcDir(), name+".json"), b, 0600); e != nil {
		t.Fatal(e)
	}
}

func TestListAndResolveProjects(t *testing.T) {
	t.Setenv("STEERLINE_HOME", t.TempDir())
	localGlobalExecutionFixture(t)
	writeProjectConfig(t, "zeta", minimalConfig(projectRepo(t)))
	writeProjectConfig(t, "alpha", minimalConfig(projectRepo(t)))

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
	must := []string{"console.bat", "service-window.bat", "README.txt"}
	got := map[string]bool{}
	for _, p := range written {
		got[filepath.Base(p)] = true
	}
	for _, name := range must {
		if !got[name] {
			t.Fatalf("launcher %s not written (wrote %v)", name, written)
		}
	}
	console, e := os.ReadFile(filepath.Join(home, "console.bat"))
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(string(console), "console -config") || !strings.Contains(string(console), "\r\n") {
		t.Fatalf("console.bat missing current console entry or CRLF:\n%s", console)
	}
	window, e := os.ReadFile(filepath.Join(home, "service-window.bat"))
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(string(window), "-dispatch manual -console summary -service-window") || !strings.Contains(string(window), "etc\\SteerLine.json") || strings.Contains(string(window), "merge-h-308-code.json") || !strings.Contains(string(window), "\r\n") {
		t.Fatalf("service-window.bat missing required window/manual/summary behavior or CRLF:\n%s", window)
	}
	if len(written) != len(must) {
		t.Fatalf("unexpected launchers written: %v", written)
	}
}
