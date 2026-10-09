package integration

import (
	"context"
	"strings"
	"testing"

	orcaworker "github.com/erranliu/SteerLine/internal/execution/adapters/orca"
	remote "github.com/erranliu/SteerLine/internal/execution/adapters/orcaremote"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func TestPS2FixedConfigurationKeepsMandatoryChecks(t *testing.T) {
	c, _, _ := nativeAdmissionFixture(t)
	codex := nativeProfileSettings(c)
	codex.Executable = "must-not-be-called"
	setNativeProfile(t, &c, codex)
	c.PublicationPolicy.Publisher = "host_service"
	if err := requireNativeFlowCapabilities(context.Background(), c); err == nil || !strings.Contains(err.Error(), "publication_policy") {
		t.Fatalf("empty requirements bypassed publisher check: %v", err)
	}
	c.PublicationPolicy = executionPublicationFixture()
	if err := requireNativeFlowCapabilities(context.Background(), c); err == nil {
		t.Fatal("empty requirements bypassed mandatory CLI preflight")
	}
	c.Host = "orca_supervised"
	c.PublicationPolicy.Publisher = executionbackend.PublisherWorker
	orca := &orcaworker.ProfileConfig{Executable: "missing-remote-orca-executable", Shell: "cmd.exe", WaitTimeout: "1m"}
	remoteSettings := &remote.ProfileConfig{ConnectionID: "peer", EnvironmentID: "remote", RepositorySelector: "id:repo", CoordinatorTerminal: "terminal", PollInterval: "1s", CapabilityReportVersion: 1}
	worker := nativeProfileSettings(c)
	worker.Model, worker.ReasoningEffort = "", ""
	setNativeImplementationProfiles(t, &c, worker, orca, remoteSettings)
	if err := requireNativeFlowCapabilities(context.Background(), c); err == nil || !strings.Contains(err.Error(), "Orca version probe") {
		t.Fatalf("empty requirements bypassed remote launch selection: %v", err)
	}
}
