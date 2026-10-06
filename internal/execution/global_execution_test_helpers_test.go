package execution

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

// Frozen publication fixtures describe the current supported route before a unit
// test mutates records. They do not reconstruct missing production snapshots.
func publicationSnapshotFixture(t *testing.T, c Config) executionmodel.FixedExecutionProfile {
	t.Helper()
	c = withNeutralProfile(c, neutralProfileSettings(c))
	dependencies, _ := neutralFixtureRuntime(c)
	require(t, validateWorkflowConfig(c, dependencies.Directory))
	require(t, c.BindPublicationTarget(context.Background(), dependencies.ProcessHost))
	p := testProjectExecutionProfile(c).Freeze(dependencies.Directory, c.Identity())
	if !VerifyFixedExecutionProfile(p, dependencies.Directory) {
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
	fields["task_authority"], _ = json.Marshal("native")
	fields["claimant"], _ = json.Marshal("operator")
	fields["executable_types"], _ = json.Marshal([]string{"task", "bug", "feature", "chore"})
	fields["task_poll"], _ = json.Marshal("10s")
	fields["pr_poll"], _ = json.Marshal("30s")
	fields["heartbeat"], _ = json.Marshal("5s")
	fields["shutdown_grace"], _ = json.Marshal("10s")
	data, err = json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
