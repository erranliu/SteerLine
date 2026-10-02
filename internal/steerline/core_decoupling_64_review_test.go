package steerline

import (
	"context"
	"errors"
	"testing"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

type recoveryOnlyLifecycle struct{ executionapi.LifecyclePort }

func (recoveryOnlyLifecycle) PolicyCapabilities() executionapi.LifecyclePolicyCapabilities {
	return executionapi.LifecyclePolicyCapabilities{}
}
func (recoveryOnlyLifecycle) VerifyManualRecovery(context.Context, executionapi.LifecycleSnapshot) (executionapi.LifecycleRecoveryEvidence, error) {
	return executionapi.LifecycleRecoveryEvidence{SessionAvailable: true, ProcessAbsent: true}, nil
}

type recoveryOnlyResolver struct{}

func (recoveryOnlyResolver) ResolveLifecycle(executionapi.LifecycleSetup) (executionapi.LifecyclePort, error) {
	return recoveryOnlyLifecycle{}, nil
}

type inspectionLifecycle struct {
	executionapi.LifecyclePort
	want executionapi.ProcessIdentity
	ref  executionapi.LifecycleSnapshot
}

func (p *inspectionLifecycle) VerifyManualRecovery(_ context.Context, snapshot executionapi.LifecycleSnapshot) (executionapi.LifecycleRecoveryEvidence, error) {
	p.ref = snapshot.Clone()
	if snapshot.Ref.External != p.want {
		return executionapi.LifecycleRecoveryEvidence{}, ErrWorkerTurnStale
	}
	return executionapi.LifecycleRecoveryEvidence{ManagedProcess: true, SessionAvailable: true}, nil
}

func (p *inspectionLifecycle) InspectLifecycleWorkspace(_ context.Context, snapshot executionapi.LifecycleSnapshot) (executionapi.WorkspaceInspection, error) {
	p.ref = snapshot.Clone()
	return executionapi.WorkspaceInspection{Repository: snapshot.Session.ProjectID, Worktree: snapshot.Ref.WorkspacePath, Branch: snapshot.Ref.WorkspaceBranch, Base: snapshot.Ref.BaseCommit, Head: snapshot.Ref.BaseCommit, Dirty: snapshot.WorkspaceDirty}, nil
}

type inspectionResolver struct{ port *inspectionLifecycle }

func (r inspectionResolver) ResolveLifecycle(executionapi.LifecycleSetup) (executionapi.LifecyclePort, error) {
	return r.port, nil
}

func TestCD64ReviewReceiptAndRecoveryIsolation(t *testing.T) {
	t.Run("recovery_preview_uses_original_lifecycle_port", func(t *testing.T) {
		s, run := appServerAuditService(t)
		execution := s.executionRecord(run)
		workflow := s.workflowRecord(run)
		execution.Generation, execution.ConfigurationDigest = "resources", workflow.ExecutionProfile.Digest
		identity := executionapi.ProcessIdentity{Kind: "fixture-process", PID: 42, Created: "original-generation"}
		execution.Process = &identity
		execution.PolicyResources = policyResourceReceiptFixture(workflow.ExecutionProfile.Digest, execution.Workspace.Worktree, execution)
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
		execution := ExecutionRecord{ProjectIdentity: "project", ExecutionID: "execution", CurrentAttemptID: "attempt", Generation: "generation", ConfigurationDigest: "config",
			Target: executionapi.ExecutionTarget{ID: "target"}}
		before := recordFingerprint(execution)
		target := execution.Target
		target.WorktreeID = "uncommitted-worktree"
		receipt := executionapi.LifecycleReceipt{Target: &target,
			Implementation: []executionapi.ImplementationDataEnvelope{{ImplementationKey: "broken"}}}
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
		s.domain.Operations = append(s.domain.Operations, OperationRecord{OperationID: newID(), Kind: string(executionapi.LifecycleAcknowledge), Status: "outcome_unknown",
			IdempotencyKey: "worker-delivery-ack:another-execution:another-delivery", Request: mustJSON(executionapi.AcknowledgeInput{DeliveryID: "another-delivery"})})
		if handled, err := s.resumeLifecycleWorkerFlow(context.Background(), nil, run); err != nil || !handled {
			t.Fatalf("unrelated ack blocked original recovery: handled=%v err=%v", handled, err)
		}
	})
	t.Run("intent_does_not_rebind_to_current_attempt", func(t *testing.T) {
		s, run := appServerAuditService(t)
		execution := s.executionRecord(run)
		workflow := s.workflowRecord(run)
		execution.Generation, execution.ConfigurationDigest = "resources", workflow.ExecutionProfile.Digest
		input := executionapi.TurnInput{CommandID: newID(), Prompt: "original"}
		key := "worker-session:" + execution.ExecutionID + ":" + input.CommandID
		_, err := s.startWorkerOperation(run, "prepare_worker_session", key, &input)
		require(t, err)
		snapshot, err := s.lifecycleSnapshot(run, workflow, execution, s.workflowAttempt(run, run.AttemptID), &input)
		require(t, err)
		snapshot.Session.AttemptID = "replacement-attempt"
		snapshot.Ref.AttemptID = snapshot.Session.AttemptID
		request := lifecycleRequest(snapshot, *s.operationByKey(key), executionapi.LifecyclePrepareSession, executionapi.LifecycleInput{Turn: &input})
		if err := request.Validate(); err == nil {
			t.Fatal("old intent was rebound to a replacement attempt")
		}
		if request.Operation.AttemptID == snapshot.Session.AttemptID {
			t.Fatal("original durable operation identity was replaced")
		}
	})
	t.Run("late_session_receipt_rejects_changed_binding", func(t *testing.T) {
		s, run := appServerAuditService(t)
		execution := s.executionRecord(run)
		workflow := s.workflowRecord(run)
		execution.Generation, execution.ConfigurationDigest = "resources", workflow.ExecutionProfile.Digest
		input := executionapi.TurnInput{CommandID: newID(), Prompt: "original"}
		execution.PendingCommandID = input.CommandID
		execution.PolicyResources = policyResourceReceiptFixture(workflow.ExecutionProfile.Digest, execution.Workspace.Worktree, execution)
		key := "worker-session:" + execution.ExecutionID + ":" + input.CommandID
		_, err := s.startWorkerOperation(run, "prepare_worker_session", key, &input)
		require(t, err)
		snapshot, err := s.lifecycleSnapshot(run, workflow, execution, s.workflowAttempt(run, run.AttemptID), &input)
		require(t, err)
		request := lifecycleRequest(snapshot, *s.operationByKey(key), executionapi.LifecyclePrepareSession, executionapi.LifecycleInput{Turn: &input})
		execution.Process = &executionapi.ProcessIdentity{Kind: "os", PID: 42, Created: "replacement"}
		before := recordFingerprint(execution)
		receipt := executionapi.LifecycleReceipt{Operation: request.Operation, Disposition: executionapi.OperationConfirmed, State: executionapi.LifecycleStateFacts{SessionID: "late-session"}}
		if _, proceed, err := s.applyAppServerSessionReceipt(run, request, receipt, nil); !errors.Is(err, ErrWorkerTurnStale) || proceed {
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
	_, err := s.domainStore.db.Exec(`CREATE TRIGGER reject_delivery_checkpoint BEFORE UPDATE ON projection BEGIN SELECT RAISE(FAIL, 'delivery commit failed'); END`)
	require(t, err)
	// An ack requires resolving an implementation. No resolver may be reached
	// after a failed content/consumption transaction.
	s.lifecycleResolver = nil
	event := WorkerEvent{RunID: run.RunID, AttemptID: run.AttemptID, Source: "execution", SourceEventID: "message-original",
		Fact:       executionapi.EventFact{Kind: executionapi.EventStatus, Source: "execution", SourceEventID: "message-original", LastMessage: "original content"},
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
		if operation.Kind == string(executionapi.LifecycleAcknowledge) {
			t.Fatal("failed message transaction issued an acknowledgement intent")
		}
	}
}
