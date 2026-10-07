package app

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/erranliu/SteerLine/internal/execution"
	"github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	"github.com/erranliu/SteerLine/internal/execution/adapters/orca"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func TestProfileSelectionDigestMatchesServicePreflight(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("current execution policy mappings require Windows")
	}
	t.Setenv("STEERLINE_HOME", t.TempDir())
	t.Setenv("STEERLINE_APP_SERVER_FIXTURE", "1")
	repository := configTestRepository(t)
	if output, err := exec.Command("git", "-C", repository, "remote", "add", "origin", "https://github.com/owner/repo.git").CombinedOutput(); err != nil {
		t.Fatalf("configure fixture remote: %v: %s", err, output)
	}
	profiles := mixedPublicationProfiles()
	for i := range profiles {
		settings := codexProfileFromInputs(profiles[i].ImplementationConfigs)
		settings.Executable = os.Args[0]
		payload, err := codexworker.EncodeProfileConfig(settings)
		require(t, err)
		profiles[i].ImplementationConfigs[0].Payload = payload
	}
	global, err := json.Marshal(map[string]any{"version": 2, "default_execution_profile": "local", "execution_profiles": profiles})
	require(t, err)
	require(t, os.WriteFile(GlobalExecutionConfigPath(), global, 0600))
	provider := hostPublicationProviderFixture()
	provider.Close = executionbackend.Command{Executable: "gh", Args: []string{"pr", "close", "{pr_url}", "--repo", "{repository}"}}
	project, err := json.Marshal(map[string]any{
		"repository": repository, "remote": "origin", "branch": "main", "task_authority": "native", "workflow_protocol": WorkerFlowVersion,
		"git":                executionbackend.Command{Executable: "git"},
		"publication_policy": map[string]string{"publisher": "worker"}, "pr": provider,
	})
	require(t, err)
	path := filepath.Join(t.TempDir(), "project.json")
	require(t, os.WriteFile(path, project, 0600))
	client, err := LoadConfig(path)
	require(t, err)
	// Match the console's makeProjectRef path, without running service preflight.
	require(t, client.ResolveIdentity(context.Background()))
	chosen := client.AvailableExecutionProfiles()
	service, err := LoadConfig(path)
	require(t, err)
	require(t, Preflight(context.Background(), &service, true))
	for _, profile := range chosen {
		_, selected, err := service.ResolveExecutionProfile(profile.ID, profile.Digest())
		require(t, err)
		if selected.Digest() != profile.Digest() || selected.PublicationPolicy != profile.PublicationPolicy {
			t.Fatal("service preflight changed the console's chosen profile")
		}
	}
	if _, err := os.Stat(client.RunDir); !os.IsNotExist(err) {
		t.Fatalf("read-only profile selection initialized runtime state: %v", err)
	}
}

func mixedPublicationProfiles() []ExecutionProfile {
	localCodex, _ := codexworker.EncodeProfileConfig(codexworker.ProfileConfig{Executable: "codex.exe"})
	orcaCodex, _ := codexworker.EncodeProfileConfig(codexworker.ProfileConfig{Executable: "codex.exe"})
	orcaOptions, _ := orcaworker.EncodeProfileConfig(orcaworker.ProfileConfig{Executable: "orca.exe", Shell: "powershell.exe", WaitTimeout: "15m"})
	return []ExecutionProfile{
		{ID: "local", Name: "Local", Host: "local", Agent: "codex", ImplementationConfigs: []executionbackend.ImplementationConfigInput{{ImplementationKey: codexworker.ProfileCodecKey, CodecVersion: 2, Payload: localCodex}}},
		{ID: "orca", Name: "Orca", Host: "orca", Agent: "codex", ImplementationConfigs: []executionbackend.ImplementationConfigInput{{ImplementationKey: codexworker.ProfileCodecKey, CodecVersion: 2, Payload: orcaCodex}, {ImplementationKey: orcaworker.ProfileCodecKey, CodecVersion: 1, Payload: orcaOptions}}},
	}
}

func TestSimplifiedWorkerOnlyPublication(t *testing.T) {
	for _, publisher := range []string{"host_service", "execution_profile", ""} {
		if (executionbackend.PublicationPolicy{Publisher: publisher}).Validate() == nil {
			t.Fatalf("retired publisher accepted: %q", publisher)
		}
	}
	if err := (executionbackend.PublicationPolicy{Publisher: executionbackend.PublisherWorker}).Validate(); err != nil {
		t.Fatal(err)
	}
	var profile execution.ExecutionProfile
	if err := decodeStrictJSON([]byte(`{"id":"local","name":"Local","publication_policy":{"publisher":"worker"}}`), &profile); err == nil {
		t.Fatal("per-profile publication override accepted")
	}
}
