package steerline

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
)

// Frozen publication fixtures describe the current supported route before a unit
// test mutates records. They do not reconstruct missing production snapshots.
func publicationSnapshotFixture(t *testing.T, c Config) FixedExecutionProfile {
	t.Helper()
	if c.PR.Kind == "gitea" {
		c.Host, c.Agent = "orca", "codex"
		c.PublicationPolicy.Publisher = "worker"
		codex := codexProfileSettings(c)
		codex.WindowsSandbox = "unelevated"
		orca := orcaProfileSettings(c)
		orca.Shell, orca.WaitTimeout = "powershell.exe", "1m"
		setTestImplementationProfiles(t, &c, codex, &orca)
	}
	require(t, validateWorkflowConfig(&c, nil))
	require(t, c.bindPublicationTarget(context.Background()))
	p := c.projectExecutionProfile().Freeze()
	if !verifyFixedExecutionProfile(p) {
		t.Fatal("invalid publication snapshot fixture")
	}
	return p
}

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
