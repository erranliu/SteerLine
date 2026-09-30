package steerline

import "testing"

func TestWorkerFlowAdapterUsesFixedProtocol(t *testing.T) {
	run := &Run{RunID: "run", ExecutionID: "execution", AttemptID: "attempt", Protocol: string(ProtocolLocalCodexAppServer)}
	service := Service{
		Config: Config{Repository: "repo", Host: "local", Agent: "codex"},
		State: Runtime{
			WorkflowRuns: []WorkflowRunRecord{{WorkflowRunID: "run", WorkflowVersion: WorkerFlowVersion}},
			Executions:   []ExecutionRecord{{ExecutionID: "execution", WorkflowRunID: "run"}},
			Attempts:     []StepAttemptRecord{{AttemptID: "attempt", ExecutionID: "execution"}},
		},
	}
	adapter, err := service.adapterForWorkerFlow(run)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := adapter.(codexAppWorkerAdapter); !ok {
		t.Fatalf("local fixed protocol chose %T", adapter)
	}
	adapter.stageIntent(&service.State.Executions[0], "command")
	if service.State.Executions[0].PendingCommandID != "command" || service.State.Executions[0].PendingTurnID != "" {
		t.Fatal("Codex command identity was treated as a native turn receipt")
	}
	wrong := *run
	wrong.Protocol = string(ProtocolOrcaTerminalV2)
	if _, err := service.adapterForWorkerFlow(&wrong); err == nil {
		t.Fatal("local execution accepted an Orca protocol adapter")
	}
}
