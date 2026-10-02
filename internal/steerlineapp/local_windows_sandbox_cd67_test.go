package steerlineapp

import (
	"context"
	"runtime"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
)

func TestLocalWindowsSandboxProfileFreezesAndRestores(t *testing.T) {
	config := Config{ExecutionPolicy: workspacePolicyFixture(), PublicationPolicy: executionPublicationFixture(), PR: hostPublicationProviderFixture(), Host: "local", Agent: "codex", Repository: "repo", Branch: "main"}
	setTestCodexProfile(t, &config, codexworker.ProfileConfig{Executable: "codex", WindowsSandbox: "elevated"})
	profile := unitProjectProfile(config)
	fixed := profile.Freeze()
	if !testVerifyFixedExecutionProfile(fixed) || !strings.Contains(strings.Join(fixed.ImplementationDisplay, " "), "Windows 权限：elevated") {
		t.Fatalf("bad frozen profile: %+v", fixed)
	}
	changed := config
	setTestCodexProfile(t, &changed, codexworker.ProfileConfig{Executable: "codex", WindowsSandbox: "unelevated"})
	restored := changed.ApplyFixedExecutionProfile(fixed)
	if codexProfileSettings(restored).WindowsSandbox != "elevated" {
		t.Fatal("current config changed frozen mode")
	}
	fixed.ImplementationData[0].Payload[0] = '['
	if testVerifyFixedExecutionProfile(fixed) {
		t.Fatal("sandbox mode drift was accepted")
	}
	if runtime.GOOS == "windows" {
		setTestCodexProfile(t, &config, codexworker.ProfileConfig{Executable: "codex"})
		profile = unitProjectProfile(config)
		settings, err := codexworker.DecodeProfileConfig(profile.Freeze().ImplementationData[0].Payload)
		if err != nil || settings.WindowsSandbox != "elevated" {
			t.Fatal("default mode was not frozen")
		}
		settings.WindowsSandbox = "unelevated"
		profile.ImplementationConfigs[0].Payload, _ = codexworker.EncodeProfileConfig(settings)
		if !strings.Contains(profile.StaticUnavailableReason(), "split writable roots") {
			t.Fatal("unsupported profile was offered")
		}
		invalid := Config{Host: "local", Agent: "codex", WorkflowProtocol: WorkerFlowVersion, ExecutionPolicy: workspacePolicyFixture()}
		setTestCodexProfile(t, &invalid, codexworker.ProfileConfig{Executable: "codex", WindowsSandbox: "unelevated"})
		entry, err := invalid.ExecutionDirectory.Resolve(invalid)
		require(t, err)
		if err := entry.ValidateConfig(context.Background(), invalid); err == nil {
			t.Fatal("claim validation accepted unsupported sandbox")
		}
	}
	orcaConfig := Config{ExecutionPolicy: workspacePolicyFixture(), PublicationPolicy: executionPublicationFixture(), PR: hostPublicationProviderFixture(), Host: "orca", Agent: "codex", Repository: "repo", Branch: "main"}
	orcaConfig = bindUnitDirectory(t, orcaConfig)
	orcaProfile := testExecutionProfile(t, ProjectExecutionProfileID, "Terminal", "orca", "codex", codexworker.ProfileConfig{Executable: "codex"}, &orcaworker.ProfileConfig{Executable: "orca", Shell: "powershell.exe", WaitTimeout: "1m"})
	raw, err := codexworker.EncodeProfileConfig(codexworker.ProfileConfig{Executable: "codex"})
	require(t, err)
	orcaProfile.ImplementationConfigs[0].Payload = raw
	orcaProfile = boundProfile(orcaConfig, orcaProfile)
	orcaSettings, err := codexworker.DecodeProfileConfig(orcaProfile.ImplementationConfigs[0].Payload)
	if err != nil || orcaSettings.WindowsSandbox != "" {
		t.Fatal("local default changed Orca path")
	}
	if orcaProfile.StaticUnavailableReason() == "" {
		t.Fatal("terminal profile without explicit native sandbox mapping was offered")
	}
}
