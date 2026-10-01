package steerline

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

func TestWorkerFlowAdapterUsesFixedProtocol(t *testing.T) {
	config := workerContextConfigFixture()
	fixed := config.projectExecutionProfile().Freeze()
	target := targetFor(config)
	run := &Run{RunID: "run", ExecutionID: "execution", AttemptID: "attempt", Host: "local", Agent: "codex", Backend: string(executionapi.BackendLocal), Protocol: string(executionapi.ProtocolLocalCodexAppServer), Target: target}
	service := Service{
		Config: config,
		domain: DomainSnapshot{WorkflowRuns: []WorkflowRunRecord{{WorkflowRunID: "run", WorkflowVersion: WorkerFlowVersion, Repository: config.Repository, Branch: config.Branch, ConfigDigest: fixed.AcceptedConfigVersion, ExecutionProfile: &fixed}},
			Executions: []ExecutionRecord{{ExecutionID: "execution", WorkflowRunID: "run", Agent: "codex", Backend: string(executionapi.BackendLocal), Protocol: string(executionapi.ProtocolLocalCodexAppServer), Target: target}},
			Attempts:   []StepAttemptRecord{{AttemptID: "attempt", ExecutionID: "execution"}}},
	}
	mode, err := service.workerFlowRegistration(run)
	if err != nil {
		t.Fatal(err)
	}
	if mode.Key.Protocol != executionapi.ProtocolLocalCodexAppServer {
		t.Fatalf("local fixed protocol chose %v", mode)
	}
	service.domain.Executions[0].PendingTurnID, service.domain.Executions[0].PendingCommandID = mode.PendingIdentity("command")
	if service.domain.Executions[0].PendingCommandID != "command" || service.domain.Executions[0].PendingTurnID != "" {
		t.Fatal("Codex command identity was treated as a native turn receipt")
	}
	wrong := *run
	wrong.Protocol = string(executionapi.ProtocolOrcaTerminalV2)
	if _, err := service.workerFlowRegistration(&wrong); err == nil {
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
		want     executionapi.ExecutionProtocol
	}{
		{"local", executionapi.ProtocolLocalCodexAppServer, executionapi.ProtocolLocalCodexAppServer},
		{"orca", executionapi.ProtocolOrcaTerminalV2, executionapi.ProtocolOrcaTerminalV2},
		{"orca_supervised", executionapi.ProtocolOrcaSupervised, executionapi.ProtocolOrcaSupervised},
	} {
		t.Run(test.host, func(t *testing.T) {
			config := Config{Host: test.host, Agent: "codex", Orca: OrcaConfig{Executable: "orca", Supervised: &SupervisedOrcaConfig{ConnectionID: "fixed-connection", EnvironmentID: "fixed-environment"}}}
			backend, _, err := executionapi.ExecutionBackendKind(test.host)
			require(t, err)
			target := targetFor(config)
			run := &Run{Host: test.host, Agent: "codex", Backend: backend, Protocol: string(test.protocol), Target: target}
			execution := &ExecutionRecord{Agent: "codex", Backend: backend, Protocol: string(test.protocol), Target: target}
			mode, err := workerFlowRegistrationFor(config, run, execution)
			require(t, err)
			if mode.Key.Protocol != test.want {
				t.Fatalf("mode = %v, want %v", mode, test.want)
			}
			changed := config
			changed.Host = "local"
			if test.host == "local" {
				changed.Host = "orca"
			}
			if _, err := workerFlowRegistrationFor(changed, run, execution); !feedbackIdentityMismatch(err) {
				t.Fatalf("replacement host routed fixed feedback: %v", err)
			}
			execution.Protocol = string(executionapi.ProtocolLocalProcess)
			if _, err := workerFlowRegistrationFor(config, run, execution); !feedbackIdentityMismatch(err) {
				t.Fatalf("replacement protocol routed fixed feedback: %v", err)
			}
		})
	}
}

func feedbackIdentityMismatch(err error) bool {
	var backendErr *executionapi.BackendError
	return errors.As(err, &backendErr) && backendErr.Code == executionapi.BackendIdentityMismatch
}

func TestRegisteredOrcaOperationsOwnDeliveryAndSettlement(t *testing.T) {
	c := Config{Host: "orca", Agent: "codex", Orca: OrcaConfig{Executable: "must-not-run"}}
	entry, err := registrationFor(c)
	require(t, err)
	original := builtinExecutionRegistry
	defer func() { builtinExecutionRegistry = original }()
	fake := *entry
	delivery, settlement, verified := false, false, false
	fake.OrcaOperations = func(Config) orcaExecutionOperations {
		ops := nativeOrcaOperations(c)
		ops.Delivery = func(context.Context, *Run, *ExecutionRecord, *OrcaNativeTurn) (executionapi.EventBatch, error) {
			delivery = true
			return executionapi.EventBatch{DeliveryID: "fixed-delivery", Facts: []executionapi.EventFact{{SourceEventID: "fixed-message", Matched: true}}}, nil
		}
		ops.Settlement = func(context.Context, *ExecutionRecord, *OrcaNativeTurn) (nativeOrcaSettlement, error) {
			settlement = true
			return nativeOrcaSettlement{Outcome: "completed", DispatchID: "fixed-dispatch"}, nil
		}
		ops.VerifyActive = func(context.Context, *ExecutionRecord, *OrcaNativeTurn) error { verified = true; return nil }
		return ops
	}
	builtinExecutionRegistry = map[executionRegistrationKey]*executionRegistration{fake.Key: &fake}
	e := &ExecutionRecord{OrcaNative: &OrcaNativeExecution{Coordinator: executionapi.ProcessIdentity{Terminal: "coordinator"}}, Process: &executionapi.ProcessIdentity{Terminal: "original"}, Target: executionapi.ExecutionTarget{OrchestrationRun: "original-run"}}
	run := &Run{RunID: "workflow", ExecutionID: "execution", AttemptID: "attempt"}
	tnr := &OrcaNativeTurn{TaskID: "task", DispatchID: "fixed-dispatch", TurnID: "fixed-turn"}
	d, err := readNativeOrcaDelivery(context.Background(), c, run, e, tnr)
	require(t, err)
	result, err := readNativeOrcaSettlement(context.Background(), c, e, tnr)
	require(t, err)
	require(t, verifyNativeOrcaActiveDispatch(context.Background(), c, e, tnr))
	if !delivery || !settlement || !verified || d.DeliveryID != "fixed-delivery" || result.DispatchID != "fixed-dispatch" {
		t.Fatal("business consumer bypassed registered target operations")
	}
	incomplete := *entry
	incomplete.OrcaOperations = nil
	if _, err = buildExecutionRegistry([]executionRegistration{incomplete}); err == nil {
		t.Fatal("missing registered Orca operations accepted")
	}
	fake.OrcaOperations = func(Config) orcaExecutionOperations { return orcaExecutionOperations{} }
	if _, err = orcaOperationsFor(c); !isBackendError(err, executionapi.BackendUnsupported) {
		t.Fatalf("incomplete operation set did not fail before invocation: %v", err)
	}
}

func TestRegisteredOrcaAckReconcilesLostReceipt(t *testing.T) {
	for _, host := range []string{"orca", "orca_supervised"} {
		t.Run(host, func(t *testing.T) {
			ctx := context.Background()
			dir := t.TempDir()
			c := Config{Host: host, Agent: "codex", Repository: dir, CommandTimeout: "5s", Orca: OrcaConfig{Executable: os.Args[0], WaitTimeout: "1ms"}}
			run := &Run{RunID: strings.Repeat("a", 32)}
			e := &ExecutionRecord{Process: &executionapi.ProcessIdentity{}, OrcaNative: &OrcaNativeExecution{}}
			deliveryID := "delivery-ask-1"
			if host == "orca" {
				c.Orca.Args = []string{"--fake", "orca-native", dir}
				require(t, os.WriteFile(filepath.Join(dir, "native-orca.json"), mustJSON(fakeNativeOrcaState{Dispatches: 1, LoseAckOnce: true}), 0600))
				e.Target.OrchestrationRun = "run-fake"
				e.OrcaNative.Coordinator = executionapi.ProcessIdentity{Terminal: "term-coordinator", RuntimeID: "fake-runtime"}
			} else {
				c.Orca.Args = []string{"--fake", "orca-remote-worker", dir}
				c.Orca.Supervised = &SupervisedOrcaConfig{ConnectionID: "home-connection", EnvironmentID: "env-home", CoordinatorTerminal: "term-remote-coordinator", PollInterval: "1ms"}
				require(t, os.WriteFile(filepath.Join(dir, "remote.json"), mustJSON(remoteFixtureState{Coordinator: "term-remote-coordinator", LoseAck: true, Question: true, Tasks: []remoteFixtureTask{{ID: "task-1", Dispatch: "dispatch-1"}}}), 0600))
				e.Target = targetFor(c)
				e.Target.OrchestrationRun, e.Target.CoordinatorTerminal = "remote-run", "term-remote-coordinator"
				e.Process = &executionapi.ProcessIdentity{Kind: "orca_supervised", OrchestrationRun: "remote-run", CoordinatorRuntimeID: "center"}
				e.OrcaNative.Coordinator = executionapi.ProcessIdentity{Terminal: "term-remote-coordinator", RuntimeID: "center"}
				deliveryID = "delivery-dispatch-1-question"
			}
			ops, err := orcaOperationsFor(c)
			require(t, err)
			acknowledged, err := ops.DeliveryAcknowledged(ctx, run, e, deliveryID)
			require(t, err)
			if acknowledged {
				t.Fatal("pending delivery was treated as acknowledged")
			}
			_, err = ops.Ack(ctx, run, e, deliveryID)
			if !isBackendError(err, executionapi.BackendOutcomeUnknown) {
				t.Fatalf("lost acknowledgement receipt was not uncertain: %v", err)
			}
			acknowledged, err = ops.DeliveryAcknowledged(ctx, run, e, deliveryID)
			require(t, err)
			if !acknowledged {
				t.Fatal("original applied acknowledgement could not be reconciled without resending")
			}
		})
	}
}
