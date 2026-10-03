package steerline

import (
	"bytes"
	"os"
	"testing"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/sqlite"
)

func TestWorkerFlowContextKeepsFixedRunAndCopiesConfig(t *testing.T) {
	run := &Run{RunID: "run", ExecutionID: "execution", AttemptID: "attempt"}
	config := workerContextConfigFixture()
	settings := neutralProfileSettings(config)
	settings.Args = []string{"--no-alt-screen"}
	setNeutralProfile(t, &config, settings)
	config.workerEnv = map[string]string{"A": "original"}
	fixedProfile := config.projectExecutionProfile().Freeze(configurationProjectIdentity(config))
	run.Host, run.Agent, run.Backend, run.Protocol, run.Target = "local", "fixture", "local", string(neutralProtocol), targetFor(config)
	service := Service{
		Config: config,
		domain: DomainSnapshot{WorkflowRuns: []WorkflowRunRecord{{ProjectIdentity: configurationProjectIdentity(config), WorkflowRunID: "run", WorkflowVersion: WorkerFlowVersion, Repository: config.Repository, Branch: config.Branch, ConfigDigest: fixedProfile.AcceptedConfigVersion, ExecutionProfile: &fixedProfile}},
			Executions: []ExecutionRecord{{ProjectIdentity: configurationProjectIdentity(config), ExecutionID: "execution", WorkflowRunID: "run", CurrentAttemptID: run.AttemptID, ConfigurationDigest: fixedProfile.Digest}},
			Attempts:   []StepAttemptRecord{{AttemptID: "attempt", ExecutionID: "execution"}}},
	}
	fixed, err := service.workerFlowContext(run)
	if err != nil || fixed.run != run || fixed.execution.ExecutionID != "execution" {
		t.Fatalf("fixed worker context lost its identity: %+v %v", fixed, err)
	}
	fixed.config.ImplementationData[0].Payload[0] = '['
	fixed.config.workerEnv["A"] = "changed"
	if neutralProfileSettings(service.Config).Args[0] != "--no-alt-screen" || service.Config.workerEnv["A"] != "original" {
		t.Fatal("fixed worker context shares mutable configuration with the project")
	}
	wrong := *run
	wrong.AttemptID = "other"
	if _, err := service.workerFlowContext(&wrong); err == nil {
		t.Fatal("worker context accepted an attempt from another execution")
	}
}

func TestWorkerFlowContextsDoNotExchangeExecutionProfiles(t *testing.T) {
	base := workerContextConfigFixture()
	routes := []ExecutionProfile{
		neutralProfile(t, "first", "First", neutralSettings{Executable: "fixture-a", Model: "model-a", Args: []string{"--search"}}),
		neutralProfile(t, "second", "Second", neutralSettings{Executable: "fixture-b", Model: "model-b", Args: []string{"--no-alt-screen"}}),
	}
	service := Service{Config: base, baseConfig: base}
	for _, route := range routes {
		id := route.ID
		fixed := base.bindProjectPolicy(route).Freeze(configurationProjectIdentity(base))
		selected := base.ApplyFixedExecutionProfile(fixed)
		run := &Run{RunID: id, ExecutionID: id + "-execution", AttemptID: id + "-attempt", Host: "local", Agent: "fixture", Backend: string(executionapi.BackendLocal), Protocol: string(neutralProtocol), Target: targetFor(selected), ExecutionStatus: "pending"}
		service.State.Runs = append(service.State.Runs, run)
		service.domain.WorkflowRuns = append(service.domain.WorkflowRuns, WorkflowRunRecord{ProjectIdentity: configurationProjectIdentity(base), WorkflowRunID: id, WorkflowVersion: WorkerFlowVersion, Repository: base.Repository, Branch: base.Branch, ConfigDigest: fixed.AcceptedConfigVersion, ExecutionProfile: &fixed})
		service.domain.Executions = append(service.domain.Executions, ExecutionRecord{ProjectIdentity: configurationProjectIdentity(base), ExecutionID: run.ExecutionID, WorkflowRunID: id, CurrentAttemptID: run.AttemptID, ConfigurationDigest: fixed.Digest})
		service.domain.Attempts = append(service.domain.Attempts, StepAttemptRecord{AttemptID: run.AttemptID, ExecutionID: run.ExecutionID})
	}
	first, err := service.workerFlowContext(service.State.Runs[0])
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.workerFlowContext(service.State.Runs[1])
	if err != nil {
		t.Fatal(err)
	}
	basePayload := append([]byte(nil), base.ImplementationData[0].Payload...)
	first.config.ImplementationData[0].Payload[0] = '['
	secondSettings := neutralProfileSettings(second.config)
	projectSettings := neutralProfileSettings(service.Config)
	if first.config.ImplementationData[0].Payload[0] != '[' || !bytes.Equal(basePayload, base.ImplementationData[0].Payload) || secondSettings.Model != "model-b" || secondSettings.Args[0] != "--no-alt-screen" || projectSettings.Model != "" {
		t.Fatalf("fixed worker routes were mixed: first=%+v second=%+v project=%+v", first.config, second.config, service.Config)
	}
}

func workerContextConfigFixture() Config {
	config := Config{Persistence: sqlite.Factory{}, Repository: "repo", Branch: "main", Remote: "origin", publicationRemoteURL: "https://github.com/owner/repo.git", Host: "local", Agent: "fixture", WorkflowProtocol: WorkerFlowVersion, ExecutionPolicy: workspacePolicyFixture(), PublicationPolicy: executionPublicationFixture(), PR: hostPublicationProviderFixture(), Git: Command{Executable: "git"}}
	return withNeutralProfile(config, neutralSettings{Executable: os.Args[0]})
}
