package app

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func TestLoadProjectsSupportsZeroPartialAndExplicitConfigs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STEERLINE_HOME", home)
	localGlobalExecutionFixture(t)
	projects, err := LoadConsoleProjects("")
	if err != nil || len(projects) != 0 {
		t.Fatalf("empty config directory: %+v %v", projects, err)
	}
	etc := filepath.Join(home, "etc")
	if err = os.MkdirAll(etc, 0700); err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(t.TempDir(), "repo")
	if err = os.MkdirAll(repo, 0700); err != nil {
		t.Fatal(err)
	}
	if out, initErr := exec.Command("git", "init", repo).CombinedOutput(); initErr != nil {
		t.Fatalf("git init: %v: %s", initErr, out)
	}
	validConfig := filepath.Join(etc, "alpha.json")
	writeConsoleTestConfig(t, validConfig, repo, filepath.Join(home, "state-alpha"))
	if err = os.WriteFile(filepath.Join(etc, "broken.json"), []byte("not-json"), 0600); err != nil {
		t.Fatal(err)
	}
	projects, err = LoadConsoleProjects("")
	if err != nil || len(projects) != 2 {
		t.Fatalf("partial config directory: %+v %v", projects, err)
	}
	if projects[0].Name != "alpha" || projects[0].Error != "" || projects[0].Identity == "" {
		t.Fatalf("valid project not usable: %+v", projects[0])
	}
	if projects[1].Name != "broken" || projects[1].Error == "" || !strings.HasSuffix(projects[1].Key, "\x00") {
		t.Fatalf("broken config row was not retained: %+v", projects[1])
	}
	external := filepath.Join(t.TempDir(), "outside.json")
	writeConsoleTestConfig(t, external, repo, filepath.Join(home, "state-explicit"))
	explicit, err := LoadConsoleProjects(external)
	if err != nil || len(explicit) != 1 || explicit[0].ConfigPath != external || explicit[0].Identity == "" {
		t.Fatalf("explicit config outside etc was not selected: %+v %v", explicit, err)
	}
}

func writeConsoleTestConfig(t *testing.T, path, repository, runDir string) {
	t.Helper()
	data, err := json.Marshal(map[string]any{"publication_policy": executionbackend.PublicationPolicy{Publisher: executionbackend.PublisherWorker}, "pr": map[string]any{"kind": "command", "base_url": "https://github.com", "repository": "Org/Repo", "query": map[string]any{"executable": "gh"}}, "execution_policy": map[string]any{"filesystem": map[string]any{"read_scope": "host", "write_scope": "workspace", "git_metadata_write": true}, "network": "allow", "escalation": "deny"}, "repository": repository, "branch": "main", "task_authority": "native", "workflow_protocol": WorkerFlowVersion})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}
