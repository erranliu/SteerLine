package steerline

import (
	"context"
	"testing"

	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
)

func testCD63RecoverySeedResources(t *testing.T) {
	s, run := appServerAuditService(t)
	workflow, record := s.workflowRecords(run)
	record.Process, record.PolicyResources = nil, nil
	record.Status, record.RecoveryState, record.RecoveryRequired = "pending", "resuming", false
	record.Generation, record.ConfigurationDigest = newID(), workflow.ExecutionProfile.Digest
	record.CurrentTurnID, record.PendingTurnID = "", ""
	removeImplementationEnvelope(record, codexworker.ExecutionBindingCodecKey)
	require(t, applyCodexBinding(record, codexworker.Binding{SessionID: workerFlowMockSession}))
	s.refreshRun(run)
	if _, err := s.executionConfigForRun(run); err != nil {
		t.Fatalf("original thread recovery seed was rejected before resource binding: %v", err)
	}
	require(t, s.commitWorker(run))
	require(t, preflightWorkerFlowState(context.Background(), s.Config))
	acceptedDigest := record.ConfigurationDigest
	record.ConfigurationDigest = "other-accepted-snapshot"
	require(t, applyCodexBinding(record, codexworker.Binding{SessionID: workerFlowMockSession}))
	if _, err := s.executionConfigForRun(run); err == nil {
		t.Fatal("recovery seed from another fixed configuration was accepted")
	}
	record.ConfigurationDigest = acceptedDigest
	require(t, applyCodexBinding(record, codexworker.Binding{SessionID: workerFlowMockSession}))
	record.RecoveryState = "interrupted"
	if _, err := s.executionConfigForRun(run); err == nil {
		t.Fatal("missing original resource receipt was accepted outside pending recovery")
	}
}

func testCD63BindingCodecIsolation(t *testing.T) {
	record := ExecutionRecord{Protocol: string(executionapi.ProtocolLocalCodexAppServer)}
	setTestCodexBinding(t, &record, "ws://127.0.0.1:12345", "native-generation", workerFlowMockSession)
	t.Run("duplicate binding is an error", func(t *testing.T) {
		copy := record
		copy.ImplementationData = append(cloneImplementationData(record.ImplementationData), record.ImplementationData[0].Clone())
		if _, err := decodeCodexAppServerBinding(&copy); err == nil {
			t.Fatal("duplicate binding was treated as an unbound execution")
		}
	})
	t.Run("codec must belong to protocol", func(t *testing.T) {
		copy := record
		foreign, err := orcaworker.NewExecutionStateEnvelope(orcaStateIdentity(&copy), orcaworker.NativeExecutionState{RepositoryID: "repo"})
		require(t, err)
		copy.ImplementationData = []executionapi.ImplementationDataEnvelope{foreign}
		if err := validateExecutionImplementationData(&copy); err == nil {
			t.Fatal("Codex execution accepted an Orca binding")
		}
	})
	t.Run("seed cannot stand in for a bound process", func(t *testing.T) {
		copy := record
		removeImplementationEnvelope(&copy, codexworker.ExecutionBindingCodecKey)
		require(t, applyCodexBinding(&copy, codexworker.Binding{SessionID: workerFlowMockSession}))
		copy.Process = &executionapi.ProcessIdentity{Kind: "os", PID: 1, Created: "process-generation"}
		if err := validateExecutionImplementationData(&copy); err == nil {
			t.Fatal("bound process accepted a recovery seed with no endpoint or native generation")
		}
	})
	t.Run("native coordinator must match the target", func(t *testing.T) {
		copy := record
		copy.Protocol = string(executionapi.ProtocolOrcaTerminalV2)
		copy.Target.CoordinatorTerminal = "original-coordinator"
		foreign, err := orcaworker.NewExecutionStateEnvelope(orcaStateIdentity(&copy), orcaworker.NativeExecutionState{Coordinator: executionapi.ProcessIdentity{Terminal: "replacement-coordinator", RuntimeID: "runtime"}})
		require(t, err)
		copy.ImplementationData = []executionapi.ImplementationDataEnvelope{foreign}
		if err := validateExecutionImplementationData(&copy); err == nil {
			t.Fatal("recomputed envelope digest hid a different native target")
		}
	})
}
