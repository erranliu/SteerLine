package integration

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	steerlineapp "github.com/erranliu/SteerLine/internal/app"
	"github.com/erranliu/SteerLine/internal/execution"
	codexworker "github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	orcaworker "github.com/erranliu/SteerLine/internal/execution/adapters/orca"
	remote "github.com/erranliu/SteerLine/internal/execution/adapters/orcaremote"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
)

// builtinFixtureConfig loads an app-owned project config with an injected
// implementation directory. Lower execution APIs receive only its selected
// frozen profile through Config.ExecutionConfig().
func builtinFixtureConfig(t *testing.T, c steerlineapp.Config, worker codexworker.ProfileConfig, terminal *orcaworker.ProfileConfig, remoteSettings ...*remote.ProfileConfig) steerlineapp.Config {
	t.Helper()
	if c.Repository == "" {
		c.Repository = filepath.Join(t.TempDir(), "repository")
		require(t, os.MkdirAll(c.Repository, 0700))
		command := exec.Command("git", "init", "--quiet", "--initial-branch=main", c.Repository)
		if out, err := command.CombinedOutput(); err != nil {
			t.Fatalf("init profile fixture repository: %v: %s", err, out)
		}
	}
	if c.Branch == "" {
		c.Branch = "main"
	}
	if c.Remote == "" {
		c.Remote = "origin"
	}
	if c.Git.Executable == "" {
		c.Git.Executable = "git"
	}
	if c.CommandTimeout == "" {
		c.CommandTimeout = "15s"
	}
	if c.PR.BaseURL != "" && c.PR.Repository != "" {
		remoteURL := strings.TrimSuffix(c.PR.BaseURL, "/") + "/" + c.PR.Repository + ".git"
		setRemote := exec.Command("git", "-C", c.Repository, "remote", "set-url", c.Remote, remoteURL)
		if out, err := setRemote.CombinedOutput(); err != nil {
			addRemote := exec.Command("git", "-C", c.Repository, "remote", "add", c.Remote, remoteURL)
			if addOut, addErr := addRemote.CombinedOutput(); addErr != nil {
				t.Fatalf("configure profile fixture remote: set=%v %s add=%v %s", err, out, addErr, addOut)
			}
		}
	}
	profile := execution.ExecutionProfile{ID: execution.ProjectExecutionProfileID, Name: "Native fixture", Host: c.Host, Agent: c.Agent}
	payload, err := codexworker.EncodeProfileConfig(worker)
	require(t, err)
	profile.ImplementationConfigs = append(profile.ImplementationConfigs, executionbackend.ImplementationConfigInput{ImplementationKey: codexworker.ProfileCodecKey, CodecVersion: 2, Payload: payload})
	if terminal != nil {
		payload, err = orcaworker.EncodeProfileConfig(*terminal)
		require(t, err)
		profile.ImplementationConfigs = append(profile.ImplementationConfigs, executionbackend.ImplementationConfigInput{ImplementationKey: orcaworker.ProfileCodecKey, CodecVersion: 1, Payload: payload})
	}
	if len(remoteSettings) > 0 && remoteSettings[0] != nil {
		payload, err = remote.EncodeProfileConfig(*remoteSettings[0])
		require(t, err)
		profile.ImplementationConfigs = append(profile.ImplementationConfigs, executionbackend.ImplementationConfigInput{ImplementationKey: remote.ProfileCodecKey, CodecVersion: 1, Payload: payload})
	}
	global, err := json.Marshal(map[string]any{"version": 2, "default_execution_profile": profile.ID, "execution_profiles": []execution.ExecutionProfile{profile}})
	require(t, err)
	globalPath := steerlineapp.GlobalExecutionConfigPath()
	require(t, os.MkdirAll(filepath.Dir(globalPath), 0700))
	require(t, os.WriteFile(globalPath, global, 0600))
	project := map[string]any{
		"repository": c.Repository, "remote": nonEmpty(c.Remote, "origin"), "branch": nonEmpty(c.Branch, "main"),
		"run_dir": c.RunDir, "worktree_root": c.WorktreeRoot, "task_authority": "native", "workflow_protocol": executionmodel.Version,
		"claimant": "integration", "task_poll": "1s", "pr_poll": "30s", "heartbeat": "1s", "command_timeout": nonEmpty(c.CommandTimeout, "15s"), "shutdown_grace": "5s",
		"executable_types": []string{"task"}, "git": c.Git,
		"publication_policy": c.PublicationPolicy, "pr": c.PR,
	}
	if c.Git.Executable == "" {
		project["git"] = executionbackend.Command{Executable: "git"}
	}
	projectData, err := json.Marshal(project)
	require(t, err)
	projectPath := filepath.Join(t.TempDir(), "fixture-project.json")
	require(t, os.WriteFile(projectPath, projectData, 0600))
	loaded, err := steerlineapp.LoadQueryConfig(projectPath)
	require(t, err)
	return loaded
}

func nonEmpty(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func nativeProfileSettings(c steerlineapp.Config) codexworker.ProfileConfig {
	for _, profile := range c.ExecutionProfiles {
		for _, input := range profile.ImplementationConfigs {
			if input.ImplementationKey == codexworker.ProfileCodecKey {
				value, _ := codexworker.DecodeProfileConfig(input.Payload)
				return value
			}
		}
	}
	return codexworker.ProfileConfig{}
}

func fixtureBoundProfile(c steerlineapp.Config) execution.ExecutionProfile {
	for _, profile := range c.AvailableExecutionProfiles() {
		if profile.Host == c.Host && profile.Agent == c.Agent {
			return profile
		}
	}
	panic("fixture profile was not projected")
}

func executionPublicationFixture() executionbackend.PublicationPolicy {
	return executionbackend.PublicationPolicy{Publisher: executionbackend.PublisherWorker}
}

func hostPublicationProviderFixture() executionmodel.PRProviderConfig {
	return executionmodel.PRProviderConfig{Kind: "command", BaseURL: "https://github.com", Repository: "owner/repo", Query: executionbackend.Command{Executable: "git", Args: []string{"status"}}}
}

func setNativeImplementationProfiles(t *testing.T, c *steerlineapp.Config, worker codexworker.ProfileConfig, terminal *orcaworker.ProfileConfig, remotes ...*remote.ProfileConfig) {
	if c == nil {
		t.Fatal("app config is required")
	}
	*c = builtinFixtureConfig(t, *c, worker, terminal, remotes...)
}

func setNativeProfile(t *testing.T, c *steerlineapp.Config, worker codexworker.ProfileConfig) {
	var terminal *orcaworker.ProfileConfig
	var remoteSettings []*remote.ProfileConfig
	if c.Host != "local" {
		settings := orcaworker.ProfileConfig{Executable: "orca", Shell: "powershell.exe", WaitTimeout: "5s"}
		terminal = &settings
	}
	if c.Host == "orca_supervised" {
		settings := remote.ProfileConfig{ConnectionID: "home-connection", EnvironmentID: "env-home", RepositorySelector: "id:repo-test", CoordinatorTerminal: "term-coordinator", PollInterval: "2s", CapabilityReportVersion: 1}
		remoteSettings = append(remoteSettings, &settings)
	}
	setNativeImplementationProfiles(t, c, worker, terminal, remoteSettings...)
}

func nativeAdmissionFixture(t *testing.T) (steerlineapp.Config, string, *sqlite.SQLiteStore) {
	t.Helper()
	fixture := newLifecycleProject(t, "local")
	store, err := steerlineapp.OpenNativeTaskCommandStore(context.Background(), &fixture.config)
	require(t, err)
	t.Cleanup(func() { _ = store.Close() })
	return fixture.config, fixture.commit, store
}

func nativeConfigError(c steerlineapp.Config) error {
	if err := c.ValidateProfiles(); err != nil {
		return err
	}
	selected := c.ExecutionConfig()
	entry, err := c.ExecutionDirectory.Resolve(selected)
	if err != nil {
		return err
	}
	return entry.ValidateConfig(context.Background(), selected)
}

func historicalProfileValid(p executionmodel.FixedExecutionProfile) bool {
	d, err := steerlineapp.NewBuiltinDirectory(steerlineapp.NewProcessHost(time.Second))
	return err == nil && (steerlineapp.Config{ExecutionDirectory: d}).CheckFixedProfile(p) == nil
}

func historicalResourceError(_ execution.Config, r *executionmodel.PolicyResources, digest, _ string, e *executionmodel.ExecutionRecord) error {
	if r == nil || r.Version != 3 {
		return errors.New("historical resource wrapper unsupported")
	}
	_, err := codexworker.DecodePolicyResourceEnvelope(r.ImplementationData, codexworker.PolicyResourceIdentity{ProjectID: e.ProjectIdentity, ExecutionID: e.ExecutionID, AttemptID: e.CurrentAttemptID, Generation: e.Generation, ConfigDigest: digest})
	return err
}

func digestValue(value any) string { return string(mustJSON(value)) }

func nativeRequirementConfig(t *testing.T) steerlineapp.Config {
	t.Helper()
	return builtinFixtureConfig(t, steerlineapp.Config{Host: "local", Agent: "codex", WorkflowProtocol: executionmodel.Version, TaskAuthority: "native", PublicationPolicy: executionPublicationFixture(), PR: hostPublicationProviderFixture()}, codexworker.ProfileConfig{Executable: os.Args[0]}, nil)
}

func nativeTarget(c steerlineapp.Config) executionbackend.ExecutionTarget {
	entry, err := c.ExecutionDirectory.Resolve(c.ExecutionConfig())
	if err != nil {
		return executionbackend.ExecutionTarget{}
	}
	return entry.Target(c.ExecutionConfig())
}

func requireNativeFlowCapabilities(ctx context.Context, c steerlineapp.Config) error {
	if err := c.ValidateProfiles(); err != nil {
		return err
	}
	config := c.ExecutionConfig()
	entry, err := c.ExecutionDirectory.Resolve(config)
	if err != nil {
		return err
	}
	if err := entry.ValidateConfig(ctx, config); err != nil {
		return err
	}
	return entry.Preflight(ctx, config)
}
