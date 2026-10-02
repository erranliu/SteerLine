package steerlineapp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestFiveGlobalLunaProfilesLoadAndFreeze(t *testing.T) {
	t.Setenv("STEERLINE_HOME", t.TempDir())
	data, err := os.ReadFile(filepath.Join("..", "..", "examples", "execution.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(GlobalExecutionConfigPath(), data, 0600); err != nil {
		t.Fatal(err)
	}
	projectPath := filepath.Join(t.TempDir(), "project.json")
	project, err := json.Marshal(map[string]any{"publication_policy": map[string]string{"publisher": PublisherExecutionProfile}, "pr": hostPublicationProviderFixture(), "execution_policy": workspacePolicyFixture(), "repository": configTestRepository(t), "branch": "main", "task_authority": "native", "workflow_protocol": WorkerFlowVersion})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(projectPath, project, 0600); err != nil {
		t.Fatal(err)
	}
	config, err := LoadConfig(projectPath)
	if err != nil {
		t.Fatal(err)
	}
	profiles := config.AvailableExecutionProfiles()
	if len(profiles) != 5 || config.DefaultProfileID() != "local-luna-medium" {
		t.Fatalf("global catalog = %+v; default = %q", profiles, config.DefaultProfileID())
	}
	want := []struct{ id, host, effort string }{
		{"local-luna-medium", "local", "medium"},
		{"local-luna-xhigh", "local", "xhigh"},
		{"orca-local-luna-xhigh", "orca", "xhigh"},
		{"orca-local-luna-medium", "orca", "medium"},
		{"orca-local-luna-high", "orca", "high"},
	}
	for i, expected := range want {
		profile := profiles[i]
		codex := codexProfileFromInputs(profile.ImplementationConfigs)
		if profile.ID != expected.id || profile.Host != expected.host || profile.Agent != "codex" || codex.Model != "gpt-6-luna" || codex.ReasoningEffort != expected.effort || !slices.Contains(profile.ImplementationDisplay, "权限：workspace-write") || !testVerifyFixedExecutionProfile(profile.Freeze()) {
			t.Fatalf("profile %d = %+v", i, profile)
		}
		if profile.Host == "orca" {
			selected, _, err := config.ResolveExecutionProfile(profile.ID, profile.Digest())
			if err != nil {
				t.Fatal(err)
			}
			command, err := nativeOrcaFixedAgentCommand(selected)
			if err != nil || !strings.Contains(command, "'gpt-6-luna'") || !strings.Contains(command, "'model_reasoning_effort="+expected.effort+"'") {
				t.Fatalf("fixed Orca launch %q: %v", command, err)
			}
		}
	}
}
