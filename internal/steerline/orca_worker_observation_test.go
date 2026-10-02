package steerline

import (
	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
	"testing"
)

func TestNativeOrcaAgentSelectionUsesFrozenProfile(t *testing.T) {
	base := Config{Host: "orca", Agent: "codex", ExecutionPolicy: workspacePolicyFixture()}
	for _, agent := range []string{"codex", "opencode"} {
		codex := codexworker.ProfileConfig{Executable: "codex.exe"}
		orca := &orcaworker.ProfileConfig{Executable: "orca", Shell: "cmd.exe", WaitTimeout: "2m"}
		if agent == "codex" {
			codex.WindowsSandbox = "unelevated"
			orca.Shell = "powershell.exe"
		}
		profile := testExecutionProfile(t, "orca-"+agent, "Orca "+agent, "orca", agent, codex, orca)
		profile = base.bindProjectPolicy(profile)
		base.ExecutionProfiles = []ExecutionProfile{profile}
		if err := validateExecutionProfiles(base); err != nil {
			t.Fatal(err)
		}
		frozen := profile.Freeze()
		if !testVerifyFixedExecutionProfile(frozen) {
			t.Fatal("profile digest changed while freezing")
		}
		selected := base.ApplyFixedExecutionProfile(frozen)
		if selected.agentName() != agent || selected.needsCodexExecutable() != (agent == "codex") {
			t.Fatalf("Orca agent selection was tied to Codex: %+v", selected)
		}
	}
}
