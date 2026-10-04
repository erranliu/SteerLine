package steerline

import (
	"context"
	"errors"
	"testing"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

type recoveryOnlyLifecycle struct{ executionbackend.LifecyclePort }

func (recoveryOnlyLifecycle) PolicyCapabilities() executionbackend.LifecyclePolicyCapabilities {
	return executionbackend.LifecyclePolicyCapabilities{}
}

func (recoveryOnlyLifecycle) VerifyManualRecovery(context.Context, executionbackend.LifecycleSnapshot) (executionbackend.LifecycleRecoveryEvidence, error) {
	return executionbackend.LifecycleRecoveryEvidence{SessionAvailable: true, ProcessAbsent: true}, nil
}

type recoveryOnlyResolver struct{}

func (recoveryOnlyResolver) ResolveLifecycle(executionbackend.LifecycleSetup) (executionbackend.LifecyclePort, error) {
	return recoveryOnlyLifecycle{}, nil
}

type inspectionLifecycle struct {
	executionbackend.LifecyclePort
	want executionbackend.ProcessIdentity
	ref  executionbackend.LifecycleSnapshot
}

func (p *inspectionLifecycle) VerifyManualRecovery(_ context.Context, snapshot executionbackend.LifecycleSnapshot) (executionbackend.LifecycleRecoveryEvidence, error) {
	p.ref = snapshot.Clone()
	if snapshot.Ref.External != p.want {
		return executionbackend.LifecycleRecoveryEvidence{}, executionmodel.ErrTurnStale
	}
	return executionbackend.LifecycleRecoveryEvidence{ManagedProcess: true, SessionAvailable: true}, nil
}

func (p *inspectionLifecycle) InspectLifecycleWorkspace(_ context.Context, snapshot executionbackend.LifecycleSnapshot) (executionbackend.WorkspaceInspection, error) {
	p.ref = snapshot.Clone()
	return executionbackend.WorkspaceInspection{Repository: snapshot.Session.ProjectID, Worktree: snapshot.Ref.WorkspacePath, Branch: snapshot.Ref.WorkspaceBranch, Base: snapshot.Ref.BaseCommit, Head: snapshot.Ref.BaseCommit, Dirty: snapshot.WorkspaceDirty}, nil
}

type inspectionResolver struct{ port *inspectionLifecycle }

func (r inspectionResolver) ResolveLifecycle(executionbackend.LifecycleSetup) (executionbackend.LifecyclePort, error) {
	return r.port, nil
}

func TestCD64ReviewReceiptAndRecoveryIsolation(t *testing.T) {
	t.Run("recovery_preview_uses_original_lifecycle_port", func(t *testing.T) {
		s, run := neutralAuditService(t)
		execution := s.executionRecord(run)
		workflow := s.workflowRecord(run)
		execution.Generation, execution.ConfigurationDigest = "resources", workflow.ExecutionProfile.Digest
		execution.Workspace = executionmodel.WorkspaceEvidence{Repository: s.Config.identity, Worktree: run.WorkspaceIntent.Path, Branch: run.WorkspaceIntent.Branch, Base: run.WorkspaceIntent.BaseCommit}
		identity := executionbackend.ProcessIdentity{Kind: "fixture-process", PID: 42, Created: "original-generation"}
		execution.Process = &identity
		execution.PolicyResources = neutralPolicyReceipt(workflow.ExecutionProfile.Digest, execution.Workspace.Worktree, execution)
		s.refreshRun(run)
		port := &inspectionLifecycle{want: identity}
		s.lifecycleResolver = inspectionResolver{port: port}
		if got := s.inspectRunProcess(run); got != "matching process still exists" {
			t.Fatalf("inspection bypassed the injected original lifecycle: %q", got)
		}
		if port.ref.Session.ExecutionID != execution.ExecutionID || port.ref.Session.AttemptID != run.AttemptID {
			t.Fatal("recovery preview lost original execution/attempt identity")
		}
		evidence, err := s.inspectWorkflowWorkspace(context.Background(), run)
		require(t, err)
		if evidence.Worktree != execution.Workspace.Worktree || port.ref.Ref.Target != execution.Target || port.ref.Session.AttemptID != run.AttemptID {
			t.Fatal("workspace inspection bypassed the original lifecycle binding")
		}
	})
	t.Run("delivery_commit_failure_never_acknowledges", testCD64InboxCommitBeforeAck)
	t.Run("invalid_receipt_keeps_binding", func(t *testing.T) {
		execution := executionmodel.ExecutionRecord{ProjectIdentity: "project", ExecutionID: "execution", CurrentAttemptID: "attempt", Generation: "generation", ConfigurationDigest: "config",
			Target: executionbackend.ExecutionTarget{ID: "target"}}
		before := recordFingerprint(execution)
		target := execution.Target
		target.WorktreeID = "uncommitted-worktree"
		receipt := executionbackend.LifecycleReceipt{Target: &target,
			Implementation: []executionbackend.ImplementationDataEnvelope{{ImplementationKey: "broken"}}}
		if err := mergeLifecycleExecutionReceipt(&execution, receipt); err == nil {
			t.Fatal("damaged envelope was accepted")
		}
		if recordFingerprint(execution) != before {
			t.Fatal("rejected receipt leaked a candidate binding")
		}
	})
	t.Run("other_execution_ack_does_not_block_recovery", func(t *testing.T) {
		s, run := waitingDecisionService(t)
		t.Cleanup(func() { _ = s.Close() })
		s.lifecycleResolver = recoveryOnlyResolver{}
		execution := s.executionRecord(run)
		execution.RecoveryRequired = true
		s.domain.Operations = append(s.domain.Operations, executionmodel.OperationRecord{OperationID: newID(), Kind: string(executionbackend.LifecycleAcknowledge), Status: "outcome_unknown",
			IdempotencyKey: "worker-delivery-ack:another-execution:another-delivery", Request: mustJSON(executionbackend.AcknowledgeInput{DeliveryID: "another-delivery"})})
		if handled, err := s.resumeLifecycleWorkerFlow(context.Background(), nil, run); err != nil || !handled {
			t.Fatalf("unrelated ack blocked original recovery: handled=%v err=%v", handled, err)
		}
	})
	t.Run("intent_does_not_rebind_to_current_attempt", func(t *testing.T) {
		s, run := neutralAuditService(t)
		execution := s.executionRecord(run)
		workflow := s.workflowRecord(run)
		execution.Generation, execution.ConfigurationDigest = "resources", workflow.ExecutionProfile.Digest
		execution.Workspace = executionmodel.WorkspaceEvidence{Repository: s.Config.identity, Worktree: run.WorkspaceIntent.Path, Branch: run.WorkspaceIntent.Branch, Base: run.WorkspaceIntent.BaseCommit}
		input := executionbackend.TurnInput{CommandID: newID(), Prompt: "original"}
		key := "worker-session:" + execution.ExecutionID + ":" + input.CommandID
		_, err := s.startWorkerOperation(run, "prepare_worker_session", key, &input)
		require(t, err)
		snapshot, err := s.lifecycleSnapshot(run, workflow, execution, s.workflowAttempt(run, run.AttemptID), &input)
		require(t, err)
		snapshot.Session.AttemptID = "replacement-attempt"
		snapshot.Ref.AttemptID = snapshot.Session.AttemptID
		request := lifecycleRequest(snapshot, *s.operationByKey(key), executionbackend.LifecyclePrepareSession, executionbackend.LifecycleInput{Turn: &input})
		if err := request.Validate(); err == nil {
			t.Fatal("old intent was rebound to a replacement attempt")
		}
		if request.Operation.AttemptID == snapshot.Session.AttemptID {
			t.Fatal("original durable operation identity was replaced")
		}
	})
	t.Run("late_session_receipt_rejects_changed_binding", func(t *testing.T) {
		s, run := neutralAuditService(t)
		execution := s.executionRecord(run)
		workflow := s.workflowRecord(run)
		execution.Generation, execution.ConfigurationDigest = "resources", workflow.ExecutionProfile.Digest
		execution.Workspace = executionmodel.WorkspaceEvidence{Repository: s.Config.identity, Worktree: run.WorkspaceIntent.Path, Branch: run.WorkspaceIntent.Branch, Base: run.WorkspaceIntent.BaseCommit}
		input := executionbackend.TurnInput{CommandID: newID(), Prompt: "original"}
		execution.PendingCommandID = input.CommandID
		execution.PolicyResources = neutralPolicyReceipt(workflow.ExecutionProfile.Digest, execution.Workspace.Worktree, execution)
		key := "worker-session:" + execution.ExecutionID + ":" + input.CommandID
		_, err := s.startWorkerOperation(run, "prepare_worker_session", key, &input)
		require(t, err)
		snapshot, err := s.lifecycleSnapshot(run, workflow, execution, s.workflowAttempt(run, run.AttemptID), &input)
		require(t, err)
		request := lifecycleRequest(snapshot, *s.operationByKey(key), executionbackend.LifecyclePrepareSession, executionbackend.LifecycleInput{Turn: &input})
		execution.Process = &executionbackend.ProcessIdentity{Kind: "os", PID: 42, Created: "replacement"}
		before := recordFingerprint(execution)
		receipt := executionbackend.LifecycleReceipt{Operation: request.Operation, Disposition: executionbackend.OperationConfirmed, State: executionbackend.LifecycleStateFacts{SessionID: "late-session"}}
		if _, proceed, err := s.applyAppServerSessionReceipt(run, request, receipt, nil); !errors.Is(err, executionmodel.ErrTurnStale) || proceed {
			t.Fatalf("late session advanced changed binding: proceed=%v err=%v", proceed, err)
		}
		if recordFingerprint(*s.executionRecord(run)) != before || s.operationByKey(key).Status != "in_flight" {
			t.Fatal("stale result changed binding or original operation")
		}
	})
}

func testCD64InboxCommitBeforeAck(t *testing.T) {
	s, run := waitingDecisionService(t)
	t.Cleanup(func() { _ = s.Close() })
	_, err := testSQLite(s).db.Exec(`CREATE TRIGGER reject_delivery_checkpoint BEFORE UPDATE ON projection BEGIN SELECT RAISE(FAIL, 'delivery commit failed'); END`)
	require(t, err)
	// An ack requires resolving an implementation. No resolver may be reached
	// after a failed content/consumption transaction.
	s.lifecycleResolver = nil
	event := WorkerEvent{RunID: run.RunID, AttemptID: run.AttemptID, Source: "execution", SourceEventID: "message-original",
		Fact:       executionbackend.EventFact{Kind: executionbackend.EventStatus, Source: "execution", SourceEventID: "message-original", LastMessage: "original content"},
		Activity:   ActivityEvent{Source: "execution", Kind: "execution", Status: "observed", Title: "Original message"},
		DeliveryID: "delivery-original", DeliveryFinal: true, Persisted: make(chan error, 1)}
	if err := s.eventWorkerFlowFor(run, event); err == nil {
		t.Fatal("message committed despite transaction failure")
	}
	if err := <-event.Persisted; err == nil {
		t.Fatal("observer received a successful persistence acknowledgement")
	}
	if s.executionRecord(run).PendingDeliveryID != "" {
		t.Fatal("failed consumption fact leaked into actor state")
	}
	snapshot, err := s.domainStore.Query(context.Background(), s.Config.identity)
	require(t, err)
	for _, operation := range snapshot.Operations {
		if operation.Kind == string(executionbackend.LifecycleAcknowledge) {
			t.Fatal("failed message transaction issued an acknowledgement intent")
		}
	}
}
