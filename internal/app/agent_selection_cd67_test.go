package app

import (
	"testing"

	"github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	"github.com/erranliu/SteerLine/internal/execution/adapters/orca"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func TestNativeOrcaAgentSelectionUsesFrozenProfile(t *testing.T) {
	base := bindUnitDirectory(t, Config{Host: "orca_supervised", Agent: "codex", PublicationPolicy: executionbackend.PublicationPolicy{Publisher: executionbackend.PublisherWorker}})
	for _, agent := range []string{"codex", "opencode"} {
		codex := codexworker.ProfileConfig{Executable: "codex.exe"}
		orca := &orcaworker.ProfileConfig{Executable: "orca", Shell: "cmd.exe", WaitTimeout: "2m"}
		if agent == "codex" {
			orca.Shell = "powershell.exe"
		}
		profile := testExecutionProfile(t, "orca-"+agent, "Orca "+agent, "orca_supervised", agent, codex, orca)
		profile = boundProfile(base, profile)
		base.ExecutionProfiles = []ExecutionProfile{profile}
		err := base.ValidateProfiles()
		if err != nil {
			t.Fatal(err)
		}
		frozen := profile.Freeze(base.ExecutionDirectory, base.Identity())
		if base.CheckFixedProfile(frozen) != nil {
			t.Fatal("profile digest changed while freezing")
		}
		base.Agent = "opencode" // Mutable defaults cannot redirect the frozen native route.
		selected := base.ApplyFixedExecutionProfile(frozen)
		if selected.Agent != agent || (selected.Agent == "codex") != (agent == "codex") {
			t.Fatalf("Orca agent selection was tied to Codex: %+v", selected)
		}
	}
}
