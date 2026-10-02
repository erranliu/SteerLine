package steerlineapp

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline"
	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
	"github.com/erranliu/SteerLine/internal/steerline/publication"
)

type Config = steerline.Config
type Command = steerline.Command
type ExecutionProfile = steerline.ExecutionProfile

const WorkerFlowVersion = steerline.WorkerFlowVersion
const ProjectExecutionProfileID = steerline.ProjectExecutionProfileID
const PublisherExecutionProfile = steerline.PublisherExecutionProfile

func workspacePolicyFixture() executionapi.ExecutionPolicy {
	return executionapi.ExecutionPolicy{Filesystem: executionapi.FilesystemPolicy{ReadScope: "host", WriteScope: "workspace", GitMetadataWrite: true}, Network: "allow", Escalation: "deny"}
}
func executionPublicationFixture() executionapi.PublicationPolicy {
	return executionapi.PublicationPolicy{Publisher: executionapi.PublisherHostService}
}
func hostPublicationProviderFixture() publication.Config {
	return publication.Config{Kind: "command", BaseURL: "https://github.com", Repository: "owner/repo", Query: Command{Executable: "gh"}}
}
func testExecutionProfile(t *testing.T, id, name, host, agent string, worker codexworker.ProfileConfig, terminal *orcaworker.ProfileConfig) ExecutionProfile {
	t.Helper()
	if worker.WindowsSandbox == "" {
		if host == "local" {
			worker.WindowsSandbox = "elevated"
		} else {
			worker.WindowsSandbox = "unelevated"
		}
	}
	payload, err := codexworker.EncodeProfileConfig(worker)
	require(t, err)
	p := ExecutionProfile{ID: id, Name: name, Host: host, Agent: agent, ImplementationConfigs: []executionapi.ImplementationConfigInput{{ImplementationKey: codexworker.ProfileCodecKey, CodecVersion: 1, Payload: payload}}}
	if host != "local" {
		if terminal == nil {
			t.Fatal("terminal profile settings missing")
		}
		payload, err := orcaworker.EncodeProfileConfig(*terminal)
		require(t, err)
		p.ImplementationConfigs = append(p.ImplementationConfigs, executionapi.ImplementationConfigInput{ImplementationKey: orcaworker.ProfileCodecKey, CodecVersion: 1, Payload: payload})
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
func codexProfileSettings(c Config) codexworker.ProfileConfig {
	value, _ := codexSettings(c)
	return value
}
func codexProfileFromInputs(inputs []executionapi.ImplementationConfigInput) codexworker.ProfileConfig {
	for _, input := range inputs {
		if input.ImplementationKey == codexworker.ProfileCodecKey {
			value, _ := codexworker.DecodeProfileConfig(input.Payload)
			return value
		}
	}
	return codexworker.ProfileConfig{}
}
func testVerifyFixedExecutionProfile(p steerline.FixedExecutionProfile) bool {
	host := NewProcessHost(time.Second)
	directory, err := NewBuiltinDirectory(host)
	return err == nil && (Config{ExecutionDirectory: directory}).CheckFixedProfile(p) == nil
}
func nativeOrcaFixedAgentCommand(c Config) (string, error) { return managedTerminalCommand(c, "", nil) }
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
	repo := configTestRepository(t)
	require(t, os.WriteFile(filepath.Join(repo, "base.txt"), []byte("fixture"), 0600))
	fixtureGit(t, repo, "add", ".")
	fixtureGit(t, repo, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "-qm", "fixture")
	fixtureGit(t, repo, "remote", "add", "origin", "https://github.com/owner/repo.git")
	localGlobalExecutionFixture(t)
	c := Config{TaskPoll: "10s", PRPoll: "30s", Heartbeat: "5s", CommandTimeout: "30s", ShutdownGrace: "10s", ExecutableTypes: []string{"task"}, Git: Command{Executable: "git"}, Repository: repo, Remote: "origin", Branch: "main", TaskAuthority: "native", WorkflowProtocol: WorkerFlowVersion, ExecutionPolicy: workspacePolicyFixture(), PublicationPolicy: executionPublicationFixture(), PR: hostPublicationProviderFixture()}
	path := filepath.Join(t.TempDir(), "project.json")
	require(t, os.WriteFile(path, projectConfigJSON(t, c), 0600))
	c, err := LoadConfig(path)
	require(t, err)
	require(t, c.ResolveIdentity(context.Background()))
	return c, fixtureGit(t, repo, "rev-parse", "HEAD")
}
