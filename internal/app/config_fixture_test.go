package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnerstatepath"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const WorkerFlowVersion = executionmodel.Version

func executionPublicationFixture() executionbackend.PublicationPolicy {
	return executionbackend.PublicationPolicy{Publisher: executionbackend.PublisherWorker}
}

func hostPublicationProviderFixture() executionmodel.PRProviderConfig {
	return executionmodel.PRProviderConfig{Kind: "command", BaseURL: "https://github.com", Repository: "owner/repo", Query: executionbackend.Command{Executable: "gh"}}
}

type terminalFixtureProfile struct {
	Executable, Shell, WaitTimeout, Model, ReasoningEffort string
	Args                                                   []string
	KeepTerminal, Activate                                 bool
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
	home := HomeDir()
	t.Cleanup(func() { require(t, runnerstatepath.RemoveStopped(home)) })
	return c, fixtureGit(t, repo, "rev-parse", "HEAD")
}

func configTestRepository(t *testing.T) string { return projectRepo(t) }
func localGlobalExecutionFixture(t *testing.T) {
	t.Helper()
	exe, err := os.Executable()
	require(t, err)
	raw, err := os.ReadFile(exe)
	require(t, err)
	sum := sha256.Sum256(raw)
	config := RunnerExecutionConfig{Version: 3, Runner: runnerclient.Selection{Executable: exe, Version: "unit-fixture", SHA256: hex.EncodeToString(sum[:])}, DefaultProfile: "fixture", Profiles: []RunnerProfile{{ID: "fixture", Name: "Fixture", Execution: runnerclient.ExecutionInput{Adapter: "codex-app-server", Target: runnerclient.ExecutionTarget{Kind: "local"}, Settings: json.RawMessage(`{"executable":"native-fixture"}`)}, Workflow: runnerclient.Workflow{ID: "development", Version: 1}}}}
	require(t, os.MkdirAll(HomeDir(), 0700))
	body, err := json.Marshal(config)
	require(t, err)
	require(t, os.WriteFile(GlobalExecutionConfigPath(), body, 0600))
}
