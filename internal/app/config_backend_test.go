package app

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	"github.com/erranliu/SteerLine/internal/execution/adapters/orca"
)

func configTestRepository(t *testing.T) string {
	t.Helper()
	repository := t.TempDir()
	if output, err := exec.Command("git", "init", "--quiet", "-b", "main", repository).CombinedOutput(); err != nil {
		t.Fatalf("initialize config fixture repository: %v: %s", err, output)
	}
	return repository
}

func TestConfigKeepsOrcaCodexNativeByDefault(t *testing.T) {
	t.Setenv("STEERLINE_HOME", t.TempDir())
	path := filepath.Join(t.TempDir(), "config.json")
	write := func(host string) Config {
		t.Helper()
		codex := codexworker.ProfileConfig{Executable: "codex"}
		orca := &orcaworker.ProfileConfig{Executable: "orca", Shell: "cmd.exe", WaitTimeout: "15m"}
		if host == "orca" {
			orca.Shell = "powershell.exe"
		}
		profile := testExecutionProfile(t, "selected", "Selected", host, "codex", codex, orca)
		writeGlobalExecutionFixture(t, []ExecutionProfile{profile}, "selected")
		data, err := json.Marshal(map[string]any{"publication_policy": executionPublicationFixture(), "pr": hostPublicationProviderFixture(), "repository": configTestRepository(t), "branch": "main", "task_authority": "native", "workflow_protocol": WorkerFlowVersion})
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		cfg, err := LoadConfig(path)
		if err != nil {
			t.Fatal(err)
		}
		return cfg
	}
	orca := write("orca")
	if codexProfileSettings(orca.executionConfig()).ReasoningEffort != "" || orca.WorkflowProtocol != WorkerFlowVersion {
		t.Fatalf("Orca worker flow acquired old project steps or a local model default: %+v", orca)
	}
	local := write("local")
	if codexProfileSettings(local.executionConfig()).ReasoningEffort != "high" || local.Host != "local" {
		t.Fatalf("local Codex defaults changed: %+v", local)
	}
}

func TestConfigRejectsRemovedOrcaFallback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	data, err := json.Marshal(map[string]any{
		"repository": configTestRepository(t), "branch": "main", "task_authority": "native",
		"host": "orca", "orca": map[string]any{"fallback": "local"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("removed orca.fallback field was accepted")
	}
}

func TestConfigRequiresAnExplicitTaskAuthority(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	data, err := json.Marshal(map[string]any{"publication_policy": executionPublicationFixture(), "pr": hostPublicationProviderFixture(), "repository": t.TempDir(), "run_dir": t.TempDir(), "branch": "main"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(path); err == nil || !strings.Contains(err.Error(), "task_authority must be explicitly set to native") {
		t.Fatalf("missing task authority silently selected a task source: %v", err)
	}
}

func TestConfigRejectsRetiredBeadsExecutionFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	data, err := json.Marshal(map[string]any{
		"repository": configTestRepository(t), "remote": "origin", "branch": "main", "task_authority": "native",
		"bd":         map[string]any{"executable": "bd"},
		"completion": map[string]any{"command": map[string]any{"executable": "finish-task"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = LoadConfig(path); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("retired Beads config fields were silently ignored: %v", err)
	}
}

func TestOrcaNativeProfileKeepsFrozenAuthority(t *testing.T) {
	t.Setenv("STEERLINE_HOME", t.TempDir())
	profile := testExecutionProfile(t, "orca", "Orca", "orca", "codex", codexworker.ProfileConfig{Executable: "codex"}, &orcaworker.ProfileConfig{Executable: "orca", Shell: "powershell.exe", WaitTimeout: "1m"})
	payload, err := orcaworker.EncodeProfileConfig(orcaworker.ProfileConfig{Executable: "orca", Shell: "cmd.exe", WaitTimeout: "1m", Model: "gpt-6-luna", ReasoningEffort: "medium"})
	require(t, err)
	profile.ImplementationConfigs[0].Payload = payload
	writeGlobalExecutionFixture(t, []ExecutionProfile{profile}, "orca")
	path := filepath.Join(t.TempDir(), "project.json")
	data, err := json.Marshal(map[string]any{"repository": configTestRepository(t), "branch": "main", "task_authority": "native", "workflow_protocol": WorkerFlowVersion, "publication_policy": map[string]string{"publisher": "worker"}, "pr": hostPublicationProviderFixture()})
	require(t, err)
	require(t, os.WriteFile(path, data, 0600))
	config, err := LoadConfig(path)
	require(t, err)
	require(t, config.ResolveIdentity(context.Background()))
	selected, accepted, err := config.ResolveExecutionProfile("orca", "")
	require(t, err)
	if err := validateTerminalConfig(context.Background(), selected, config.ExecutionDirectory); err != nil || terminalReadinessRequired(selected, config.ExecutionDirectory) {
		t.Fatalf("fixed native launch lost admission or readiness: %v", err)
	}
	fixed := accepted.Freeze(config.ExecutionDirectory, config.Identity())
	if accepted.Digest(config.ExecutionDirectory) != fixed.Digest || config.CheckFixedProfile(fixed) != nil {
		t.Fatal("fixed native launch changed while selecting and freezing the profile")
	}
}
