package steerline

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIRejectsBeadsExecutionSource(t *testing.T) {
	f := newFixture(t)
	exe := buildServiceLifecycleCLI(t)
	configPath := filepath.Join(f.c.RunDir, "config.json")
	data, err := json.Marshal(f.c)
	require(t, err)
	require(t, os.WriteFile(configPath, data, 0600))
	output, err := exec.Command(exe, "run", "-config", configPath).CombinedOutput()
	if err == nil || !strings.Contains(string(output), "task_authority=native") {
		t.Fatalf("CLI admitted the inactive Beads execution path: err=%v output=%s", err, output)
	}
	if _, err = os.Stat(filepath.Join(f.c.RunDir, f.c.RuntimeFile)); !os.IsNotExist(err) {
		t.Fatalf("rejected Beads execution wrote runtime state: %v", err)
	}
}
