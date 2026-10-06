package integration

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	steerlineapp "github.com/erranliu/SteerLine/internal/app"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func TestNativeEvidenceCertificationRequiresFixedTargetAndVersion(t *testing.T) {
	fixture := newLifecycleProject(t, "local")
	c := fixture.config
	executionConfig := c.ExecutionConfig()
	entry, err := c.ExecutionDirectory.Resolve(executionConfig)
	require(t, err)
	probe, err := entry.ProbeRequirements(context.Background(), executionConfig)
	require(t, err)
	path := filepath.Join(t.TempDir(), "local-native.json")
	evidence := map[string]any{"version": 1, "captured_at": time.Now().UTC(), "platform": runtime.GOOS + "/" + runtime.GOARCH, "backend": "local", "protocol": "local_codex_app_server_v1", "target_id": probe.Target.ID, "agent": "codex", "codex_version": probe.PlatformVersion, "output_sha256": strings.Repeat("a", 64), "output_bytes": 77, "stop_status": "confirmed", "process_absent": true}
	write := func() {
		t.Helper()
		data, err := json.Marshal(evidence)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write()
	report, err := steerlineapp.CertifyExecutionBackend(context.Background(), c, []string{path})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"start", "observe", "stop", "output_reading"} {
		if integrationCapabilityStatus(report, name) != executionbackend.CapabilityVerified {
			t.Fatalf("%s not verified from native evidence: %#v", name, report)
		}
	}
	if integrationCapabilityStatus(report, "retry") != executionbackend.CapabilityNotTested {
		t.Fatalf("legacy backend retry capability was retained: %#v", report.Capabilities)
	}
	c.ExecutionRequirements.Capabilities = []string{"workspace_inspection"}
	_, err = c.EvaluateRequirements(context.Background())
	require(t, err)
	evidence["protocol"] = "local_process"
	write()
	if _, err = steerlineapp.CertifyExecutionBackend(context.Background(), c, []string{path}); err == nil {
		t.Fatal("old protocol evidence satisfied registered app-server route")
	}
	evidence["protocol"] = "local_codex_app_server_v1"
	evidence["target_id"] = "different-target"
	write()
	if _, err = steerlineapp.CertifyExecutionBackend(context.Background(), c, []string{path}); err == nil {
		t.Fatal("foreign target evidence certified")
	}
	if _, err = c.EvaluateRequirements(context.Background()); err != nil {
		t.Fatalf("certified evidence did not survive source artifact change: %v", err)
	}
}

func TestOrcaCertificationRequiresAgentAndTerminalEvidence(t *testing.T) {
	fixture := newLifecycleProject(t, "orca")
	c := fixture.config
	executionConfig := c.ExecutionConfig()
	entry, err := c.ExecutionDirectory.Resolve(executionConfig)
	require(t, err)
	probe, err := entry.ProbeRequirements(context.Background(), executionConfig)
	require(t, err)
	common := map[string]any{"version": 1, "captured_at": time.Now().UTC(), "platform": runtime.GOOS + "/" + runtime.GOARCH, "backend": "orca", "protocol": "orca_terminal_v2", "target_id": probe.Target.ID, "agent": "codex", "orca_version": probe.PlatformVersion, "codex_version": probe.AgentVersion, "stop_status": "confirmed"}
	write := func(name string, fields map[string]any) string {
		t.Helper()
		copy := map[string]any{}
		for key, value := range common {
			copy[key] = value
		}
		for key, value := range fields {
			copy[key] = value
		}
		data, err := json.Marshal(copy)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(t.TempDir(), name+".json")
		if err = os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	agent := write("agent", map[string]any{"worktree_created": true, "agent_terminal_bound": true, "initial_output_observed": true, "git_clean": true})
	adapter := write("adapter", map[string]any{"reconnect_matched": true, "worktree_retained": true, "output_bytes": 22, "output_sha256": strings.Repeat("b", 64), "output_source": "stream"})
	workspace := write("workspace", map[string]any{"workspace_inspected": true, "base_ancestor": true, "git_clean": true})
	if _, err = steerlineapp.CertifyExecutionBackend(context.Background(), c, []string{adapter}); err == nil {
		t.Fatal("Orca start was certified without agent evidence")
	}
	report, err := steerlineapp.CertifyExecutionBackend(context.Background(), c, []string{agent, adapter, workspace})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"start", "observe", "stop", "output_reading", "workspace_inspection"} {
		if integrationCapabilityStatus(report, name) != executionbackend.CapabilityVerified {
			t.Fatalf("%s not verified: %#v", name, report)
		}
	}
}

func integrationCapabilityStatus(report executionbackend.CapabilityReport, name string) executionbackend.CapabilityStatus {
	for _, capability := range report.Capabilities {
		if capability.Name == name {
			return capability.Status
		}
	}
	return executionbackend.CapabilityNotTested
}
