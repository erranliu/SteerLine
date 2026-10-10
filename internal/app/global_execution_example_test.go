package app

import (
	"encoding/json"
	remote "github.com/erranliu/SteerLine/internal/execution/adapters/orcaremote"
	"os"
	"path/filepath"
	"slices"
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
	project, err := json.Marshal(map[string]any{"publication_policy": map[string]string{"publisher": "worker"}, "pr": hostPublicationProviderFixture(), "repository": configTestRepository(t), "branch": "main", "task_authority": "native", "workflow_protocol": WorkerFlowVersion})
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
		{"orca-local-luna-xhigh", "orca_supervised", "xhigh"},
		{"orca-local-luna-medium", "orca_supervised", "medium"},
		{"orca-local-luna-high", "orca_supervised", "high"},
	}
	for i, expected := range want {
		profile := profiles[i]
		model, effort := "", ""
		if profile.Host == "orca_supervised" {
			native, decodeErr := remote.DecodeProfileConfig(profile.ImplementationConfigs[0].Payload)
			require(t, decodeErr)
			model, effort = native.Model, native.ReasoningEffort
			if len(profile.ImplementationConfigs) != 1 || profile.ImplementationConfigs[0].ImplementationKey != remote.ProfileCodecKey {
				t.Fatal("Orca example includes agent-specific configuration")
			}
		} else {
			local := codexProfileFromInputs(profile.ImplementationConfigs)
			model, effort = local.Model, local.ReasoningEffort
			if !slices.Contains(profile.ImplementationDisplay, "权限：workspace-write") {
				t.Fatal("local policy display was lost")
			}
		}
		if profile.ID != expected.id || profile.Host != expected.host || profile.Agent != "codex" || model != "gpt-6-luna" || effort != expected.effort || !testVerifyFixedExecutionProfile(profile.Freeze()) {
			t.Fatalf("profile %d = %+v", i, profile)
		}

	}
}
