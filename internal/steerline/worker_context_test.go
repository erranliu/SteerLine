package steerline

import (
	"testing"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

func TestWorkerFlowContextKeepsFixedRunAndCopiesConfig(t *testing.T) {
	run := &Run{RunID: "run", ExecutionID: "execution", AttemptID: "attempt"}
	service := Service{
		Config: Config{Repository: "repo", Codex: Command{Args: []string{"original"}}, workerEnv: map[string]string{"A": "original"}, Orca: OrcaConfig{Supervised: &SupervisedOrcaConfig{CoordinatorTerminal: "original"}}},
		domain: DomainSnapshot{WorkflowRuns: []WorkflowRunRecord{{WorkflowRunID: "run", WorkflowVersion: WorkerFlowVersion}},
			Executions: []ExecutionRecord{{ExecutionID: "execution", WorkflowRunID: "run"}},
			Attempts:   []StepAttemptRecord{{AttemptID: "attempt", ExecutionID: "execution"}}},
	}
	fixed, err := service.workerFlowContext(run)
	if err != nil || fixed.run != run || fixed.execution.ExecutionID != "execution" {
		t.Fatalf("fixed worker context lost its identity: %+v %v", fixed, err)
	}
	fixed.config.Codex.Args[0] = "changed"
	fixed.config.workerEnv["A"] = "changed"
	fixed.config.Orca.Supervised.CoordinatorTerminal = "changed"
	if service.Config.Codex.Args[0] != "original" || service.Config.workerEnv["A"] != "original" || service.Config.Orca.Supervised.CoordinatorTerminal != "original" {
		t.Fatal("fixed worker context shares mutable configuration with the project")
	}
	wrong := *run
	wrong.AttemptID = "other"
	if _, err := service.workerFlowContext(&wrong); err == nil {
		t.Fatal("worker context accepted an attempt from another execution")
	}
}

func TestWorkerFlowContextsDoNotExchangeExecutionProfiles(t *testing.T) {
	base := Config{Repository: "repo", Host: "local", Agent: "codex", Sandbox: "workspace-write"}
	routes := []ExecutionProfile{
		{ID: "first", Name: "First", Host: "local", Agent: "codex", Model: "model-a", Sandbox: "read-only", Codex: Command{Executable: "codex-a", Args: []string{"a"}}},
		{ID: "second", Name: "Second", Host: "local", Agent: "codex", Model: "model-b", Sandbox: "workspace-write", Codex: Command{Executable: "codex-b", Args: []string{"b"}}},
	}
	service := Service{Config: base, baseConfig: base}
	for _, route := range routes {
		id := route.ID
		fixed := route.Freeze()
		selected := base.ApplyFixedExecutionProfile(fixed)
		run := &Run{RunID: id, ExecutionID: id + "-execution", AttemptID: id + "-attempt", Host: "local", Agent: "codex", Backend: string(executionapi.BackendLocal), Protocol: string(executionapi.ProtocolLocalCodexAppServer), Target: targetFor(selected), ExecutionStatus: "pending"}
		service.State.Runs = append(service.State.Runs, run)
		service.domain.WorkflowRuns = append(service.domain.WorkflowRuns, WorkflowRunRecord{WorkflowRunID: id, WorkflowVersion: WorkerFlowVersion, ExecutionProfile: &fixed})
		service.domain.Executions = append(service.domain.Executions, ExecutionRecord{ExecutionID: run.ExecutionID, WorkflowRunID: id})
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
	first.config.Codex.Args[0] = "changed"
	if first.config.Model != "model-a" || second.config.Model != "model-b" || second.config.Codex.Args[0] != "b" || service.Config.Model != "" {
		t.Fatalf("fixed worker routes were mixed: first=%+v second=%+v project=%+v", first.config, second.config, service.Config)
	}
}
