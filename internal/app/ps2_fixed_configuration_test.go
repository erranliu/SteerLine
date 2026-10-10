package app

import (
	"strings"
	"testing"

	codexworker "github.com/erranliu/SteerLine/internal/execution/adapters/codex"
)

func TestPS2FixedWindowsProfilesFreezeAndRestore(t *testing.T) {
	for _, host := range []string{"local", "orca"} {
		t.Run(host, func(t *testing.T) {
			config := Config{PublicationPolicy: executionPublicationFixture(), PR: hostPublicationProviderFixture(), Host: host, Agent: "codex", Repository: "repo", Branch: "main"}
			var terminal *terminalFixtureProfile
			if host == "orca" {
				terminal = &terminalFixtureProfile{Executable: "orca", Shell: "powershell.exe", WaitTimeout: "1m"}
			}
			setTestImplementationProfiles(t, &config, codexworker.ProfileConfig{Executable: "codex", Model: "original"}, terminal)
			fixed := unitProjectProfile(config).Freeze()
			if !testVerifyFixedExecutionProfile(fixed) {
				t.Fatal("valid frozen profile rejected")
			}
			settings := []string{"workspace-write", "unelevated", "on-request", "auto_review"}
			if host == "orca" {
				settings = []string{"Orca"}
			}
			for _, setting := range settings {
				if !strings.Contains(strings.Join(fixed.ImplementationDisplay, " "), setting) {
					t.Fatal("missing fixed native setting", setting)
				}
			}
			setTestImplementationProfiles(t, &config, codexworker.ProfileConfig{Executable: "codex", Model: "changed"}, terminal)
			restored := config.ApplyFixedExecutionProfile(fixed)
			if host == "local" && codexProfileSettings(restored.executionConfig()).Model != "original" {
				t.Fatal("mutable model changed the frozen execution")
			}
			if host == "orca" && restored.ApplyExecutionProfile(config.ExecutionProfiles[0]).Agent != fixed.Agent {
				t.Fatal("frozen opaque agent changed")
			}
			fixed.ImplementationData[0].Payload[0] = '['
			if testVerifyFixedExecutionProfile(fixed) {
				t.Fatal("damaged profile accepted")
			}
		})
	}
}
