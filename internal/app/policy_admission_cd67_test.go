package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/execution/adapters/codex"
)

func TestProjectPolicyRejectsMissingOrOldConfiguration(t *testing.T) {
	t.Setenv("STEERLINE_HOME", t.TempDir())
	writeGlobalExecutionFixture(t, []ExecutionProfile{testExecutionProfile(t, "local", "Local", "local", "codex", codexworker.ProfileConfig{Executable: "codex"}, nil)}, "local")
	body := map[string]any{"publication_policy": executionPublicationFixture(), "pr": hostPublicationProviderFixture(), "repository": configTestRepository(t), "branch": "main", "task_authority": "native", "workflow_protocol": WorkerFlowVersion}
	path := filepath.Join(t.TempDir(), "project.json")
	write := func() {
		t.Helper()
		data, err := json.Marshal(body)
		require(t, err)
		require(t, os.WriteFile(path, data, 0600))
	}
	write()
	if _, err := LoadConfig(path); err != nil {
		t.Fatalf("fixed default configuration rejected: %v", err)
	}
	for _, field := range []string{"execution_policy", "execution_requirements", "sandbox", "windows_sandbox", "approval_policy", "approvals_reviewer"} {
		body[field] = nil
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
	profile := testExecutionProfile(t, "local", "Local", "local", "codex", codexworker.ProfileConfig{Executable: "codex"}, nil)
	profile.ImplementationConfigs[0].Payload = []byte(`{"executable":"codex","args":[]}`)
	writeGlobalExecutionFixture(t, []ExecutionProfile{profile}, "local")
	path := filepath.Join(t.TempDir(), "project.json")
	raw, err := json.Marshal(map[string]any{"publication_policy": executionPublicationFixture(), "pr": hostPublicationProviderFixture(), "repository": t.TempDir(), "branch": "main", "task_authority": "native", "workflow_protocol": WorkerFlowVersion, "git": map[string]string{"executable": "must-never-be-called"}})
	require(t, err)
	require(t, os.WriteFile(path, raw, 0600))
	_, err = LoadConfig(path)
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unsafe args reached repository identity probe: %v", err)
	}
}
