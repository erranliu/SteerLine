package execution

import (
	"bytes"
	"os"
	"testing"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/execution/reconstruction"
)

func TestWorkerFlowContextKeepsFixedRunAndCopiesConfig(t *testing.T) {
	run := &reconstruction.RunView{RunID: "run", ExecutionID: "execution", AttemptID: "attempt"}
	config := workerContextConfigFixture()
	settings := neutralProfileSettings(config)
	settings.Args = []string{"--no-alt-screen"}
	setNeutralProfile(t, &config, settings)
	dependencies, _ := neutralFixtureRuntime(config)
	fixedProfile := testProjectExecutionProfile(config).Freeze(dependencies.Directory, ConfigurationProjectIdentity(config))
	run.Host, run.Agent, run.Backend, run.Protocol, run.Target = "local", "fixture", "local", string(neutralProtocol), TargetFor(config, dependencies.Directory)
	service := Service{
		Config: config, baseConfig: config, runtime: dependencies,
		domain: reconstruction.Snapshot{WorkflowRuns: []executionmodel.WorkflowRunRecord{{ProjectIdentity: ConfigurationProjectIdentity(config), WorkflowRunID: "run", WorkflowVersion: executionmodel.Version, Repository: config.Repository, Branch: config.Branch, ConfigDigest: fixedProfile.AcceptedConfigVersion, ExecutionProfile: &fixedProfile}},
			Executions: []executionmodel.ExecutionRecord{{ProjectIdentity: ConfigurationProjectIdentity(config), ExecutionID: "execution", WorkflowRunID: "run", CurrentAttemptID: run.AttemptID, ConfigurationDigest: fixedProfile.Digest}},
			Attempts:   []executionmodel.StepAttemptRecord{{AttemptID: "attempt", ExecutionID: "execution"}}},
	}
	fixed, err := service.workerRecords(run)
	if err != nil || fixed.run != run || fixed.execution.ExecutionID != "execution" {
		t.Fatalf("fixed worker context lost its identity: %+v %v", fixed, err)
	}
	fixed.config.ImplementationData[0].Payload[0] = '['
	if neutralProfileSettings(service.Config).Args[0] != "--no-alt-screen" {
		t.Fatal("fixed worker context shares mutable configuration with the project")
	}
	wrong := *run
	wrong.AttemptID = "other"
	if _, err := service.workerRecords(&wrong); err == nil {
		t.Fatal("worker context accepted an attempt from another execution")
	}
}

func TestWorkerFlowContextsDoNotExchangeExecutionProfiles(t *testing.T) {
	base := workerContextConfigFixture()
	routes := []ExecutionProfile{
		neutralProfile(t, "first", "First", neutralSettings{Executable: "fixture-a", Model: "model-a", Args: []string{"--search"}}),
		neutralProfile(t, "second", "Second", neutralSettings{Executable: "fixture-b", Model: "model-b", Args: []string{"--no-alt-screen"}}),
	}
	dependencies, _ := neutralFixtureRuntime(base)
	catalog := ProfileCatalog{Base: base, Directory: dependencies.Directory, Profiles: routes, DefaultID: "first", Global: true}
	service := Service{Config: base, baseConfig: base, runtime: dependencies}
	for _, route := range routes {
		id := route.ID
		selected, profile, err := catalog.ResolveExecutionProfile(id, "")
		if err != nil {
			t.Fatal(err)
		}
		fixed := profile.Freeze(dependencies.Directory, ConfigurationProjectIdentity(base))
		run := &reconstruction.RunView{RunID: id, ExecutionID: id + "-execution", AttemptID: id + "-attempt", Host: "local", Agent: "fixture", Backend: string(executionbackend.BackendLocal), Protocol: string(neutralProtocol), Target: TargetFor(selected, dependencies.Directory), ExecutionStatus: "pending"}
		service.State.Runs = append(service.State.Runs, run)
		service.domain.WorkflowRuns = append(service.domain.WorkflowRuns, executionmodel.WorkflowRunRecord{ProjectIdentity: ConfigurationProjectIdentity(base), WorkflowRunID: id, WorkflowVersion: executionmodel.Version, Repository: base.Repository, Branch: base.Branch, ConfigDigest: fixed.AcceptedConfigVersion, ExecutionProfile: &fixed})
		service.domain.Executions = append(service.domain.Executions, executionmodel.ExecutionRecord{ProjectIdentity: ConfigurationProjectIdentity(base), ExecutionID: run.ExecutionID, WorkflowRunID: id, CurrentAttemptID: run.AttemptID, ConfigurationDigest: fixed.Digest})
		service.domain.Attempts = append(service.domain.Attempts, executionmodel.StepAttemptRecord{AttemptID: run.AttemptID, ExecutionID: run.ExecutionID})
	}
	first, err := service.workerRecords(service.State.Runs[0])
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.workerRecords(service.State.Runs[1])
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
	config := Config{Repository: "repo", Branch: "main", Remote: "origin", publicationRemoteURL: "https://github.com/owner/repo.git", Host: "local", Agent: "fixture", WorkflowProtocol: executionmodel.Version, PublicationPolicy: executionPublicationFixture(), PR: hostPublicationProviderFixture(), Git: executionbackend.Command{Executable: "git"}}
	return withNeutralProfile(config, neutralSettings{Executable: os.Args[0]})
}
