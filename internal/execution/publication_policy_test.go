package execution

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

func executionPublicationFixture() executionbackend.PublicationPolicy {
	return executionbackend.PublicationPolicy{Publisher: "worker"}
}

func hostPublicationProviderFixture() executionmodel.PRProviderConfig {
	return executionmodel.PRProviderConfig{Kind: "command", BaseURL: "https://github.com", Repository: "owner/repo", Query: executionbackend.Command{Executable: os.Args[0], Args: []string{"api", "repos/{repository}/pulls/{number}"}}, Close: executionbackend.Command{Executable: os.Args[0], Args: []string{"pr", "close", "{pr_url}"}}}
}

func TestPublicationPolicyFrozenConfigurationAndCredentials(t *testing.T) {
	f := newLocalWorkerPublicationFixture(t)
	defer f.s.Close()
	workflow, _ := f.s.workflowRecords(f.run)
	original := *workflow.ExecutionProfile
	current := f.c
	current.PublicationPolicy.Publisher = "host_service"
	current.PR.BaseURL, current.PR.Repository, current.PR.TokenEnv = "https://other.invalid", "other/repo", "OTHER_TOKEN"
	current.PR.Query = executionbackend.Command{Executable: "must-not-be-called", Args: []string{"changed"}}
	current.PR.Close = current.PR.Query
	current.Remote, current.Branch, current.Git = "other", "other", executionbackend.Command{Executable: "must-not-be-called"}
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
	worker := workerProcessEnvironment(fixed, f.run, []string{"GH_TOKEN=secret-host-token", "CUSTOM_HOST_TOKEN=secret-custom-token", "PATH=ordinary"})
	if strings.Contains(strings.Join(worker, "\n"), "secret-") || strings.Contains(string(data), "secret-") {
		t.Fatal("host credentials leaked into worker/snapshot")
	}
	if err := fixed.BindPublicationTarget(context.Background(), f.s.runtime.ProcessHost); err != nil {
		t.Fatal(err)
	}
	copy := original
	copy.Publication.Provider.TokenEnv = "edited"
	if neutralVerifyFixedExecutionProfile(copy) {
		t.Fatal("changed auth reference accepted")
	}
	for _, version := range []int{5, 2} {
		copy = original
		copy.Version = version
		copy.Digest = fixedExecutionProfileDigest(copy)
		if neutralVerifyFixedExecutionProfile(copy) {
			t.Fatalf("old snapshot version %d accepted", version)
		}
	}
}

func TestRetiredPublicationMetadataRejected(t *testing.T) {
	var settings executionmodel.PublicationSettings
	if err := decode([]byte(`{"create":{"executable":"retired-publisher"}}`), &settings); err == nil {
		t.Fatal("retired create command accepted by the current metadata schema")
	}
}

func TestPublicationOriginalProviderAfterRestartAndRemoteDrift(t *testing.T) {
	f := newLocalWorkerPublicationFixture(t)
	info := executionmodel.PRInfo{URL: "https://github.com/owner/repo/pull/1", Repository: "owner/repo", HeadRepository: "owner/repo", HeadBranch: f.run.WorkspaceIntent.Branch, HeadRevision: f.base, BaseBranch: "main", State: "open"}
	// The worker supplied this external PR; the center can only query/close it.
	require(t, os.WriteFile(os.Getenv("STEERLINE_TEST_HOST_GH_FILE"), mustJSON(info), 0600))
	original := *f.s.domain.WorkflowRuns[0].ExecutionProfile
	require(t, f.s.Close())
	changed := f.c
	changed.PR.Query = executionbackend.Command{Executable: "must-not-be-called", Args: []string{"changed"}}
	changed.PR.Close = changed.PR.Query
	changed.PR.TokenEnv = "CHANGED_AUTH_REF"
	s, err := newTestService(changed)
	require(t, err)
	defer s.Close()
	run := s.find(f.run.TaskID)
	fixed, err := s.executionConfigForRun(run)
	require(t, err)
	if digestValue(fixed.publicationSettings()) != digestValue(original.Publication) {
		t.Fatal("restart redirected provider/reference")
	}
	queried, err := publicationClient(fixed, s.runtime.ProcessHost).Query(f.ctx, info.URL)
	require(t, err)
	if queried.URL != info.URL {
		t.Fatal("original query lost exact PR")
	}
	closed, err := publicationClient(fixed, s.runtime.ProcessHost).Close(f.ctx, info.URL)
	require(t, err)
	if closed.State != "closed_unmerged" {
		t.Fatal("original close did not verify closure")
	}
	// Use the real Git reader to prove a changed transport is rejected before
	// any publish intent/action; no external push is made by this check.
	reader := fixed
	reader.Git = executionbackend.Command{Executable: "git"}
	f.git(reader.Repository, "remote", "set-url", reader.Remote, original.Publication.RemoteURL)
	require(t, reader.BindPublicationTarget(f.ctx, s.runtime.ProcessHost))
	f.git(reader.Repository, "remote", "set-url", reader.Remote, "https://changed.invalid/owner/repo.git")
	if err := reader.BindPublicationTarget(f.ctx, s.runtime.ProcessHost); err == nil {
		t.Fatal("different physical remote accepted")
	}
	if _, err := os.Stat(os.Getenv("STEERLINE_TEST_HOST_GH_FILE") + ".creates"); !os.IsNotExist(err) {
		t.Fatalf("center query/close/recovery attempted publication: %v", err)
	}
}
