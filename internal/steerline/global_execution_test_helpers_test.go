package steerline

import (
	"encoding/json"
	"os"
	"testing"
)

func writeGlobalExecutionFixture(t *testing.T, profiles []ExecutionProfile, defaultID string) {
	t.Helper()
	data, err := json.Marshal(map[string]any{"version": 2, "execution_profiles": profiles, "default_execution_profile": defaultID})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fixtureGlobalExecutionConfigPath(), data, 0600); err != nil {
		t.Fatal(err)
	}
}

func localGlobalExecutionFixture(t *testing.T) {
	t.Helper()
	writeGlobalExecutionFixture(t, []ExecutionProfile{neutralProfile(t, "local", "Local fixture", neutralSettings{Executable: os.Args[0]})}, "local")
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
	for _, field := range []string{"host", "agent", "model", "reasoning_effort", "sandbox", "profile", "execution_profiles", "default_execution_profile"} {
		delete(fields, field)
	}
	data, err = json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
