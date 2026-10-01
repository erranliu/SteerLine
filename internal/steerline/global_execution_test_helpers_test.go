package steerline

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
)

func writeGlobalExecutionFixture(t *testing.T, profiles []ExecutionProfile, defaultID string) {
	t.Helper()
	data, err := json.Marshal(map[string]any{"version": 2, "execution_profiles": profiles, "default_execution_profile": defaultID})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(GlobalExecutionConfigPath(), data, 0600); err != nil {
		t.Fatal(err)
	}
}

func localGlobalExecutionFixture(t *testing.T) {
	t.Helper()
	writeGlobalExecutionFixture(t, []ExecutionProfile{testExecutionProfile(t, "local", "Local Codex", "local", "codex", codexworker.ProfileConfig{Executable: "codex"}, nil)}, "local")
}

func projectConfigJSON(t *testing.T, c Config) []byte {
	t.Helper()
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"host", "agent", "model", "reasoning_effort", "sandbox", "profile", "codex", "orca", "execution_profiles", "default_execution_profile"} {
		delete(fields, field)
	}
	data, err = json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
