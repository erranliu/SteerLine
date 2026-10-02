package steerline

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
	"github.com/erranliu/SteerLine/internal/steerline/publication"
)

func TestProfileSelectionDigestMatchesServicePreflight(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("current execution policy mappings require Windows")
	}
	t.Setenv("STEERLINE_HOME", t.TempDir())
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
	require(t, os.WriteFile(fixtureGlobalExecutionConfigPath(), global, 0600))
	project, err := json.Marshal(map[string]any{
		"repository": repository, "remote": "origin", "branch": "main", "task_authority": "native", "workflow_protocol": WorkerFlowVersion,
		"git": Command{Executable: "git"}, "execution_policy": workspacePolicyFixture(),
		"publication_policy": map[string]string{"publisher": PublisherExecutionProfile}, "pr": hostPublicationProviderFixture(),
	})
	require(t, err)
	path := filepath.Join(t.TempDir(), "project.json")
	require(t, os.WriteFile(path, project, 0600))
	client, err := loadFixtureConfig(path)
	require(t, err)
	// Match the console's makeProjectRef path, without running service preflight.
	require(t, client.ResolveIdentity(context.Background()))
	chosen := client.AvailableExecutionProfiles()
	service, err := loadFixtureConfig(path)
	require(t, err)
	require(t, service.Preflight(context.Background(), true))
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
	host := executionapi.PublicationPolicy{Publisher: "host_service"}
	worker := executionapi.PublicationPolicy{Publisher: "worker"}
	localCodex, _ := codexworker.EncodeProfileConfig(codexworker.ProfileConfig{Executable: "codex.exe", WindowsSandbox: "elevated"})
	orcaCodex, _ := codexworker.EncodeProfileConfig(codexworker.ProfileConfig{Executable: "codex.exe", WindowsSandbox: "unelevated"})
	orcaOptions, _ := orcaworker.EncodeProfileConfig(orcaworker.ProfileConfig{Executable: "orca.exe", Shell: "powershell.exe", WaitTimeout: "15m"})
	return []ExecutionProfile{
		{ID: "local", Name: "Local", Host: "local", Agent: "codex", ConfiguredPublicationPolicy: &host, ImplementationConfigs: []executionapi.ImplementationConfigInput{{ImplementationKey: codexworker.ProfileCodecKey, CodecVersion: 1, Payload: localCodex}}},
		{ID: "orca", Name: "Orca", Host: "orca", Agent: "codex", ConfiguredPublicationPolicy: &worker, ImplementationConfigs: []executionapi.ImplementationConfigInput{{ImplementationKey: codexworker.ProfileCodecKey, CodecVersion: 1, Payload: orcaCodex}, {ImplementationKey: orcaworker.ProfileCodecKey, CodecVersion: 1, Payload: orcaOptions}}},
	}
}

func TestProfilePublicationMixedRoutesAndFrozenResponsibility(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("current execution policy mappings require Windows")
	}
	t.Setenv("STEERLINE_HOME", t.TempDir())
	writeJSON := func(path string, value any) {
		t.Helper()
		data, err := json.Marshal(value)
		require(t, err)
		require(t, os.WriteFile(path, data, 0600))
	}
	profiles := mixedPublicationProfiles()
	writeJSON(fixtureGlobalExecutionConfigPath(), map[string]any{"default_execution_profile": "local", "execution_profiles": profiles})
	path := filepath.Join(t.TempDir(), "project.json")
	writeJSON(path, map[string]any{
		"repository": configTestRepository(t), "branch": "main", "task_authority": "native", "workflow_protocol": WorkerFlowVersion,
		"execution_policy": workspacePolicyFixture(), "publication_policy": map[string]string{"publisher": PublisherExecutionProfile}, "pr": hostPublicationProviderFixture(),
	})
	c, err := loadFixtureConfig(path)
	require(t, err)
	if c.PublicationPolicy.Publisher != "host_service" {
		t.Fatal("default route did not resolve its explicit publisher")
	}
	for _, profile := range c.AvailableExecutionProfiles() {
		original := *profile.ConfiguredPublicationPolicy
		profile.ConfiguredPublicationPolicy.Publisher = "changed"
		for _, fresh := range c.AvailableExecutionProfiles() {
			if fresh.ID == profile.ID && *fresh.ConfiguredPublicationPolicy != original {
				t.Fatal("returned profile aliases the configured publication responsibility")
			}
		}
		*profile.ConfiguredPublicationPolicy = original
		selected, bound, err := c.ResolveExecutionProfile(profile.ID, profile.Digest())
		require(t, err)
		if bound.PublicationPolicy != *profile.ConfiguredPublicationPolicy || selected.PublicationPolicy != bound.PublicationPolicy || !bound.MatchesConfig(selected) {
			t.Fatal("selected route lost its publication responsibility")
		}
		if (bound.Publication.Create.Executable != "") != (bound.Host == "local") {
			t.Fatal("publication create command was bound using another route's policy")
		}
		fixed := bound.Freeze()
		if !testVerifyFixedExecutionProfile(fixed) {
			t.Fatal("resolved profile cannot be restored from its execution snapshot")
		}
		before := bound.Digest()
		unconfigured := bound
		unconfigured.ConfiguredPublicationPolicy = nil
		if unconfigured.Digest() != before {
			t.Fatal("configuration-only policy changed the frozen digest")
		}
		changed := c
		changed.PublicationPolicy = executionapi.PublicationPolicy{Publisher: "worker"}
		changed.PR = publication.Config{Kind: "gitea", BaseURL: "https://changed.invalid", Repository: "changed/repo"}
		restored := changed.ApplyFixedExecutionProfile(fixed)
		if restored.PublicationPolicy != fixed.PublicationPolicy || digestValue(restored.publicationSettings()) != digestValue(fixed.Publication) || !restored.projectExecutionProfile().MatchesConfig(restored) {
			t.Fatal("current profile/project changed the original execution publisher/provider")
		}
	}
	// Switching repeatedly on a resolved Config must still consult each profile.
	orca, _, err := c.ResolveExecutionProfile("orca", "")
	require(t, err)
	local, _, err := orca.ResolveExecutionProfile("local", "")
	require(t, err)
	if local.PublicationPolicy.Publisher != "host_service" || orca.PublicationPolicy.Publisher != "worker" {
		t.Fatal("last selected publisher leaked into another profile")
	}
}

func TestProfilePublicationRequiresExplicitProjectAndProfilePolicies(t *testing.T) {
	c := Config{ExecutionPolicy: workspacePolicyFixture(), PublicationPolicy: executionapi.PublicationPolicy{Publisher: PublisherExecutionProfile}, PR: hostPublicationProviderFixture(), ExecutionProfiles: mixedPublicationProfiles(), globalExecution: true}
	for _, publisher := range []string{"", PublisherExecutionProfile, "automatic"} {
		profiles := mixedPublicationProfiles()
		profiles[1].ConfiguredPublicationPolicy = &executionapi.PublicationPolicy{Publisher: publisher}
		c.ExecutionProfiles = profiles
		if err := validateExecutionProfiles(c); err == nil || !strings.Contains(err.Error(), "publication_policy.publisher") {
			t.Fatalf("invalid unresolved publisher accepted: %q / %v", publisher, err)
		}
	}
	c.ExecutionProfiles = mixedPublicationProfiles()
	c.ExecutionProfiles[1].ConfiguredPublicationPolicy = nil
	if err := validateExecutionProfiles(c); err == nil || !strings.Contains(err.Error(), "requires publication_policy") {
		t.Fatalf("missing profile responsibility accepted: %v", err)
	}
	c.PublicationPolicy.Publisher = "host_service"
	c.ExecutionProfiles = mixedPublicationProfiles()
	for _, profile := range c.AvailableExecutionProfiles() {
		if profile.PublicationPolicy.Publisher != "host_service" {
			t.Fatal("global profile silently overrode a fixed project policy")
		}
	}
	if err := (executionapi.PublicationPolicy{Publisher: PublisherExecutionProfile}).Validate(); err == nil {
		t.Fatal("unresolved project selection accepted as an execution responsibility")
	}
}
