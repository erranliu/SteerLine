package steerline

import (
	"context"
	"errors"
	"testing"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/workerflow"
)

func TestWorkerFlowAdapterUsesFixedProtocol(t *testing.T) {
	config := Config{Repository: "repo", Host: "local", Agent: "codex"}
	target := targetFor(config)
	run := &Run{RunID: "run", ExecutionID: "execution", AttemptID: "attempt", Host: "local", Agent: "codex", Backend: string(executionapi.BackendLocal), Protocol: string(executionapi.ProtocolLocalCodexAppServer), Target: target}
	service := Service{
		Config: config,
		domain: DomainSnapshot{WorkflowRuns: []WorkflowRunRecord{{WorkflowRunID: "run", WorkflowVersion: WorkerFlowVersion}},
			Executions: []ExecutionRecord{{ExecutionID: "execution", WorkflowRunID: "run", Agent: "codex", Backend: string(executionapi.BackendLocal), Protocol: string(executionapi.ProtocolLocalCodexAppServer), Target: target}},
			Attempts:   []StepAttemptRecord{{AttemptID: "attempt", ExecutionID: "execution"}}},
	}
	mode, err := service.workerFlowMode(run)
	if err != nil {
		t.Fatal(err)
	}
	if mode != workerflow.LocalCodex {
		t.Fatalf("local fixed protocol chose %v", mode)
	}
	service.domain.Executions[0].PendingTurnID, service.domain.Executions[0].PendingCommandID = mode.PendingIdentity("command")
	if service.domain.Executions[0].PendingCommandID != "command" || service.domain.Executions[0].PendingTurnID != "" {
		t.Fatal("Codex command identity was treated as a native turn receipt")
	}
	wrong := *run
	wrong.Protocol = string(executionapi.ProtocolOrcaTerminalV2)
	if _, err := service.workerFlowMode(&wrong); err == nil {
		t.Fatal("local execution accepted an Orca protocol adapter")
	}
}

func TestWorkerFlowRejectsConfigDriftBeforePreparationOrTurnIntent(t *testing.T) {
	service, run := appServerAuditService(t)
	ctx := context.Background()
	task := nativeTaskRecord(service.domain.Tasks, run.TaskID)
	if task == nil {
		t.Fatal("claimed native task is missing")
	}
	_, execution := service.workflowRecords(run)
	beforeOperations := len(service.domain.Operations)
	_, beforeRevision, err := service.domainStore.DatabaseVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	service.baseConfig.ExecutionPolicy.Network = "deny"
	if fixed, err := service.executionConfigForRun(run); err != nil || fixed.ExecutionPolicy.Network != "allow" {
		t.Fatalf("current config changed original policy: %v", err)
	}
	service.workflowRecord(run).ExecutionProfile.Host = "orca"
	for _, action := range []func() error{
		func() error { return service.prepareWorkerFlow(ctx, *task, run) },
		func() error { return service.startWorkerFlowTurn(ctx, run, "fixed prompt", newID()) },
	} {
		err := action()
		var backendErr *executionapi.BackendError
		if !errors.As(err, &backendErr) || backendErr.Code != executionapi.BackendIdentityMismatch {
			t.Fatalf("configuration drift reached a worker side effect: %v", err)
		}
	}
	_, afterRevision, err := service.domainStore.DatabaseVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if afterRevision != beforeRevision || len(service.domain.Operations) != beforeOperations || execution.PendingTurnID != "" || execution.PendingCommandID != "" {
		t.Fatalf("rejected route changed durable or actor state: revision=%d/%d operations=%d/%d pending=%q/%q", beforeRevision, afterRevision, beforeOperations, len(service.domain.Operations), execution.PendingTurnID, execution.PendingCommandID)
	}
}

func TestWorkerFlowModeUsesFixedExecutionIdentity(t *testing.T) {
	for _, test := range []struct {
		host     string
		protocol executionapi.ExecutionProtocol
		want     workerflow.Mode
	}{
		{"local", executionapi.ProtocolLocalCodexAppServer, workerflow.LocalCodex},
		{"orca", executionapi.ProtocolOrcaTerminalV2, workerflow.NativeOrca},
		{"orca_supervised", executionapi.ProtocolOrcaSupervised, workerflow.SupervisedOrca},
	} {
		t.Run(test.host, func(t *testing.T) {
			config := Config{Host: test.host, Agent: "codex", Orca: OrcaConfig{Executable: "orca", Supervised: &SupervisedOrcaConfig{ConnectionID: "fixed-connection", EnvironmentID: "fixed-environment"}}}
			backend, _, err := executionapi.ExecutionBackendKind(test.host)
			require(t, err)
			target := targetFor(config)
			run := &Run{Host: test.host, Agent: "codex", Backend: backend, Protocol: string(test.protocol), Target: target}
			execution := &ExecutionRecord{Agent: "codex", Backend: backend, Protocol: string(test.protocol), Target: target}
			mode, err := workerFlowModeFor(config, run, execution)
			require(t, err)
			if mode != test.want {
				t.Fatalf("mode = %v, want %v", mode, test.want)
			}
			changed := config
			changed.Host = "local"
			if test.host == "local" {
				changed.Host = "orca"
			}
			if _, err := workerFlowModeFor(changed, run, execution); !feedbackIdentityMismatch(err) {
				t.Fatalf("replacement host routed fixed feedback: %v", err)
			}
			execution.Protocol = string(executionapi.ProtocolLocalProcess)
			if _, err := workerFlowModeFor(config, run, execution); !feedbackIdentityMismatch(err) {
				t.Fatalf("replacement protocol routed fixed feedback: %v", err)
			}
		})
	}
}

func feedbackIdentityMismatch(err error) bool {
	var backendErr *executionapi.BackendError
	return errors.As(err, &backendErr) && backendErr.Code == executionapi.BackendIdentityMismatch
}
