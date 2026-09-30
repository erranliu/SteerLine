package steerline

import "testing"

func TestWorkerFlowContextKeepsFixedRunAndCopiesConfig(t *testing.T) {
	run := &Run{RunID: "run", ExecutionID: "execution", AttemptID: "attempt"}
	service := Service{
		Config: Config{Repository: "repo", Codex: Command{Args: []string{"original"}}, workerEnv: map[string]string{"A": "original"}, Orca: OrcaConfig{Supervised: &SupervisedOrcaConfig{CoordinatorTerminal: "original"}}},
		State: Runtime{
			WorkflowRuns: []WorkflowRunRecord{{WorkflowRunID: "run", WorkflowVersion: WorkerFlowVersion}},
			Executions:   []ExecutionRecord{{ExecutionID: "execution", WorkflowRunID: "run"}},
			Attempts:     []StepAttemptRecord{{AttemptID: "attempt", ExecutionID: "execution"}},
		},
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
