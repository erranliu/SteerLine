package steerline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestConfigKeepsOrcaCodexNativeByDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	write := func(host string) Config {
		t.Helper()
		data, err := json.Marshal(map[string]string{"repository": t.TempDir(), "run_dir": t.TempDir(), "branch": "main", "host": host, "agent": "codex"})
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
	if orca.ReasoningEffort != "" || orca.needsCodexExecutable() {
		t.Fatalf("Orca's native Codex path was silently converted to a local launch: %+v", orca)
	}
	local := write("local")
	if local.ReasoningEffort != "high" || !local.needsCodexExecutable() {
		t.Fatalf("local Codex defaults changed: %+v", local)
	}
}

func TestConfigRejectsRemovedOrcaFallback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	data, err := json.Marshal(map[string]any{
		"repository": t.TempDir(), "run_dir": t.TempDir(), "branch": "main",
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
