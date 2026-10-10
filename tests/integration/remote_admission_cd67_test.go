package integration

import (
	"context"
	"os"
	"strings"
	"testing"

	steerlineapp "github.com/erranliu/SteerLine/internal/app"
	"github.com/erranliu/SteerLine/internal/execution"
	codexworker "github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	orcaworker "github.com/erranliu/SteerLine/internal/execution/adapters/orca"
	remote "github.com/erranliu/SteerLine/internal/execution/adapters/orcaremote"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/execution/publication"
	productquery "github.com/erranliu/SteerLine/internal/product/query"
)

func TestPublicationAndPermissionChecksRemainCombined(t *testing.T) {
	c := steerlineapp.Config{TaskAuthority: "native", WorkflowProtocol: executionmodel.Version, PublicationPolicy: executionbackend.PublicationPolicy{Publisher: "worker"}, PR: executionmodel.PRProviderConfig{Kind: "gitea", BaseURL: "https://forge.invalid", Repository: "org/repo"}, Host: "orca_supervised", Agent: "codex"}
	setNativeImplementationProfiles(t, &c, codexworker.ProfileConfig{Executable: "codex", Model: "gpt-6-luna", ReasoningEffort: "high"}, &orcaworker.ProfileConfig{Executable: "missing-remote-orca-executable", Shell: "cmd.exe", WaitTimeout: "1m"}, &remote.ProfileConfig{ConnectionID: "peer", EnvironmentID: "remote-env", RepositorySelector: "id:repo", CoordinatorTerminal: "terminal", PollInterval: "1s", CapabilityReportVersion: 1})
	entry, err := c.ExecutionDirectory.Resolve(c.ExecutionConfig())
	if err != nil || entry.ValidatePublicationPolicy(c.PublicationPolicy, publication.Config(c.PR)) != nil {
		t.Fatal("existing remote worker publication was removed")
	}
	if reason := fixtureBoundProfile(c).StaticUnavailableReason(); reason != "" {
		t.Fatalf("valid remote profile unavailable: %s", reason)
	}
	c.Host = "local"
	c.PublicationPolicy.Publisher = "worker"
	c.PR = hostPublicationProviderFixture()
	setNativeProfile(t, &c, codexworker.ProfileConfig{Executable: "codex", Model: "gpt-6-luna", ReasoningEffort: "high"})
	if err := nativeConfigError(c); err != nil {
		t.Fatalf("valid fixed local configuration rejected: %v", err)
	}
}

func TestSupervisedDevelopmentFlowRejectsUnreachableNativeBeforeClaim(t *testing.T) {
	fixture := newLifecycleProjectBase(t, "local", false)
	c := fixture.config
	c.Host, c.Agent = "orca_supervised", "codex"
	c.PublicationPolicy.Publisher = executionbackend.PublisherWorker
	c.PR = executionmodel.PRProviderConfig{Kind: "gitea", BaseURL: "https://forge.invalid", Repository: "org/repo"}
	setNativeImplementationProfiles(t, &c, codexworker.ProfileConfig{Executable: "unused", Model: "", ReasoningEffort: ""}, &orcaworker.ProfileConfig{Executable: "missing-remote-orca-executable", Shell: "cmd.exe", WaitTimeout: "1m"}, &remote.ProfileConfig{ConnectionID: "peer", EnvironmentID: "remote-env", RepositorySelector: "id:repo", CoordinatorTerminal: "terminal", PollInterval: "1s", CapabilityReportVersion: 1})
	t.Setenv("STEERLINE_TEST_REMOTE_URL", "https://forge.invalid/org/repo.git")
	project, err := steerlineapp.OpenNativeTaskProject(context.Background(), &c)
	require(t, err)
	require(t, project.Close())
	store, err := steerlineapp.OpenNativeTaskCommandStore(context.Background(), &c)
	require(t, err)
	defer store.Close()
	// The implementation now supports declared finals. Native preflight still
	// rejects an unreachable executable before reserving a claim or operation.
	entry, err := c.ExecutionDirectory.Resolve(c.ExecutionConfig())
	require(t, err)
	require(t, entry.ValidatePublicationPolicy(c.PublicationPolicy, publication.Config(c.PR)))
	before, err := store.Query(context.Background(), c.Identity())
	require(t, err)
	_, err = (appExecutionCommands(t, store, c)).ClaimNativeTask(context.Background(), c.ExecutionConfig(), execution.TaskClaimOptions{TaskID: "60", RequestID: newID(), ExpectedTaskVersion: 1, ExpectedAuthorityVersion: 1})
	if err == nil || !strings.Contains(err.Error(), "center Orca version probe") {
		t.Fatalf("supervised development flow bypassed native preflight: %v", err)
	}
	after, queryErr := store.Query(context.Background(), c.Identity())
	require(t, queryErr)
	if digestValue(before.WorkflowRuns) != digestValue(after.WorkflowRuns) || digestValue(before.Executions) != digestValue(after.Executions) || digestValue(before.Operations) != digestValue(after.Operations) {
		t.Fatal("rejected remote policy wrote claim or operation state")
	}
}

func TestSupervisedDevelopmentFlowClaimsWithDeclaredFinalCapability(t *testing.T) {
	fixture := newLifecycleProjectBase(t, "local", false)
	c := fixture.config
	c.Host, c.Agent = "orca_supervised", "codex"
	c.PublicationPolicy.Publisher = executionbackend.PublisherWorker
	c.PR = executionmodel.PRProviderConfig{Kind: "gitea", BaseURL: "https://forge.invalid", Repository: "org/repo"}
	setNativeImplementationProfiles(t, &c, codexworker.ProfileConfig{Executable: "unused"}, &orcaworker.ProfileConfig{Executable: os.Args[0], Args: []string{"--fake-supervised"}, Shell: "cmd.exe", WaitTimeout: "10s"}, &remote.ProfileConfig{ConnectionID: "home-connection", EnvironmentID: "env-home", RepositorySelector: "id:repo-test", CoordinatorTerminal: "term-coordinator", PollInterval: "1s", CapabilityReportVersion: 1})
	t.Setenv("STEERLINE_FAKE_SUPERVISED_REMOTE_URL", "https://forge.invalid/org/repo.git")
	t.Setenv("STEERLINE_TEST_REMOTE_URL", "https://forge.invalid/org/repo.git")
	project, err := steerlineapp.OpenNativeTaskProject(context.Background(), &c)
	require(t, err)
	require(t, project.Close())
	fixture.config, fixture.remoteURL = c, "https://forge.invalid/org/repo.git"
	createIntegrationTask(t, context.Background(), fixture, 60)
	store, err := steerlineapp.OpenNativeTaskCommandStore(context.Background(), &c)
	require(t, err)
	defer store.Close()
	before, err := store.Query(context.Background(), c.Identity())
	require(t, err)
	page, err := productquery.NewService(store, c.Identity()).ShowNativeTask(context.Background(), "60")
	require(t, err)
	profile := c.AvailableExecutionProfiles()[0].Freeze(c.ExecutionDirectory, c.Identity())
	_, err = appExecutionCommands(t, store, c).ClaimNativeTask(context.Background(), c.ExecutionConfig(), execution.TaskClaimOptions{TaskID: "60", RequestID: newID(), ExpectedTaskVersion: page.Tasks[0].Version, ExpectedAuthorityVersion: page.AuthorityVersion, ExecutionProfile: &profile, Explicit: true, RequestedBy: "operator"})
	require(t, err)
	after, err := store.Query(context.Background(), c.Identity())
	require(t, err)
	if len(after.WorkflowRuns) != len(before.WorkflowRuns)+1 || len(after.Executions) != len(before.Executions)+1 {
		t.Fatal("supported supervised claim did not persist one workflow/execution")
	}
}
