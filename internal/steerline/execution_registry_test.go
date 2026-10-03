package steerline

import (
	"context"
	"errors"
	"testing"

	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	remote "github.com/erranliu/SteerLine/internal/steerline/orca_remote_worker"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
)

func TestWorkerFlowAdapterUsesFixedProtocol(t *testing.T) {
	config := workerContextConfigFixture()
	fixed := config.projectExecutionProfile().Freeze()
	target := targetFor(config)
	run := &Run{RunID: "run", ExecutionID: "execution", AttemptID: "attempt", Host: "local", Agent: "codex", Backend: string(executionapi.BackendLocal), Protocol: string(executionapi.ProtocolLocalCodexAppServer), Target: target}
	service := Service{
		Config: config,
		domain: DomainSnapshot{WorkflowRuns: []WorkflowRunRecord{{ProjectIdentity: configurationProjectIdentity(config), WorkflowRunID: "run", WorkflowVersion: WorkerFlowVersion, Repository: config.Repository, Branch: config.Branch, ConfigDigest: fixed.AcceptedConfigVersion, ExecutionProfile: &fixed}},
			Executions: []ExecutionRecord{{ProjectIdentity: configurationProjectIdentity(config), ConfigurationDigest: fixed.Digest, CurrentAttemptID: run.AttemptID, ExecutionID: "execution", WorkflowRunID: "run", Agent: "codex", Backend: string(executionapi.BackendLocal), Protocol: string(executionapi.ProtocolLocalCodexAppServer), Target: target}},
			Attempts:   []StepAttemptRecord{{AttemptID: "attempt", ExecutionID: "execution"}}},
	}
	mode, err := service.workerFlowRegistration(run)
	if err != nil {
		t.Fatal(err)
	}
	if mode.Key.Protocol != executionapi.ProtocolLocalCodexAppServer {
		t.Fatalf("local fixed protocol chose %v", mode)
	}
	if mode.Lifecycle == nil {
		t.Fatal("fixed local protocol has no lifecycle port")
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
			config := Config{ExecutionDirectory: testExecutionDirectory(t), Host: test.host, Agent: "codex"}
			orca := &orcaworker.ProfileConfig{Executable: "orca", Shell: "cmd.exe", WaitTimeout: "1m"}
			var remoteSettings *remote.ProfileConfig
			if test.host == "orca_supervised" {
				remoteSettings = &remote.ProfileConfig{ConnectionID: "fixed-connection", EnvironmentID: "fixed-environment", RepositorySelector: "repo", CoordinatorTerminal: "terminal", PollInterval: "1s", CapabilityReportVersion: 1}
			}
			setTestImplementationProfiles(t, &config, codexworker.ProfileConfig{Executable: "codex"}, orca, remoteSettings)
			entry, err := registrationFor(config)
			require(t, err)
			backend := string(entry.Key.Backend)
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

func TestEveryRegisteredExecutionUsesTheLifecyclePort(t *testing.T) {
	for key, entry := range testExecutionDirectory(t).entries {
		if entry.Lifecycle == nil {
			t.Fatalf("%+v has no lifecycle port", key)
		}
	}
}
