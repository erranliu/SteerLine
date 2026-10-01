package steerline

import (
	"context"
	"errors"
	"os"
	"os/exec"
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
	setTestCodexBinding(t, record, "", "", workerFlowMockSession)
	s.refreshRun(run)
	if _, err := s.executionConfigForRun(run); err != nil {
		t.Fatalf("original thread recovery seed was rejected before resource binding: %v", err)
	}
	require(t, s.commitWorker(run))
	require(t, preflightWorkerFlowState(context.Background(), s.Config))
	acceptedDigest := record.ConfigurationDigest
	record.ConfigurationDigest = "other-accepted-snapshot"
	setTestCodexBinding(t, record, "", "", workerFlowMockSession)
	if _, err := s.executionConfigForRun(run); err == nil {
		t.Fatal("recovery seed from another fixed configuration was accepted")
	}
	record.ConfigurationDigest = acceptedDigest
	setTestCodexBinding(t, record, "", "", workerFlowMockSession)
	record.RecoveryState = "interrupted"
	if _, err := s.executionConfigForRun(run); err == nil {
		t.Fatal("missing original resource receipt was accepted outside pending recovery")
	}
}

func testCD63BindingCodecIsolation(t *testing.T) {
	record := ExecutionRecord{Protocol: string(executionapi.ProtocolLocalCodexAppServer)}
	setTestCodexBinding(t, &record, "ws://127.0.0.1:12345", "native-generation", workerFlowMockSession)
	snapshotFor := func(value *ExecutionRecord) executionapi.LifecycleSnapshot {
		return executionapi.LifecycleSnapshot{
			Session:             executionapi.SessionContext{ProjectID: value.ProjectIdentity, WorkflowRunID: value.WorkflowRunID, ExecutionID: value.ExecutionID, AttemptID: value.CurrentAttemptID, SessionID: value.SessionID, Generation: value.SessionGeneration},
			ConfigurationDigest: value.ConfigurationDigest, ResourceGeneration: value.Generation,
			Implementation: cloneImplementationData(value.ImplementationData),
			PendingTurn:    &executionapi.TurnInput{CommandID: newID(), Prompt: "fixed instruction"},
		}
	}
	t.Run("duplicate binding is an error", func(t *testing.T) {
		copy := record
		copy.ImplementationData = append(cloneImplementationData(record.ImplementationData), record.ImplementationData[0].Clone())
		if _, err := (codexworker.ExecutionLifecycle{}).Next(context.Background(), snapshotFor(&copy)); err == nil {
			t.Fatal("duplicate binding was treated as an unbound execution")
		}
	})
	t.Run("codec must belong to protocol", func(t *testing.T) {
		copy := record
		foreign, err := orcaworker.NewExecutionStateEnvelope(orcaStateIdentity(&copy), orcaworker.NativeExecutionState{RepositoryID: "repo"})
		require(t, err)
		copy.ImplementationData = []executionapi.ImplementationDataEnvelope{foreign}
		if _, err := (codexworker.ExecutionLifecycle{}).Next(context.Background(), snapshotFor(&copy)); err == nil {
			t.Fatal("Codex implementation accepted an Orca binding")
		}
	})
	t.Run("seed cannot stand in for a bound process", func(t *testing.T) {
		copy := record
		removeImplementationEnvelope(&copy, codexworker.ExecutionBindingCodecKey)
		setTestCodexBinding(t, &copy, "", "", workerFlowMockSession)
		copy.Process = &executionapi.ProcessIdentity{Kind: "os", PID: 1, Created: "process-generation"}
		workspace, serverDir := t.TempDir(), t.TempDir()
		ref := executionapi.ExecutionRef{Version: executionapi.ExecutionRecordVersion, Backend: executionapi.BackendLocal, Protocol: executionapi.ProtocolLocalCodexAppServer, Target: copy.Target, Agent: executionapi.AgentSpec{Name: "codex"}, RunID: copy.WorkflowRunID, AttemptID: copy.CurrentAttemptID, WorkspacePath: workspace, External: *copy.Process, Generation: copy.SessionGeneration}
		session := executionapi.SessionContext{ProjectID: copy.ProjectIdentity, TaskID: "task", WorkflowRunID: copy.WorkflowRunID, ExecutionID: copy.ExecutionID, AttemptID: copy.CurrentAttemptID, Agent: executionapi.AgentSpec{Name: "codex"}, Target: copy.Target, SessionID: copy.SessionID}
		snapshot := executionapi.LifecycleSnapshot{Ref: ref, Session: session, ConfigurationDigest: copy.ConfigurationDigest, ResourceGeneration: copy.Generation, Implementation: cloneImplementationData(copy.ImplementationData), Workspace: executionapi.WorkspaceIntent{Path: workspace}}
		input := executionapi.TurnInput{CommandID: newID(), Prompt: "original saved input"}
		operation := executionapi.Operation{Version: 1, ID: newID(), RunID: copy.WorkflowRunID, AttemptID: copy.CurrentAttemptID, Backend: ref.Backend, Protocol: ref.Protocol, Target: ref.Target, Action: string(executionapi.LifecyclePrepareSession), PayloadDigest: operationDigest(mustJSON(input)), Status: executionapi.OperationInFlight, Ref: &ref}
		startCalled := false
		lifecycle := codexworker.ExecutionLifecycle{Backend: codexworker.Backend{Config: codexworker.Config{Policy: workspacePolicyFixture(), Executable: os.Args[0], Workspace: workspace, RunDir: t.TempDir(), RunID: copy.WorkflowRunID, ServerDir: serverDir, Sandbox: "workspace-write", WindowsSandbox: "elevated"}, Process: codexworker.ProcessPort{Start: func(*exec.Cmd) (executionapi.StopHandle, error) {
			startCalled = true
			return nil, errors.New("unexpected process start")
		}, Inspect: func(*executionapi.ProcessIdentity) string { return "matching process still exists" }, NewGeneration: newID, WriteAtomic: AtomicWrite}}}
		receipt, _, err := lifecycle.Execute(context.Background(), executionapi.LifecycleRequest{Snapshot: snapshot, Operation: operation, Input: executionapi.LifecycleInput{Turn: &input}})
		if err == nil || startCalled || receipt.Disposition != executionapi.OperationUnknown {
			t.Fatalf("recovery seed authorized replacing its missing original process receipt: receipt=%+v err=%v start_called=%v", receipt, err, startCalled)
		}
	})
	t.Run("native coordinator must match the target", func(t *testing.T) {
		copy := record
		copy.Protocol = string(executionapi.ProtocolOrcaTerminalV2)
		copy.Target.CoordinatorTerminal = "original-coordinator"
		foreign, err := orcaworker.NewExecutionStateEnvelope(orcaStateIdentity(&copy), orcaworker.NativeExecutionState{Coordinator: executionapi.ProcessIdentity{Terminal: "replacement-coordinator", RuntimeID: "runtime"}})
		require(t, err)
		copy.ImplementationData = []executionapi.ImplementationDataEnvelope{foreign}
		if _, err := orcaworker.DecodeExecutionStateEnvelope(foreign, orcaStateIdentity(&copy)); err == nil {
			t.Fatal("Orca implementation accepted a different native coordinator target")
		}
	})
}
