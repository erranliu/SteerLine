package app

import (
	"strings"
	"testing"

	codexworker "github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	orcaworker "github.com/erranliu/SteerLine/internal/execution/adapters/orca"
)

func TestPS2FixedWindowsProfilesFreezeAndRestore(t *testing.T) {
	for _, host := range []string{"local", "orca"} {
		t.Run(host, func(t *testing.T) {
			config := Config{PublicationPolicy: executionPublicationFixture(), PR: hostPublicationProviderFixture(), Host: host, Agent: "codex", Repository: "repo", Branch: "main"}
			var terminal *orcaworker.ProfileConfig
			if host == "orca" {
				terminal = &orcaworker.ProfileConfig{Executable: "orca", Shell: "powershell.exe", WaitTimeout: "1m"}
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
			if codexProfileSettings(restored.executionConfig()).Model != "original" {
				t.Fatal("mutable model changed the frozen execution")
			}
			fixed.ImplementationData[0].Payload[0] = '['
			if testVerifyFixedExecutionProfile(fixed) {
				t.Fatal("damaged profile accepted")
			}
		})
	}
}
