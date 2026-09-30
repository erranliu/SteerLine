package steerline

import (
	"context"
	"encoding/json"
	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/publication"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectPublicationPolicyRequiredBeforeRepositoryProbe(t *testing.T) {
	for _, policy := range []any{nil, map[string]string{}, map[string]string{"publisher": "automatic"}} {
		path := filepath.Join(t.TempDir(), "project.json")
		raw, err := json.Marshal(map[string]any{"publication_policy": policy, "execution_policy": workspacePolicyFixture(), "repository": "must-not-be-probed", "branch": "main", "task_authority": "native", "workflow_protocol": WorkerFlowVersion, "git": map[string]string{"executable": "must-not-be-called"}})
		require(t, err)
		require(t, os.WriteFile(path, raw, 0600))
		_, err = LoadConfig(path)
		if err == nil || !strings.Contains(err.Error(), "publication_policy.publisher") {
			t.Fatalf("missing/invalid policy reached repository probe: %v", err)
		}
	}
}

func executionPublicationFixture() executionapi.PublicationPolicy {
	return executionapi.PublicationPolicy{Publisher: "host_service"}
}
func hostPublicationProviderFixture() publication.Config {
	return publication.Config{Kind: "command", BaseURL: "https://github.com", Repository: "owner/repo", Query: Command{Executable: os.Args[0], Args: []string{"api", "repos/{repository}/pulls/{number}"}}, Close: Command{Executable: os.Args[0], Args: []string{"pr", "close", "{pr_url}"}}}
}

// Historical evidence is verified in its original v2 format only here. It is
// deliberately rejected by the production v3 validator, never migrated.
func verifyHistoricalPolicyProfile(t *testing.T, p FixedExecutionProfile) bool {
	t.Helper()
	rebuilt := ExecutionProfile{ExecutionPolicy: p.ExecutionPolicy, AcceptedConfigVersion: p.AcceptedConfigVersion, PolicySource: p.PolicySource, ID: p.ID, Name: p.Name, Host: p.Host, Agent: p.Agent, Model: p.Model, ReasoningEffort: p.ReasoningEffort, Sandbox: p.Sandbox, WindowsSandbox: p.WindowsSandbox, CodexProfile: p.CodexProfile, Codex: p.Codex, Orca: p.Orca}
	digest := digestValue(struct {
		ExecutionProfile
		Version               int
		Policy                executionapi.ExecutionPolicy
		AcceptedConfigVersion string
		PolicySource          string
		Sandbox               string
		WindowsSandbox        string
		Contract              string
	}{rebuilt, 2, p.ExecutionPolicy, p.AcceptedConfigVersion, p.PolicySource, p.Sandbox, p.WindowsSandbox, codexworker.PolicyContract})
	return p.Version == 2 && p.Digest == digest && p.PolicyDigest == digestValue(p.ExecutionPolicy) && !verifyFixedExecutionProfile(p)
}

func TestPublicationPolicyFrozenConfigurationAndCredentials(t *testing.T) {
	f := newLocalWorkerPublicationFixture(t)
	defer f.s.Close()
	workflow, _ := f.s.workflowRecords(f.run)
	original := *workflow.ExecutionProfile
	current := f.c
	current.PublicationPolicy.Publisher = "worker"
	current.PR.BaseURL, current.PR.Repository, current.PR.TokenEnv = "https://other.invalid", "other/repo", "OTHER_TOKEN"
	current.PR.Query = Command{Executable: "must-not-be-called", Args: []string{"changed"}}
	current.PR.Close = current.PR.Query
	current.Remote, current.Branch, current.Git = "other", "other", Command{Executable: "must-not-be-called"}
	f.s.Config, f.s.baseConfig = current, current
	fixed, err := f.s.executionConfigForRun(f.run)
	require(t, err)
	if fixed.PublicationPolicy != original.PublicationPolicy || digestValue(fixed.publicationSettings()) != digestValue(original.Publication) {
		t.Fatal("current config redirected frozen publication")
	}
	data, err := json.Marshal(original)
	require(t, err)
	t.Setenv("GH_TOKEN", "secret-host-token")
	t.Setenv("CUSTOM_HOST_TOKEN", "secret-custom-token")
	fixed.PR.TokenEnv = "CUSTOM_HOST_TOKEN"
	worker := codexBackend(fixed, f.run).Config.Environment([]string{"GH_TOKEN=secret-host-token", "CUSTOM_HOST_TOKEN=secret-custom-token", "PATH=ordinary"})
	if strings.Contains(strings.Join(worker, "\n"), "secret-") || strings.Contains(string(data), "secret-") {
		t.Fatal("host credentials leaked into worker/snapshot")
	}
	if err := fixed.bindPublicationTarget(context.Background()); err != nil {
		t.Fatal(err)
	}
	copy := original
	copy.Publication.Provider.TokenEnv = "edited"
	if verifyFixedExecutionProfile(copy) {
		t.Fatal("changed auth reference accepted")
	}
	copy = original
	copy.Version = 2
	if verifyFixedExecutionProfile(copy) {
		t.Fatal("old snapshot accepted")
	}
}

func TestPublicationOriginalProviderAfterRestartAndRemoteDrift(t *testing.T) {
	f := newLocalWorkerPublicationFixture(t)
	workflow, execution := f.s.workflowRecords(f.run)
	info, err := f.s.publishLocalWorkerRun(f.ctx, f.run, workflow, execution, f.turnID)
	require(t, err)
	original := *f.s.domain.WorkflowRuns[0].ExecutionProfile
	require(t, f.s.Close())
	changed := f.c
	changed.PR.Query = Command{Executable: "must-not-be-called", Args: []string{"changed"}}
	changed.PR.Close = changed.PR.Query
	changed.PR.TokenEnv = "CHANGED_AUTH_REF"
	s, err := NewService(changed)
	require(t, err)
	defer s.Close()
	run := s.find(f.run.TaskID)
	fixed, err := s.executionConfigForRun(run)
	require(t, err)
	if digestValue(fixed.publicationSettings()) != digestValue(original.Publication) {
		t.Fatal("restart redirected provider/reference")
	}
	queried, err := publicationClient(fixed).Query(f.ctx, info.URL)
	require(t, err)
	if queried.URL != info.URL {
		t.Fatal("original query lost exact PR")
	}
	closed, err := publicationClient(fixed).Close(f.ctx, info.URL)
	require(t, err)
	if closed.State != "closed_unmerged" {
		t.Fatal("original close did not verify closure")
	}
	// Use the real Git reader to prove a changed transport is rejected before
	// any publish intent/action; no external push is made by this check.
	reader := fixed
	reader.Git = Command{Executable: "git"}
	f.git(reader.Repository, "remote", "set-url", reader.Remote, original.Publication.RemoteURL)
	require(t, reader.bindPublicationTarget(f.ctx))
	f.git(reader.Repository, "remote", "set-url", reader.Remote, "https://changed.invalid/owner/repo.git")
	if err := reader.bindPublicationTarget(f.ctx); err == nil {
		t.Fatal("different physical remote accepted")
	}
	if count, err := os.ReadFile(os.Getenv("STEERLINE_TEST_HOST_GH_FILE") + ".creates"); err != nil || string(count) != "x" {
		t.Fatalf("query/close/recovery repeated create: %q %v", count, err)
	}
}

func TestPublicationAndPermissionChecksRemainCombined(t *testing.T) {
	c := Config{WorkflowProtocol: WorkerFlowVersion, ExecutionPolicy: workspacePolicyFixture(), PublicationPolicy: executionapi.PublicationPolicy{Publisher: "worker"}, PR: publication.Config{Kind: "gitea", BaseURL: "https://forge.invalid", Repository: "org/repo"}, Host: "orca_supervised", Agent: "codex", Orca: OrcaConfig{Supervised: &SupervisedOrcaConfig{EnvironmentID: "remote-env", ConnectionID: "peer", RepositorySelector: "id:repo"}}}
	if err := publication.CheckPolicy(c.PublicationPolicy, c.Host, c.Agent, c.PR); err != nil {
		t.Fatal("existing remote worker publication was removed")
	}
	if reason := c.projectExecutionProfile().StaticUnavailableReason(); !strings.Contains(reason, "requested execution_policy") {
		t.Fatalf("remote permission evidence gate lost: %s", reason)
	}
	c.Host = "local"
	c.PublicationPolicy.Publisher = "host_service"
	c.PR = hostPublicationProviderFixture()
	c.ExecutionPolicy.Network = "deny"
	c.Codex.Args = []string{"--search"}
	if err := validateWorkflowConfig(&c, nil); err == nil || !strings.Contains(err.Error(), "live web search") {
		t.Fatalf("publication bypassed network deny: %v", err)
	}
}

func TestRemotePublicationClaimRequiresPermissionEvidence(t *testing.T) {
	c, _, store := openDAGFixture(t)
	c.Host, c.Agent = "orca_supervised", "codex"
	c.PublicationPolicy.Publisher = executionapi.PublisherWorker
	c.PR = publication.Config{Kind: "gitea", BaseURL: "https://forge.invalid", Repository: "org/repo"}
	c.Orca.Supervised = &SupervisedOrcaConfig{ConnectionID: "peer", EnvironmentID: "remote-env", RepositorySelector: "id:repo", ExpectedCodexVersion: "0.159.2"}
	// Missing enforcement evidence must reject before Git, version, or remote
	// probes, even though this route's existing worker publication is supported.
	c.Git.Executable, c.Codex.Executable = "must-not-be-called", "must-not-be-called"
	require(t, publication.CheckPolicy(c.PublicationPolicy, c.Host, c.Agent, c.PR))
	before, err := store.Query(context.Background(), c.Identity())
	require(t, err)
	_, err = (CommandService{Store: store, Project: c.Identity()}).ClaimNativeTask(context.Background(), c, TaskClaimOptions{TaskID: "60", RequestID: newID(), ExpectedTaskVersion: 1, ExpectedAuthorityVersion: 1})
	if err == nil || !strings.Contains(err.Error(), "enforcement evidence") || strings.Contains(err.Error(), "must-not-be-called") {
		t.Fatalf("remote publication bypassed its permission admission gate: %v", err)
	}
	after, queryErr := store.Query(context.Background(), c.Identity())
	require(t, queryErr)
	if digestValue(before.WorkflowRuns) != digestValue(after.WorkflowRuns) || digestValue(before.Executions) != digestValue(after.Executions) || digestValue(before.Operations) != digestValue(after.Operations) {
		t.Fatal("rejected remote policy wrote claim or operation state")
	}
}
