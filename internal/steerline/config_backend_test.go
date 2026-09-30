package steerline

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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
		profile := ExecutionProfile{ID: "selected", Name: "Selected", Host: host, Agent: "codex", Sandbox: "workspace-write", Codex: Command{Executable: "codex"}, Orca: OrcaConfig{Executable: "orca", Shell: "cmd.exe", WaitTimeout: "15m"}}
		writeGlobalExecutionFixture(t, []ExecutionProfile{profile}, "selected")
		data, err := json.Marshal(map[string]string{"repository": configTestRepository(t), "branch": "main", "task_authority": "native", "workflow_protocol": WorkerFlowVersion})
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
	if orca.ReasoningEffort != "" || orca.WorkflowProtocol != WorkerFlowVersion {
		t.Fatalf("Orca worker flow acquired old project steps or a local model default: %+v", orca)
	}
	local := write("local")
	if local.ReasoningEffort != "high" || !local.needsCodexExecutable() {
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
	data, err := json.Marshal(map[string]string{"repository": t.TempDir(), "run_dir": t.TempDir(), "branch": "main"})
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
