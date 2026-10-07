package execution

import (
	"context"
	"errors"
	"strings"
	"testing"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	resourceprocess "github.com/erranliu/SteerLine/internal/execution/process"
)

func TestProjectPolicyMissingBoundResourcesAreNotReconstructed(t *testing.T) {
	c := Config{PublicationPolicy: executionPublicationFixture(), PR: hostPublicationProviderFixture()}
	e := executionmodel.ExecutionRecord{Generation: "original-generation", Process: &executionbackend.ProcessIdentity{Kind: "local"}}
	c = withNeutralProfile(c, neutralSettings{Executable: "fixture"})
	dependencies, _ := neutralFixtureRuntime(c)
	if err := bindPolicyResources(context.Background(), c, dependencies.Directory, dependencies.ProcessHost, &e, "workspace", nil); err == nil || !strings.Contains(err.Error(), "snapshot is missing") {
		t.Fatalf("missing original resources were reconstructed: %v", err)
	}
	if e.PolicyResources != nil {
		t.Fatal("original snapshot was backfilled")
	}
}

func TestAppServerLaunchMessagePersistsPolicyResources(t *testing.T) {
	t.Run("session_receipt", func(t *testing.T) {
		s, run := neutralAuditService(t)
		workflow, execution := s.workflowRecords(run)
		path := run.WorkspaceIntent.Path
		execution.Workspace = executionmodel.WorkspaceEvidence{Worktree: path, Branch: run.WorkspaceIntent.Branch, Base: run.WorkspaceIntent.BaseCommit}
		commandID := newID()
		execution.Generation = newID()
		execution.PendingCommandID = commandID
		execution.ConfigurationDigest = workflow.ExecutionProfile.Digest
		s.refreshRun(run)
		input := executionbackend.TurnInput{CommandID: commandID, Prompt: "fixed task instruction"}
		key := "worker-session:" + run.ExecutionID + ":" + commandID
		opID, err := s.startWorkerOperation(run, "prepare_worker_session", key, &input)
		require(t, err)
		identity, err := resourceprocess.CurrentIdentity()
		require(t, err)
		partial := *s.executionRecord(run)
		partial.Process = identity
		setNeutralBinding(t, &partial, "ws://127.0.0.1:12345", "policy-generation", workerFlowMockSession)
		resource := neutralPolicyReceipt(workflow.ExecutionProfile.Digest, path, &partial)
		s.executionRecord(run).PolicyResources = resource
		snapshot, err := lifecycleSnapshot(run, s.workflowRecord(run), s.executionRecord(run), s.workflowAttempt(run, run.AttemptID), &input)
		require(t, err)
		operation := s.operationByKey(key)
		request := lifecycleRequest(snapshot, *operation, executionbackend.LifecyclePrepareSession, executionbackend.LifecycleInput{Turn: &input})
		receipt := executionbackend.LifecycleReceipt{
			Operation: request.Operation, Disposition: executionbackend.OperationUnknown,
			RequestID: workerFlowMockSession, Process: identity,
			Implementation: partial.ImplementationData,
			State:          executionbackend.LifecycleStateFacts{SessionID: workerFlowMockSession, SessionGeneration: "policy-generation"},
		}
		_, _, applyErr := s.applyAppServerSessionReceipt(run, request, receipt, errors.New("session receipt persistence failed"))
		require(t, applyErr)
		snapshotResult, err := readServiceSnapshot(context.Background(), s.domainStore, s.Config.identity)
		require(t, err)
		got := snapshotResult.Executions[0]
		if got.PolicyResources == nil || got.PolicyResources.Digest != resource.Digest || got.SessionID != workerFlowMockSession || got.PendingTurnID != "" || s.workers[run.RunID] != nil {
			t.Fatalf("policy resource receipt was lost or advanced a turn: %+v", got)
		}
		if storedOperationStatus(snapshotResult.Operations, opID) != "outcome_unknown" {
			t.Fatal("unverified session receipt changed the original operation identity")
		}
		_, err = s.executionConfigForRun(run)
		require(t, err)
	})

	t.Run("turn_result", func(t *testing.T) {
		s, run := neutralAuditService(t)
		workflow, execution := s.workflowRecords(run)
		path := run.WorkspaceIntent.Path
		execution.Workspace = executionmodel.WorkspaceEvidence{Worktree: path, Branch: run.WorkspaceIntent.Branch, Base: run.WorkspaceIntent.BaseCommit}
		commandID := newID()
		execution.PendingCommandID = commandID
		identity, err := resourceprocess.CurrentIdentity()
		require(t, err)
		execution.Process = identity
		setNeutralBinding(t, execution, "ws://127.0.0.1:12345", "policy-generation", workerFlowMockSession)
		execution.SessionID, execution.SessionGeneration = workerFlowMockSession, "policy-generation"
		resource := neutralPolicyReceipt(workflow.ExecutionProfile.Digest, path, execution)
		execution.PolicyResources = resource
		s.refreshRun(run)
		input := executionbackend.TurnInput{CommandID: commandID, Prompt: "fixed task instruction"}
		key := "worker-turn:" + run.ExecutionID + ":" + commandID
		opID, err := s.startWorkerOperation(run, "start_worker_turn", key, &input)
		require(t, err)
		snapshot, err := lifecycleSnapshot(run, s.workflowRecord(run), execution, s.workflowAttempt(run, run.AttemptID), &input)
		require(t, err)
		operation := s.operationByKey(key)
		request := lifecycleRequest(snapshot, *operation, executionbackend.LifecycleStartTurn, executionbackend.LifecycleInput{Turn: &input})
		receipt := executionbackend.LifecycleReceipt{Operation: request.Operation, Disposition: executionbackend.OperationUnknown, RequestID: workerFlowMockSession, Process: identity,
			Implementation: CloneImplementationData(execution.ImplementationData), State: executionbackend.LifecycleStateFacts{SessionID: workerFlowMockSession, SessionGeneration: "policy-generation"}}
		applyErr := s.applyAppServerTurnReceipt(run, request, receipt, nil, errors.New("turn receipt persistence failed"))
		require(t, applyErr)
		snapshotResult, err := readServiceSnapshot(context.Background(), s.domainStore, s.Config.identity)
		require(t, err)
		got := snapshotResult.Executions[0]
		if got.PolicyResources == nil || got.PolicyResources.Digest != resource.Digest || got.SessionID != workerFlowMockSession || got.PendingTurnID != "" || s.workers[run.RunID] != nil {
			t.Fatalf("policy resource receipt was lost or advanced a turn: %+v", got)
		}
		if storedOperationStatus(snapshotResult.Operations, opID) != "outcome_unknown" {
			t.Fatal("unverified turn receipt changed the original operation identity")
		}
		_, err = s.executionConfigForRun(run)
		require(t, err)
	})
}

func storedOperationStatus(records []executionmodel.OperationRecord, id string) string {
	for _, record := range records {
		if record.OperationID == id {
			return record.Status
		}
	}
	return ""
}
