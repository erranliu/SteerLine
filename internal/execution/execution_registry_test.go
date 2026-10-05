package execution

import (
	"context"
	"errors"
	"testing"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	contracts "github.com/erranliu/SteerLine/internal/steerline/contracts"

	"github.com/erranliu/SteerLine/internal/execution/reconstruction"
)

func TestWorkerFlowAdapterUsesFixedProtocol(t *testing.T) {
	config := workerContextConfigFixture()
	fixed := config.projectExecutionProfile().Freeze()
	target := TargetFor(config)
	run := &contracts.Run{RunID: "run", ExecutionID: "execution", AttemptID: "attempt", Host: "local", Agent: "fixture", Backend: string(executionbackend.BackendLocal), Protocol: string(neutralProtocol), Target: target}
	service := Service{
		Config: config,
		domain: reconstruction.Snapshot{WorkflowRuns: []executionmodel.WorkflowRunRecord{{ProjectIdentity: ConfigurationProjectIdentity(config), WorkflowRunID: "run", WorkflowVersion: executionmodel.Version, Repository: config.Repository, Branch: config.Branch, ConfigDigest: fixed.AcceptedConfigVersion, ExecutionProfile: &fixed}},
			Executions: []executionmodel.ExecutionRecord{{ProjectIdentity: ConfigurationProjectIdentity(config), ConfigurationDigest: fixed.Digest, CurrentAttemptID: run.AttemptID, ExecutionID: "execution", WorkflowRunID: "run", Agent: "fixture", Backend: string(executionbackend.BackendLocal), Protocol: string(neutralProtocol), Target: target}},
			Attempts:   []executionmodel.StepAttemptRecord{{AttemptID: "attempt", ExecutionID: "execution"}}},
	}
	mode, err := service.workerFlowRegistration(run)
	if err != nil {
		t.Fatal(err)
	}
	if mode.Key.Protocol != neutralProtocol {
		t.Fatalf("local fixed protocol chose %v", mode)
	}
	if mode.Lifecycle == nil {
		t.Fatal("fixed local protocol has no lifecycle port")
	}
	wrong := *run
	wrong.Protocol = string(executionbackend.ProtocolLocalProcess)
	if _, err := service.workerFlowRegistration(&wrong); err == nil {
		t.Fatal("local execution accepted a foreign protocol adapter")
	}
}

func TestWorkerFlowRejectsConfigDriftBeforePreparationOrTurnIntent(t *testing.T) {
	service, run := neutralAuditService(t)
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
	service.workflowRecord(run).ExecutionProfile.Host = "unregistered"
	for _, action := range []func() error{
		func() error { return service.prepareWorkerFlow(ctx, *task, run) },
		func() error { return service.startWorkerFlowTurn(ctx, run, "fixed prompt", newID()) },
	} {
		err := action()
		var backendErr *executionbackend.BackendError
		if !errors.As(err, &backendErr) || backendErr.Code != executionbackend.BackendIdentityMismatch {
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
	config := workerContextConfigFixture()
	entry, err := registrationFor(config)
	require(t, err)
	foreign := entry
	foreign.Host, foreign.ProfileKey = "other-fixture-host", "fixture/other-stream-v1"
	foreign.Key.Protocol = "fixture_other_stream_v1"
	config.ExecutionDirectory, err = NewExecutionDirectory(config.ExecutionDirectory.Version(), neutralCodecs(), []ExecutionRegistration{entry, foreign})
	require(t, err)
	target := TargetFor(config)
	run := &contracts.Run{Host: config.Host, Agent: config.Agent, Backend: string(entry.Key.Backend), Protocol: string(entry.Key.Protocol), Target: target}
	record := &executionmodel.ExecutionRecord{Agent: config.Agent, Backend: run.Backend, Protocol: run.Protocol, Target: target}
	mode, err := workerFlowRegistrationFor(config, run, record)
	require(t, err)
	if mode.Key.Protocol != neutralProtocol {
		t.Fatalf("fixed protocol was replaced: %+v", mode)
	}
	changed := config
	changed.Host = "other-fixture-host"
	if _, err := workerFlowRegistrationFor(changed, run, record); !feedbackIdentityMismatch(err) {
		t.Fatalf("replacement host routed fixed feedback: %v", err)
	}
	record.Protocol = string(executionbackend.ProtocolLocalProcess)
	if _, err := workerFlowRegistrationFor(config, run, record); !feedbackIdentityMismatch(err) {
		t.Fatalf("replacement protocol routed fixed feedback: %v", err)
	}
}

func feedbackIdentityMismatch(err error) bool {
	var backendErr *executionbackend.BackendError
	return errors.As(err, &backendErr) && backendErr.Code == executionbackend.BackendIdentityMismatch
}

func TestEveryRegisteredExecutionUsesTheLifecyclePort(t *testing.T) {
	for key, entry := range neutralExecutionDirectory(t).entries {
		if entry.Lifecycle == nil {
			t.Fatalf("%+v has no lifecycle port", key)
		}
	}
}
