package steerlineintegration

import (
	"context"
	"strings"
	"testing"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/steerline"
	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	remote "github.com/erranliu/SteerLine/internal/steerline/orca_remote_worker"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
	"github.com/erranliu/SteerLine/internal/steerline/publication"
	taskapp "github.com/erranliu/SteerLine/internal/task"
)

func TestPublicationAndPermissionChecksRemainCombined(t *testing.T) {
	c := steerline.Config{WorkflowProtocol: executionmodel.Version, ExecutionPolicy: workspacePolicyFixture(), PublicationPolicy: executionbackend.PublicationPolicy{Publisher: "worker"}, PR: publication.Config{Kind: "gitea", BaseURL: "https://forge.invalid", Repository: "org/repo"}, Host: "orca_supervised", Agent: "codex"}
	setNativeImplementationProfiles(t, &c, codexworker.ProfileConfig{Executable: "codex", Model: "gpt-6-luna", ReasoningEffort: "high"}, &orcaworker.ProfileConfig{Executable: "orca", Shell: "cmd.exe", WaitTimeout: "1m"}, &remote.ProfileConfig{ConnectionID: "peer", EnvironmentID: "remote-env", RepositorySelector: "id:repo", CoordinatorTerminal: "terminal", PollInterval: "1s", CapabilityReportVersion: 1})
	entry, err := c.ExecutionDirectory.Resolve(c)
	if err != nil || entry.ValidatePublicationPolicy(c.PublicationPolicy, c.PR) != nil {
		t.Fatal("existing remote worker publication was removed")
	}
	if reason := fixtureBoundProfile(c).StaticUnavailableReason(); !strings.Contains(reason, "requested execution_policy") {
		t.Fatalf("remote permission evidence gate lost: %s", reason)
	}
	c.Host = "local"
	c.PublicationPolicy.Publisher = "worker"
	c.PR = hostPublicationProviderFixture()
	c.ExecutionPolicy.Network = "deny"
	settings := nativeProfileSettings(c)
	settings.Args = []string{"--search"}
	setNativeProfile(t, &c, settings)
	if err := nativeConfigError(c); err == nil || !strings.Contains(err.Error(), "live web search") {
		t.Fatalf("publication bypassed network deny: %v", err)
	}
}

func TestRemotePublicationClaimRequiresPermissionEvidence(t *testing.T) {
	c, _, store := nativeAdmissionFixture(t)
	c.Host, c.Agent = "orca_supervised", "codex"
	c.PublicationPolicy.Publisher = executionbackend.PublisherWorker
	c.PR = publication.Config{Kind: "gitea", BaseURL: "https://forge.invalid", Repository: "org/repo"}
	setNativeImplementationProfiles(t, &c, codexworker.ProfileConfig{Executable: "must-not-be-called", Model: "gpt-6-luna", ReasoningEffort: "high"}, &orcaworker.ProfileConfig{Executable: "orca", Shell: "cmd.exe", WaitTimeout: "1m"}, &remote.ProfileConfig{ConnectionID: "peer", EnvironmentID: "remote-env", RepositorySelector: "id:repo", CoordinatorTerminal: "terminal", ExpectedCodexVersion: "0.159.2", PollInterval: "1s", CapabilityReportVersion: 1})
	// Missing enforcement evidence must reject before Git, version, or remote
	// probes, even though this route's existing worker publication is supported.
	c.Git.Executable = "must-not-be-called"
	entry, err := c.ExecutionDirectory.Resolve(c)
	require(t, err)
	require(t, entry.ValidatePublicationPolicy(c.PublicationPolicy, c.PR))
	before, err := store.Query(context.Background(), c.Identity())
	require(t, err)
	_, err = (steerline.NewCommandService(store, c.Identity(), taskapp.Service{Reader: store}, taskapp.Service{Reader: store})).ClaimNativeTask(context.Background(), c, steerline.TaskClaimOptions{TaskID: "60", RequestID: newID(), ExpectedTaskVersion: 1, ExpectedAuthorityVersion: 1})
	if err == nil || !strings.Contains(err.Error(), "enforcement evidence") || strings.Contains(err.Error(), "must-not-be-called") {
		t.Fatalf("remote publication bypassed its permission admission gate: %v", err)
	}
	after, queryErr := store.Query(context.Background(), c.Identity())
	require(t, queryErr)
	if digestValue(before.WorkflowRuns) != digestValue(after.WorkflowRuns) || digestValue(before.Executions) != digestValue(after.Executions) || digestValue(before.Operations) != digestValue(after.Operations) {
		t.Fatal("rejected remote policy wrote claim or operation state")
	}
}
