package steerlineapp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	"github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	remote "github.com/erranliu/SteerLine/internal/execution/adapters/orcaremote"
	"github.com/erranliu/SteerLine/internal/execution/adapters/orca"
)

func TestConfigLoadsLocalAndRemoteOrcaProfilesWithoutProbingTargets(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("local Orca terminal target uses Windows PowerShell")
	}
	base, _ := nativeTaskFixture(t)
	t.Setenv("STEERLINE_HOME", t.TempDir())
	localOrca := testExecutionProfile(t, "orca-local", "Orca 本机", "orca", "codex", codexworker.ProfileConfig{Executable: "codex", WindowsSandbox: "unelevated"}, &orcaworker.ProfileConfig{Executable: "orca", Shell: "powershell.exe", WaitTimeout: "15m"})
	remoteOrca := testExecutionProfile(t, "orca-remote", "Orca 远程", "orca_supervised", "codex", codexworker.ProfileConfig{Executable: "codex", Model: "chosen-model", ReasoningEffort: "xhigh"}, &orcaworker.ProfileConfig{Executable: "orca", Shell: "cmd.exe", WaitTimeout: "15m"}, &remote.ProfileConfig{ConnectionID: "connection", EnvironmentID: "environment", RepositorySelector: "repo", CoordinatorTerminal: "terminal", PollInterval: "5s", CapabilityReportVersion: 1})
	local := testExecutionProfile(t, "local", "本地", "local", "codex", codexworker.ProfileConfig{Executable: "codex"}, nil)
	writeGlobalExecutionFixture(t, []ExecutionProfile{local, localOrca, remoteOrca}, "local")
	input := map[string]any{"publication_policy": executionbackend.PublicationPolicy{Publisher: executionbackend.PublisherWorker}, "pr": hostPublicationProviderFixture(), "execution_policy": workspacePolicyFixture(), "repository": base.Repository, "branch": "main", "task_authority": "native", "workflow_protocol": WorkerFlowVersion}
	path := filepath.Join(t.TempDir(), "config.json")
	write := func() {
		t.Helper()
		data, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write()
	loaded, err := LoadQueryConfig(path)
	if err != nil || len(loaded.AvailableExecutionProfiles()) != 3 || loaded.DefaultProfileID() != "local" {
		t.Fatalf("configured profiles were not loaded: %+v / %v", loaded.ExecutionProfiles, err)
	}
	for _, profile := range loaded.AvailableExecutionProfiles() {
		selected, chosen, err := loaded.ResolveExecutionProfile(profile.ID, profile.Digest())
		if profile.Host == "orca_supervised" {
			if err == nil || !strings.Contains(err.Error(), "remote worker-start enforcement evidence") || profile.StaticUnavailableReason() == "" {
				t.Fatalf("unverified remote enforcement was accepted: %v", err)
			}
			continue
		}
		if err != nil || selected.Host != profile.Host {
			t.Fatalf("Orca profile changed route: %+v / %v", chosen, err)
		}
		if profile.StaticUnavailableReason() != "" {
			t.Fatalf("Orca profile availability is wrong: %+v", profile)
		}
	}
}

func TestUnsupportedLocalAgentProfileIsMarkedUnavailable(t *testing.T) {
	profile := ExecutionProfile{ID: "local-pi", Name: "Local Pi", Host: "local", Agent: "pi"}
	config := bindUnitDirectory(t, Config{ExecutionProfiles: []ExecutionProfile{profile}})
	profile = boundProfile(config, profile)
	if reason := profile.StaticUnavailableReason(); !strings.Contains(reason, "no implementation") {
		t.Fatalf("unsupported native agent was shown as selectable: %q", reason)
	}
}
