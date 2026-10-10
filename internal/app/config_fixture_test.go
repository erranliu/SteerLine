package app

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/execution"
	codexworker "github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	orcaworker "github.com/erranliu/SteerLine/internal/execution/adapters/orca"
	remote "github.com/erranliu/SteerLine/internal/execution/adapters/orcaremote"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

type ExecutionProfile = execution.ExecutionProfile

const WorkerFlowVersion = executionmodel.Version

const ProjectExecutionProfileID = execution.ProjectExecutionProfileID

func executionPublicationFixture() executionbackend.PublicationPolicy {
	return executionbackend.PublicationPolicy{Publisher: executionbackend.PublisherWorker}
}

func hostPublicationProviderFixture() executionmodel.PRProviderConfig {
	return executionmodel.PRProviderConfig{Kind: "command", BaseURL: "https://github.com", Repository: "owner/repo", Query: executionbackend.Command{Executable: "gh"}}
}

func testExecutionProfile(t *testing.T, id, name, host, agent string, worker codexworker.ProfileConfig, terminal *orcaworker.ProfileConfig, remoteConfig ...*remote.ProfileConfig) ExecutionProfile {
	t.Helper()
	if host == "orca_supervised" {
		if terminal == nil {
			t.Fatal("supervised fixture configuration missing")
		}
		r := remote.ProfileConfig{Target: "local", PollInterval: "1s", CapabilityReportVersion: 1}
		if len(remoteConfig) > 0 && remoteConfig[0] != nil {
			r = *remoteConfig[0]
		}
		r.Executable = terminal.Executable
		r.Args = terminal.Args
		r.Shell = terminal.Shell
		r.WaitTimeout = terminal.WaitTimeout
		r.Model = worker.Model
		r.ReasoningEffort = worker.ReasoningEffort
		payload, err := remote.EncodeProfileConfig(r)
		require(t, err)
		return ExecutionProfile{ID: id, Name: name, Host: host, Agent: agent, ImplementationConfigs: []executionbackend.ImplementationConfigInput{{ImplementationKey: remote.ProfileCodecKey, CodecVersion: remote.ProfileCodecVersion, Payload: payload}}}
	}
	payload, err := codexworker.EncodeProfileConfig(worker)
	require(t, err)
	p := ExecutionProfile{ID: id, Name: name, Host: host, Agent: agent, ImplementationConfigs: []executionbackend.ImplementationConfigInput{{ImplementationKey: codexworker.ProfileCodecKey, CodecVersion: 2, Payload: payload}}}
	if host == "orca" {
		p.ImplementationConfigs = nil
	}
	if host != "local" {
		if terminal == nil {
			t.Fatal("terminal profile settings missing")
		}
		native := *terminal
		if host == "orca" {
			native.Model, native.ReasoningEffort = worker.Model, worker.ReasoningEffort
		}
		payload, err := orcaworker.EncodeProfileConfig(native)
		require(t, err)
		p.ImplementationConfigs = append(p.ImplementationConfigs, executionbackend.ImplementationConfigInput{ImplementationKey: orcaworker.ProfileCodecKey, CodecVersion: 1, Payload: payload})
	}
	return p
}

func writeGlobalExecutionFixture(t *testing.T, profiles []ExecutionProfile, defaultID string) {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"version": 2, "execution_profiles": profiles, "default_execution_profile": defaultID})
	require(t, err)
	require(t, os.MkdirAll(filepath.Dir(GlobalExecutionConfigPath()), 0700))
	require(t, os.WriteFile(GlobalExecutionConfigPath(), raw, 0600))
}

func localGlobalExecutionFixture(t *testing.T) {
	writeGlobalExecutionFixture(t, []ExecutionProfile{testExecutionProfile(t, "local", "Local", "local", "codex", codexworker.ProfileConfig{Executable: "codex"}, nil)}, "local")
}

func codexProfileSettings(c execution.Config, directories ...*execution.ExecutionDirectory) codexworker.ProfileConfig {
	if c.Host == "orca_supervised" {
		native, _ := remoteSettings(c, directories...)
		return codexworker.ProfileConfig{Model: native.Model, ReasoningEffort: native.ReasoningEffort}
	}
	value, _ := codexSettings(c, directories...)
	return value
}

func codexProfileFromInputs(inputs []executionbackend.ImplementationConfigInput) codexworker.ProfileConfig {
	for _, input := range inputs {
		if input.ImplementationKey == codexworker.ProfileCodecKey {
			value, _ := codexworker.DecodeProfileConfig(input.Payload)
			return value
		}
	}
	return codexworker.ProfileConfig{}
}

func testVerifyFixedExecutionProfile(p executionmodel.FixedExecutionProfile) bool {
	host := NewProcessHost(time.Second)
	directory, err := NewBuiltinDirectory(host)
	return err == nil && (Config{ExecutionDirectory: directory}).CheckFixedProfile(p) == nil
}

func mustJSON(value any) []byte {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return raw
}

func require(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func projectConfigJSON(t *testing.T, c Config) []byte {
	t.Helper()
	raw, err := json.Marshal(c)
	require(t, err)
	var fields map[string]json.RawMessage
	require(t, json.Unmarshal(raw, &fields))
	for _, key := range []string{"host", "agent", "execution_profiles", "default_execution_profile"} {
		delete(fields, key)
	}
	raw, err = json.Marshal(fields)
	require(t, err)
	return raw
}

func fixtureGit(t *testing.T, repo string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("isolated git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func nativeTaskFixture(t *testing.T) (Config, string) {
	t.Helper()
	t.Setenv("STEERLINE_HOME", t.TempDir())
	repo := configTestRepository(t)
	require(t, os.WriteFile(filepath.Join(repo, "base.txt"), []byte("fixture"), 0600))
	fixtureGit(t, repo, "add", ".")
	fixtureGit(t, repo, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "-qm", "fixture")
	fixtureGit(t, repo, "remote", "add", "origin", "https://github.com/owner/repo.git")
	localGlobalExecutionFixture(t)
	c := Config{Repository: repo, Remote: "origin", Branch: "main", Git: executionbackend.Command{Executable: "git"}, CommandTimeout: "5s", TaskPoll: "1s", PRPoll: "1s", Heartbeat: "1s", ShutdownGrace: "1s", TaskAuthority: "native", WorkflowProtocol: WorkerFlowVersion, PublicationPolicy: executionPublicationFixture(), PR: hostPublicationProviderFixture()}
	c.ExecutableTypes = []string{"task"}
	path := filepath.Join(t.TempDir(), "project.json")
	require(t, os.WriteFile(path, projectConfigJSON(t, c), 0600))
	c, err := LoadConfig(path)
	require(t, err)
	require(t, c.ResolveIdentity(context.Background()))
	return c, fixtureGit(t, repo, "rev-parse", "HEAD")
}

func bindUnitDirectory(t *testing.T, c Config) Config {
	t.Helper()
	if c.PublicationPolicy.Publisher == "" {
		c.PublicationPolicy = executionPublicationFixture()
	}
	if c.PR.Kind == "" {
		c.PR = hostPublicationProviderFixture()
	}
	host := NewProcessHost(time.Second)
	directory, err := NewBuiltinDirectory(host)
	require(t, err)
	c.ProcessHost = host
	c.ExecutionDirectory = directory
	c.bindApplicationFiles("fixture-project.json", "fixture-global.json", "fixture-accepted-v1", true)
	return c
}

func boundProfile(c Config, p ExecutionProfile) ExecutionProfile {
	c.ExecutionProfiles = []ExecutionProfile{p}
	for _, bound := range c.AvailableExecutionProfiles() {
		if bound.ID == p.ID {
			return bound
		}
	}
	panic("bound profile absent")
}

func digestValue(value any) string { return shortDigest(value) }

func unitProjectProfile(c Config) ExecutionProfile {
	executionConfig := c.executionConfig()
	inputs := make([]executionbackend.ImplementationConfigInput, 0, len(executionConfig.ImplementationData))
	for _, e := range executionConfig.ImplementationData {
		inputs = append(inputs, executionbackend.ImplementationConfigInput{ImplementationKey: e.ImplementationKey, CodecVersion: e.CodecVersion, Payload: e.Payload})
	}
	return boundProfile(c, ExecutionProfile{ID: ProjectExecutionProfileID, Name: "Project default", Host: c.Host, Agent: c.Agent, ImplementationConfigs: inputs})
}

func setTestImplementationProfiles(t *testing.T, c *Config, worker codexworker.ProfileConfig, terminal *orcaworker.ProfileConfig, remoteConfig ...*remote.ProfileConfig) {
	t.Helper()
	if c.ExecutionDirectory == nil {
		*c = bindUnitDirectory(t, *c)
	}
	p := testExecutionProfile(t, ProjectExecutionProfileID, "Project default", c.Host, c.Agent, worker, terminal, remoteConfig...)
	*c = c.ApplyExecutionProfile(p)
}

func setTestCodexProfile(t *testing.T, c *Config, worker codexworker.ProfileConfig) {
	t.Helper()
	var terminal *orcaworker.ProfileConfig
	if c.Host != "local" {
		settings, _ := remoteSettings(c.executionConfig(), c.ExecutionDirectory)
		terminal = &orcaworker.ProfileConfig{Executable: settings.Executable, Args: settings.Args, Shell: settings.Shell, WaitTimeout: settings.WaitTimeout}
	}
	setTestImplementationProfiles(t, c, worker, terminal)
}
