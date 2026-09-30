package steerline

import (
	"context"
	"errors"
	"testing"

	"github.com/erranliu/SteerLine/internal/steerline/workerflow"
)

func TestWorkerFlowAdapterUsesFixedProtocol(t *testing.T) {
	config := Config{Repository: "repo", Host: "local", Agent: "codex"}
	target := targetFor(config)
	run := &Run{RunID: "run", ExecutionID: "execution", AttemptID: "attempt", Host: "local", Agent: "codex", Backend: string(BackendLocal), Protocol: string(ProtocolLocalCodexAppServer), Target: target}
	service := Service{
		Config: config,
		State: Runtime{
			WorkflowRuns: []WorkflowRunRecord{{WorkflowRunID: "run", WorkflowVersion: WorkerFlowVersion}},
			Executions:   []ExecutionRecord{{ExecutionID: "execution", WorkflowRunID: "run", Agent: "codex", Backend: string(BackendLocal), Protocol: string(ProtocolLocalCodexAppServer), Target: target}},
			Attempts:     []StepAttemptRecord{{AttemptID: "attempt", ExecutionID: "execution"}},
		},
	}
	mode, err := service.workerFlowMode(run)
	if err != nil {
		t.Fatal(err)
	}
	if mode != workerflow.LocalCodex {
		t.Fatalf("local fixed protocol chose %v", mode)
	}
	service.State.Executions[0].PendingTurnID, service.State.Executions[0].PendingCommandID = mode.PendingIdentity("command")
	if service.State.Executions[0].PendingCommandID != "command" || service.State.Executions[0].PendingTurnID != "" {
		t.Fatal("Codex command identity was treated as a native turn receipt")
	}
	wrong := *run
	wrong.Protocol = string(ProtocolOrcaTerminalV2)
	if _, err := service.workerFlowMode(&wrong); err == nil {
		t.Fatal("local execution accepted an Orca protocol adapter")
	}
}

func TestWorkerFlowRejectsConfigDriftBeforePreparationOrTurnIntent(t *testing.T) {
	service, run := appServerAuditService(t)
	ctx := context.Background()
	task := nativeTaskRecord(service.State.Tasks, run.TaskID)
	if task == nil {
		t.Fatal("claimed native task is missing")
	}
	_, execution := service.workflowRecords(run)
	beforeOperations := len(service.State.Operations)
	_, beforeRevision, err := service.domainStore.DatabaseVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	service.baseConfig.Host = "orca"
	for _, action := range []func() error{
		func() error { return service.prepareWorkerFlow(ctx, *task, run) },
		func() error { return service.startWorkerFlowTurn(ctx, run, "fixed prompt", newID()) },
	} {
		err := action()
		var backendErr *BackendError
		if !errors.As(err, &backendErr) || backendErr.Code != BackendIdentityMismatch {
			t.Fatalf("configuration drift reached a worker side effect: %v", err)
		}
	}
	_, afterRevision, err := service.domainStore.DatabaseVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if afterRevision != beforeRevision || len(service.State.Operations) != beforeOperations || execution.PendingTurnID != "" || execution.PendingCommandID != "" {
		t.Fatalf("rejected route changed durable or actor state: revision=%d/%d operations=%d/%d pending=%q/%q", beforeRevision, afterRevision, beforeOperations, len(service.State.Operations), execution.PendingTurnID, execution.PendingCommandID)
	}
}
