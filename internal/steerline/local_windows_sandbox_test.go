package steerline

import (
	"runtime"
	"strings"
	"testing"
)

func TestLocalWindowsSandboxProfileFreezesAndRestores(t *testing.T) {
	profile := ExecutionProfile{ID: "local", Name: "Local", Host: "local", Agent: "codex", Sandbox: "workspace-write", WindowsSandbox: "elevated"}
	fixed := profile.Freeze()
	if fixed.WindowsSandbox != "elevated" || !verifyFixedExecutionProfile(fixed) {
		t.Fatalf("bad frozen profile: %+v", fixed)
	}
	config := (Config{WindowsSandbox: "unelevated"}).ApplyFixedExecutionProfile(fixed)
	if config.WindowsSandbox != "elevated" {
		t.Fatal("current config changed frozen mode")
	}
	fixed.WindowsSandbox = "unelevated"
	if verifyFixedExecutionProfile(fixed) {
		t.Fatal("sandbox mode drift was accepted")
	}
	if runtime.GOOS == "windows" {
		profile.WindowsSandbox = ""
		if profile.effective().WindowsSandbox != "elevated" || profile.Freeze().WindowsSandbox != "elevated" {
			t.Fatal("default mode was not frozen")
		}
		profile.WindowsSandbox = "unelevated"
		if !strings.Contains(profile.StaticUnavailableReason(), "split writable roots") {
			t.Fatal("unsupported profile was offered")
		}
		config = Config{Host: "local", Agent: "codex", Sandbox: "workspace-write", WindowsSandbox: "unelevated", WorkflowProtocol: WorkerFlowVersion}
		if err := validateWorkflowConfig(&config, nil); err == nil {
			t.Fatal("claim validation accepted unsupported sandbox")
		}
	}
	profile.Host, profile.WindowsSandbox = "orca", ""
	if profile.effective().WindowsSandbox != "" || profile.Freeze().WindowsSandbox != "" {
		t.Fatal("local default changed Orca path")
	}
}
