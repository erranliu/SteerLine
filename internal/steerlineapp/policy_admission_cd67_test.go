package steerlineapp

import (
	"encoding/json"
	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectPolicyRejectsMissingOrOldConfiguration(t *testing.T) {
	t.Setenv("STEERLINE_HOME", t.TempDir())
	writeGlobalExecutionFixture(t, []ExecutionProfile{testExecutionProfile(t, "local", "Local", "local", "codex", codexworker.ProfileConfig{Executable: "codex"}, nil)}, "local")
	body := map[string]any{"publication_policy": executionPublicationFixture(), "repository": configTestRepository(t), "branch": "main", "task_authority": "native", "workflow_protocol": WorkerFlowVersion}
	path := filepath.Join(t.TempDir(), "project.json")
	write := func() {
		t.Helper()
		data, err := json.Marshal(body)
		require(t, err)
		require(t, os.WriteFile(path, data, 0600))
	}
	write()
	if _, err := LoadConfig(path); err == nil || !strings.Contains(err.Error(), "execution_policy") {
		t.Fatalf("missing authority accepted: %v", err)
	}
	body["execution_policy"] = workspacePolicyFixture()
	for _, field := range []string{"sandbox", "windows_sandbox"} {
		body[field] = "workspace-write"
		write()
		if _, err := LoadConfig(path); err == nil {
			t.Fatalf("old project authorization field accepted: %s", field)
		}
		delete(body, field)
	}
	write()
	if _, err := LoadConfig(path); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"sandbox", "windows_sandbox"} {
		raw, err := json.Marshal(map[string]any{"execution_profiles": []map[string]any{{"id": "local", "name": "Local", "host": "local", "agent": "codex", field: "workspace-write", "codex": map[string]string{"executable": "codex"}}}})
		require(t, err)
		require(t, os.WriteFile(GlobalExecutionConfigPath(), raw, 0600))
		if _, err := LoadConfig(path); err == nil {
			t.Fatalf("old global authorization field accepted: %s", field)
		}
	}
}

func TestProjectPolicyRejectsGlobalConflictsBeforeRepositoryProbe(t *testing.T) {
	t.Setenv("STEERLINE_HOME", t.TempDir())
	writeGlobalExecutionFixture(t, []ExecutionProfile{testExecutionProfile(t, "local", "Local", "local", "codex", codexworker.ProfileConfig{Executable: "codex", Args: []string{"--config", "sandbox_mode=danger-full-access"}}, nil)}, "local")
	path := filepath.Join(t.TempDir(), "project.json")
	raw, err := json.Marshal(map[string]any{"publication_policy": executionPublicationFixture(), "pr": hostPublicationProviderFixture(), "execution_policy": workspacePolicyFixture(), "repository": t.TempDir(), "branch": "main", "task_authority": "native", "workflow_protocol": WorkerFlowVersion, "git": map[string]string{"executable": "must-never-be-called"}})
	require(t, err)
	require(t, os.WriteFile(path, raw, 0600))
	_, err = LoadConfig(path)
	if err == nil || !strings.Contains(err.Error(), "unsupported managed launch") {
		t.Fatalf("unsafe args reached repository identity probe: %v", err)
	}
}
