package steerline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGlobalExecutionProfilesSharedAcrossProjects(t *testing.T) {
	home := t.TempDir()
	t.Setenv("STEERLINE_HOME", home)
	profiles := []ExecutionProfile{
		{ID: "local", Name: "本地", Host: "local", Agent: "codex", Sandbox: "workspace-write", Codex: Command{Executable: "codex"}},
		{ID: "strong", Name: "高强度", Host: "local", Agent: "codex", Model: "chosen-model", ReasoningEffort: "xhigh", Sandbox: "danger-full-access", Codex: Command{Executable: "codex"}},
	}
	writeGlobal := func() {
		t.Helper()
		data, err := json.Marshal(map[string]any{"default_execution_profile": "local", "execution_profiles": profiles})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(GlobalExecutionConfigPath(), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeGlobal()
	loadProject := func() (Config, string) {
		t.Helper()
		path := filepath.Join(t.TempDir(), "project.json")
		data, err := json.Marshal(map[string]any{"execution_policy": workspacePolicyFixture(), "repository": configTestRepository(t), "branch": "main", "task_authority": "native", "workflow_protocol": WorkerFlowVersion})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		config, err := LoadConfig(path)
		if err != nil {
			t.Fatal(err)
		}
		return config, path
	}
	first, _ := loadProject()
	second, secondPath := loadProject()
	if len(first.AvailableExecutionProfiles()) != 2 || len(second.AvailableExecutionProfiles()) != 2 || first.DefaultProfileID() != "local" || second.DefaultProfileID() != "local" || first.AvailableExecutionProfiles()[0].ID == ProjectExecutionProfileID {
		t.Fatalf("global profiles were not shared: first=%+v second=%+v", first.AvailableExecutionProfiles(), second.AvailableExecutionProfiles())
	}
	selected, profile, err := second.ResolveExecutionProfile("strong", second.bindProjectPolicy(profiles[1]).Digest())
	if err != nil || selected.Model != "chosen-model" || profile.ID != "strong" {
		t.Fatalf("global profile selection failed: %+v / %v", selected, err)
	}
	oldDigest := second.LoadedConfigDigest()
	profiles[1].Model = "later-edit"
	writeGlobal()
	updated, err := LoadConfig(secondPath)
	if err != nil {
		t.Fatal(err)
	}
	if updated.LoadedConfigDigest() == oldDigest {
		t.Fatal("global config edit did not change combined config digest")
	}
	if _, _, err := updated.ResolveExecutionProfile("strong", profile.Digest()); err == nil || !strings.Contains(err.Error(), "stale_profile") {
		t.Fatalf("edited global profile accepted stale digest: %v", err)
	}
}

func TestProjectExecutionSettingsAreRejected(t *testing.T) {
	t.Setenv("STEERLINE_HOME", t.TempDir())
	path := filepath.Join(t.TempDir(), "project.json")
	data, err := json.Marshal(map[string]any{"execution_policy": workspacePolicyFixture(), "repository": configTestRepository(t), "branch": "main", "task_authority": "native", "workflow_protocol": WorkerFlowVersion, "host": "local"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(path); err == nil || !strings.Contains(err.Error(), "global execution config") {
		t.Fatalf("project execution setting was accepted: %v", err)
	}
}
