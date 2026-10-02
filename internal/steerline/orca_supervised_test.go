package steerline

import (
	"context"
	"errors"
	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	remote "github.com/erranliu/SteerLine/internal/steerline/orca_remote_worker"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
	"github.com/erranliu/SteerLine/internal/steerline/testprotocol"
	"os"
	"strings"
	"testing"
)

func supervisedFixtureConfig(t *testing.T) Config {
	t.Helper()
	f := newFixture(t)
	return supervisedConfigForFixture(t, f)
}

func supervisedConfigForFixture(t *testing.T, f *fixture) Config {
	t.Helper()
	remoteBytes, err := (Runner{Dir: f.c.Repository, Timeout: duration(f.c.CommandTimeout)}).Output(context.Background(), f.c.Git, "remote", "get-url", f.c.Remote)
	if err != nil {
		t.Fatal(err)
	}
	remoteURL := strings.TrimSpace(string(remoteBytes))
	t.Setenv("STEERLINE_FAKE_SUPERVISED_REMOTE_URL", remoteURL)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	c := f.c
	c.Host = "orca_supervised"
	c.Agent = "codex"
	setTestImplementationProfiles(t, &c, codexworker.ProfileConfig{Executable: "codex", Model: "gpt-6-luna", ReasoningEffort: "high"}, &orcaworker.ProfileConfig{Executable: exe, Args: []string{"--fake", "orca-supervised"}, Shell: "powershell", WaitTimeout: "5s"}, &remote.ProfileConfig{ConnectionID: "home-connection", EnvironmentID: "env-home", RepositorySelector: "id:repo-test", CoordinatorTerminal: "term-coordinator", ExpectedCodexVersion: "codex-cli 0.156.1", PollInterval: "2s", CapabilityReportVersion: 1})
	return c
}

func isBackendError(err error, want executionapi.BackendErrorCode) bool {
	var backendErr *executionapi.BackendError
	return errors.As(err, &backendErr) && backendErr.Code == want
}

func fakeSupervisedOrca(args []string) int { return testprotocol.FakeSupervisedOrca(args) }
