package steerlineintegration

import (
	"context"
	"github.com/erranliu/SteerLine/internal/steerline"
	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	remote "github.com/erranliu/SteerLine/internal/steerline/orca_remote_worker"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
	"strings"
	"testing"
)

func TestExecutionRequirementsCannotInferOtherTargetEnvironment(t *testing.T) {
	for _, host := range []string{"orca", "orca_supervised"} {
		c := steerline.Config{Host: host, Agent: "codex"}
		orca := &orcaworker.ProfileConfig{Executable: "must-not-run", Shell: "cmd.exe", WaitTimeout: "1m"}
		if host == "orca_supervised" {
			remoteSettings := &remote.ProfileConfig{ConnectionID: "connection", EnvironmentID: "remote", RepositorySelector: "repo", CoordinatorTerminal: "terminal", PollInterval: "1s", CapabilityReportVersion: 1}
			setNativeImplementationProfiles(t, &c, codexworker.ProfileConfig{Executable: "codex"}, orca, remoteSettings)
		} else {
			setNativeImplementationProfiles(t, &c, codexworker.ProfileConfig{Executable: "codex"}, orca)
		}
		c.ExecutionRequirements.Tools = []executionapi.ToolRequirement{{ID: "go"}}
		e, err := c.EvaluateRequirements(context.Background())
		if err == nil || len(e.Tools) != 1 || e.Tools[0].Status != executionapi.RequirementUnknown || e.Tools[0].Target != nativeTarget(c) {
			t.Fatalf("center environment proved another target: %+v %v", e, err)
		}
	}
}

func TestEmptyExecutionRequirementsKeepMandatoryChecks(t *testing.T) {
	c := nativeRequirementConfig(t)
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
	c.PublicationPolicy.Publisher = executionapi.PublisherWorker
	orca := &orcaworker.ProfileConfig{Executable: "orca", Shell: "cmd.exe", WaitTimeout: "1m"}
	remoteSettings := &remote.ProfileConfig{ConnectionID: "peer", EnvironmentID: "remote", RepositorySelector: "id:repo", CoordinatorTerminal: "terminal", PollInterval: "1s", CapabilityReportVersion: 1}
	worker := nativeProfileSettings(c)
	worker.WindowsSandbox, worker.Model, worker.ReasoningEffort = "", "fixture-model", "high"
	setNativeImplementationProfiles(t, &c, worker, orca, remoteSettings)
	if err := requireNativeFlowCapabilities(context.Background(), c); err == nil || !strings.Contains(err.Error(), "enforcement evidence") {
		t.Fatalf("empty requirements bypassed remote policy: %v", err)
	}
}
