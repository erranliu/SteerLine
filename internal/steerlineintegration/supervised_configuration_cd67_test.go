package steerlineintegration

import (
	"context"
	"github.com/erranliu/SteerLine/internal/steerline"
	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	remote "github.com/erranliu/SteerLine/internal/steerline/orca_remote_worker"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestSupervisedOrcaRequiresPinnedModelAndKeepsStopUnsupported(t *testing.T) {
	c := supervisedAdmissionConfig(t)
	codexSettings := nativeProfileSettings(c)
	codexSettings.Model = ""
	setNativeProfile(t, &c, codexSettings)
	if err := c.ValidateProfiles(); err == nil {
		t.Fatal("missing model unexpectedly accepted")
	}
	codexSettings.Model = "gpt-6-luna"
	setNativeProfile(t, &c, codexSettings)
	remoteSettings := nativeRemoteSettings(c)
	remoteSettings.RemoteCancelAvailable = true
	orcaSettings := nativeTerminalSettings(c)
	if _, err := remote.EncodeProfileConfig(remoteSettings); err == nil || !strings.Contains(err.Error(), "fixed unavailable") {
		t.Fatalf("remote cancellation was enabled by its codec: %v", err)
	}
	remoteSettings = nativeRemoteSettings(c)
	remoteSettings.RemoteCancelAvailable = false
	remoteSettings.ExpectedCodexVersion = ""
	orcaSettings = nativeTerminalSettings(c)
	if _, err := remote.EncodeProfileConfig(remoteSettings); err != nil {
		t.Fatalf("optional native version pin was required by the codec: %v", err)
	}
	setNativeImplementationProfiles(t, &c, nativeProfileSettings(c), &orcaSettings, &remoteSettings)
	if err := c.ValidateProfiles(); err == nil || !strings.Contains(err.Error(), "enforcement evidence") {
		t.Fatalf("missing remote permission evidence bypassed actual admission: %v", err)
	}
	report, err := (remote.NewBackend(nativeRemoteConfig(c))).Probe(context.Background(), nativeTarget(c), executionapi.AgentSpec{Name: c.Agent})
	if err != nil {
		t.Fatal(err)
	}
	if declaredCapabilityStatus(report, "stop") != executionapi.CapabilityUnsupported || declaredCapabilityStatus(report, "start") != executionapi.CapabilityNotTested {
		t.Fatalf("supervised capabilities = %+v", report.Capabilities)
	}
	if _, supported := interface{}(remote.NewBackend(nativeRemoteConfig(c))).(executionapi.ExecutionStopper); supported {
		t.Fatal("remote backend unexpectedly implements process cancellation")
	}

}

func TestSupervisedOrcaExecutionBindingRequiresExactDispatchIdentity(t *testing.T) {
	c := supervisedAdmissionConfig(t)
	entry, err := c.ExecutionDirectory.Resolve(c)
	require(t, err)
	runID, attemptID := newID(), newID()
	ref := executionapi.ExecutionRef{Version: executionapi.ExecutionRecordVersion, Backend: executionapi.BackendOrca, Protocol: executionapi.ProtocolOrcaSupervised, Target: nativeTarget(c), Agent: executionapi.AgentSpec{Name: c.Agent}, RunID: runID, AttemptID: attemptID, WorkspacePath: "C:/remote/worktree", WorkspaceBranch: "erranliu/remote", BaseCommit: strings.Repeat("a", 40), External: executionapi.ProcessIdentity{Kind: "orca_supervised", RuntimeID: "runtime-1", OrchestrationRun: "run-1", Task: "task-1", Dispatch: "dispatch-1", ExecutionHost: "host-1", WorktreeID: "repo-1::C:/remote/worktree"}}
	if err := entry.ValidateRef(ref); err != nil {
		t.Fatalf("complete supervised binding rejected: %v", err)
	}
	ref.External.Dispatch = ""
	if err := entry.ValidateRef(ref); err == nil {
		t.Fatal("supervised binding without its exact Dispatch was accepted")
	}
	entry, err = c.ExecutionDirectory.Resolve(c)
	backend, protocol := string(entry.Key.Backend), string(entry.Key.Protocol)
	if err != nil || backend != string(executionapi.BackendOrca) || protocol != string(executionapi.ProtocolOrcaSupervised) {
		t.Fatalf("backend resolution = %q/%q, %v", backend, protocol, err)
	}
}

func supervisedAdmissionConfig(t *testing.T) steerline.Config {
	t.Setenv("STEERLINE_FAKE_SUPERVISED_REMOTE_URL", "https://git.example/owner/repo.git")
	return builtinFixtureConfig(t, steerline.Config{Host: "orca_supervised", Agent: "codex", ExecutionPolicy: workspacePolicyFixture(), PublicationPolicy: executionapi.PublicationPolicy{Publisher: "worker"}, PR: hostPublicationProviderFixture()}, codexworker.ProfileConfig{Executable: "codex", Model: "gpt-6-luna", ReasoningEffort: "high"}, &orcaworker.ProfileConfig{Executable: "must-not-run", Shell: "powershell.exe", WaitTimeout: "5s"}, &remote.ProfileConfig{ConnectionID: "home-connection", EnvironmentID: "env-home", RepositorySelector: "id:repo-test", CoordinatorTerminal: "term-coordinator", PollInterval: "2s", CapabilityReportVersion: 1})
}
func nativeTerminalSettings(c steerline.Config) orcaworker.ProfileConfig {
	for _, p := range c.ExecutionProfiles {
		for _, input := range p.ImplementationConfigs {
			if input.ImplementationKey == orcaworker.ProfileCodecKey {
				v, _ := orcaworker.DecodeProfileConfig(input.Payload)
				return v
			}
		}
	}
	return orcaworker.ProfileConfig{}
}
func nativeRemoteSettings(c steerline.Config) remote.ProfileConfig {
	for _, p := range c.ExecutionProfiles {
		for _, input := range p.ImplementationConfigs {
			if input.ImplementationKey == remote.ProfileCodecKey {
				v, _ := remote.DecodeProfileConfig(input.Payload)
				return v
			}
		}
	}
	return remote.ProfileConfig{}
}
func nativeRemoteConfig(c steerline.Config) remote.Config {
	w, o, r := nativeProfileSettings(c), nativeTerminalSettings(c), nativeRemoteSettings(c)
	client := orcaworker.New(orcaworker.Config{Repository: c.Repository, CommandTimeout: c.CommandTimeout, Output: func(ctx context.Context, timeout time.Duration, args ...string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return exec.CommandContext(ctx, os.Args[0], append([]string{"--fake-supervised"}, args...)...).Output()
	}})
	return remote.Config{Repository: c.Repository, ProjectID: c.Identity(), Agent: c.Agent, Model: w.Model, ReasoningEffort: w.ReasoningEffort, Settings: remote.Settings{ConnectionID: r.ConnectionID, EnvironmentID: r.EnvironmentID, RepositorySelector: r.RepositorySelector, CoordinatorTerminal: r.CoordinatorTerminal, ExpectedCodexVersion: r.ExpectedCodexVersion, PollInterval: r.PollInterval}, Call: client.Raw, Origin: func(context.Context) ([]byte, error) { return []byte("https://git.example/owner/repo.git"), nil }, CommandTimeout: o.WaitTimeout}
}
func declaredCapabilityStatus(r executionapi.CapabilityReport, name string) executionapi.CapabilityStatus {
	for _, c := range r.Capabilities {
		if c.Name == name {
			return c.Status
		}
	}
	return ""
}
