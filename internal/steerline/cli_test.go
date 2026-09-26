package steerline

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIRejectsLegacyTaskAuthorityBeforeStateInitialization(t *testing.T) {
	f := newFixture(t)
	exe := buildServiceLifecycleCLI(t)
	config := f.c
	config.TaskAuthority = "beads"
	configPath := filepath.Join(t.TempDir(), "config.json")
	data, err := json.Marshal(config)
	require(t, err)
	require(t, os.WriteFile(configPath, data, 0600))
	output, err := exec.Command(exe, "run", "-config", configPath).CombinedOutput()
	if err == nil || !strings.Contains(string(output), "task_authority must be explicitly set to native") {
		t.Fatalf("CLI admitted the inactive Beads execution path: err=%v output=%s", err, output)
	}
	if _, err = os.Stat(filepath.Join(f.c.RunDir, f.c.RuntimeFile)); !os.IsNotExist(err) {
		t.Fatalf("rejected legacy authority wrote runtime state: %v", err)
	}
	if _, err = os.Stat(filepath.Join(f.c.RunDir, "steerline.db")); !os.IsNotExist(err) {
		t.Fatalf("rejected legacy authority initialized a native database: %v", err)
	}
}
