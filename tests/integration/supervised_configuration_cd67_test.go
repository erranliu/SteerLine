package integration

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	steerlineapp "github.com/erranliu/SteerLine/internal/app"
	codexworker "github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	orcaworker "github.com/erranliu/SteerLine/internal/execution/adapters/orca"
	remote "github.com/erranliu/SteerLine/internal/execution/adapters/orcaremote"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

func TestSupervisedOrcaAcceptsNativeDefaultsAndKeepsStopUnsupported(t *testing.T) {
	c := supervisedAdmissionConfig(t)
	settings := nativeRemoteSettings(c)
	invalid := settings
	invalid.Model = ""
	invalid.ReasoningEffort = "high"
	if _, err := remote.EncodeProfileConfig(invalid); err == nil {
		t.Fatal("Orca effort without model accepted")
	}
	inherited := settings
	inherited.Model = ""
	inherited.ReasoningEffort = ""
	if _, err := remote.EncodeProfileConfig(inherited); err != nil {
		t.Fatal(err)
	}
	remoteSettings := nativeRemoteSettings(c)
	remoteSettings.RemoteCancelAvailable = true
	if _, err := remote.EncodeProfileConfig(remoteSettings); err == nil || !strings.Contains(err.Error(), "fixed unavailable") {
		t.Fatalf("remote cancellation was enabled by its codec: %v", err)
	}
	remoteSettings = nativeRemoteSettings(c)
	remoteSettings.RemoteCancelAvailable = false
	orcaSettings := orcaworker.ProfileConfig{Executable: settings.Executable, Args: settings.Args, Shell: settings.Shell, WaitTimeout: settings.WaitTimeout}
	if _, err := remote.EncodeProfileConfig(remoteSettings); err != nil {
		t.Fatalf("valid Orca profile was rejected: %v", err)
	}
	setNativeImplementationProfiles(t, &c, codexworker.ProfileConfig{Executable: "unused", Model: settings.Model, ReasoningEffort: settings.ReasoningEffort}, &orcaSettings, &remoteSettings)
	if err := c.ValidateProfiles(); err != nil {
		t.Fatalf("valid remote profile rejected: %v", err)
	}
	report, err := (remote.NewBackend(nativeRemoteConfig(c))).Probe(context.Background(), nativeTarget(c), executionbackend.AgentSpec{Name: c.Agent})
	if err != nil {
		t.Fatal(err)
	}
	if declaredCapabilityStatus(report, "stop") != executionbackend.CapabilityUnsupported || declaredCapabilityStatus(report, "start") != executionbackend.CapabilityNotTested {
		t.Fatalf("supervised capabilities = %+v", report.Capabilities)
	}
	if _, supported := interface{}(remote.NewBackend(nativeRemoteConfig(c))).(executionbackend.ExecutionStopper); supported {
		t.Fatal("remote backend unexpectedly implements process cancellation")
	}
}

func TestSupervisedOrcaExecutionBindingRequiresExactDispatchIdentity(t *testing.T) {
	c := supervisedAdmissionConfig(t)
	entry, err := c.ExecutionDirectory.Resolve(c.ExecutionConfig())
	require(t, err)
	runID, attemptID := newID(), newID()
	ref := executionbackend.ExecutionRef{Version: executionbackend.ExecutionRecordVersion, Backend: executionbackend.BackendOrca, Protocol: executionbackend.ProtocolOrcaSupervised, Target: nativeTarget(c), Agent: executionbackend.AgentSpec{Name: c.Agent}, RunID: runID, AttemptID: attemptID, WorkspacePath: "C:/remote/worktree", WorkspaceBranch: "erranliu/remote", BaseCommit: strings.Repeat("a", 40), External: executionbackend.ProcessIdentity{Kind: "orca_supervised", RuntimeID: "runtime-1", OrchestrationRun: "run-1", Task: "task-1", Dispatch: "dispatch-1", ExecutionHost: "host-1", WorktreeID: "repo-1::C:/remote/worktree"}}
	if err := entry.ValidateRef(ref); err != nil {
		t.Fatalf("complete supervised binding rejected: %v", err)
	}
	ref.External.Dispatch = ""
	if err := entry.ValidateRef(ref); err == nil {
		t.Fatal("supervised binding without its exact Dispatch was accepted")
	}
	entry, err = c.ExecutionDirectory.Resolve(c.ExecutionConfig())
	backend, protocol := string(entry.Key.Backend), string(entry.Key.Protocol)
	if err != nil || backend != string(executionbackend.BackendOrca) || protocol != string(executionbackend.ProtocolOrcaSupervised) {
		t.Fatalf("backend resolution = %q/%q, %v", backend, protocol, err)
	}
}

func supervisedAdmissionConfig(t *testing.T) steerlineapp.Config {
	t.Setenv("STEERLINE_FAKE_SUPERVISED_REMOTE_URL", "https://git.example/owner/repo.git")
	return builtinFixtureConfig(t, steerlineapp.Config{Host: "orca_supervised", Agent: "codex", TaskAuthority: "native", WorkflowProtocol: executionmodel.Version, PublicationPolicy: executionbackend.PublicationPolicy{Publisher: "worker"}, PR: hostPublicationProviderFixture()}, codexworker.ProfileConfig{Executable: "codex", Model: "gpt-6-luna", ReasoningEffort: "high"}, &orcaworker.ProfileConfig{Executable: "must-not-run", Shell: "powershell.exe", WaitTimeout: "5s"}, &remote.ProfileConfig{ConnectionID: "home-connection", EnvironmentID: "env-home", RepositorySelector: "id:repo-test", CoordinatorTerminal: "term-coordinator", PollInterval: "2s", CapabilityReportVersion: 1})
}

func nativeTerminalSettings(c steerlineapp.Config) orcaworker.ProfileConfig {
	for _, profile := range c.ExecutionProfiles {
		for _, input := range profile.ImplementationConfigs {
			if input.ImplementationKey == orcaworker.ProfileCodecKey {
				value, _ := orcaworker.DecodeProfileConfig(input.Payload)
				return value
			}
		}
	}
	return orcaworker.ProfileConfig{}
}

func nativeRemoteSettings(c steerlineapp.Config) remote.ProfileConfig {
	for _, profile := range c.ExecutionProfiles {
		for _, input := range profile.ImplementationConfigs {
			if input.ImplementationKey == remote.ProfileCodecKey {
				value, _ := remote.DecodeProfileConfig(input.Payload)
				return value
			}
		}
	}
	return remote.ProfileConfig{}
}

func nativeRemoteConfig(c steerlineapp.Config) remote.Config {
	r := nativeRemoteSettings(c)
	client := orcaworker.New(orcaworker.Config{Repository: c.Repository, CommandTimeout: c.CommandTimeout, Output: func(ctx context.Context, timeout time.Duration, args ...string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return exec.CommandContext(ctx, os.Args[0], append([]string{"--fake-supervised"}, args...)...).Output()
	}})
	return remote.Config{Repository: c.Repository, ProjectID: c.Identity(), Agent: c.Agent, Model: r.Model, ReasoningEffort: r.ReasoningEffort, Settings: remote.Settings{ConnectionID: r.ConnectionID, EnvironmentID: r.EnvironmentID, RepositorySelector: r.RepositorySelector, CoordinatorTerminal: r.CoordinatorTerminal, PollInterval: r.PollInterval}, Call: client.Raw, Origin: func(context.Context) ([]byte, error) { return []byte("https://git.example/owner/repo.git"), nil }, CommandTimeout: r.WaitTimeout}
}

func declaredCapabilityStatus(r executionbackend.CapabilityReport, name string) executionbackend.CapabilityStatus {
	for _, c := range r.Capabilities {
		if c.Name == name {
			return c.Status
		}
	}
	return ""
}
