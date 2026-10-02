package steerline

import (
	"runtime"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
)

func TestLocalWindowsSandboxProfileFreezesAndRestores(t *testing.T) {
	config := Config{ExecutionPolicy: workspacePolicyFixture(), PublicationPolicy: executionPublicationFixture(), PR: hostPublicationProviderFixture(), Host: "local", Agent: "codex", Repository: "repo", Branch: "main"}
	setTestCodexProfile(t, &config, codexworker.ProfileConfig{Executable: "codex", WindowsSandbox: "elevated"})
	profile := config.projectExecutionProfile()
	fixed := profile.Freeze(configurationProjectIdentity(config))
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
		profile = config.projectExecutionProfile()
		settings, err := codexworker.DecodeProfileConfig(profile.Freeze(configurationProjectIdentity(config)).ImplementationData[0].Payload)
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
		if err := validateWorkflowConfig(&invalid, nil); err == nil {
			t.Fatal("claim validation accepted unsupported sandbox")
		}
	}
	orcaConfig := Config{ExecutionPolicy: workspacePolicyFixture(), PublicationPolicy: executionPublicationFixture(), PR: hostPublicationProviderFixture(), Host: "orca", Agent: "codex", Repository: "repo", Branch: "main"}
	setTestImplementationProfiles(t, &orcaConfig, codexworker.ProfileConfig{Executable: "codex"}, &orcaworker.ProfileConfig{Executable: "orca", Shell: "powershell.exe", WaitTimeout: "1m"})
	orcaProfile := orcaConfig.projectExecutionProfile()
	orcaSettings, err := codexworker.DecodeProfileConfig(orcaProfile.Freeze(configurationProjectIdentity(orcaConfig)).ImplementationData[0].Payload)
	if err != nil || orcaSettings.WindowsSandbox != "" {
		t.Fatal("local default changed Orca path")
	}
}
