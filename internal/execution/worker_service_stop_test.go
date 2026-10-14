package execution

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	resourceprocess "github.com/erranliu/SteerLine/internal/execution/process"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
)

func stoppedWorkerFixture(t *testing.T) (*Service, *reconstructiontypes.RunView, workerRecoveryResult) {
	t.Helper()
	s, run := neutralAuditService(t)
	t.Cleanup(func() { _ = s.Close() })
	cmd := exec.Command(os.Args[0], "--fake", "sleep")
	owned, err := resourceprocess.StartManaged(cmd)
	require(t, err)
	identity := owned.Identity()
	owned.Close()
	_ = cmd.Wait()
	workflow, e := s.workflowRecords(run)
	e.PolicyResources = neutralPolicyReceipt(workflow.ExecutionProfile.Digest, e.Workspace.Worktree, e)
	e.Process = &identity
	setNeutralBinding(t, e, "retained-original-endpoint", "generation", workerFlowMockSession)
	e.CurrentTurnID, e.CurrentCommandID, e.TurnNumber = "00000000-0000-0000-0000-000000000001", newID(), 1
	e.Status, workflow.Status = "active", "running"
	s.refreshRun(run)
	require(t, s.markServiceRecoveryGate())
	result := appServerRecoveryFixture(t, s, run)
	result.dead = true
	result.receipt.State.WorkerAbsent, result.receipt.State.WritersAbsent = true, true
	return s, run, result
}

type stopProofLifecycle struct {
	executionbackend.LifecyclePort
	calls *int
}

func (p stopProofLifecycle) Reconcile(_ context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, executionbackend.LifecycleObserver, error) {
	*p.calls++
	return executionbackend.LifecycleReceipt{Operation: request.Operation, RequestID: request.Operation.ID, Disposition: executionbackend.OperationConfirmed,
		State: executionbackend.LifecycleStateFacts{WorkerAbsent: true, WritersAbsent: true, Stopped: true}, Diagnostic: "original writer tree exited; historical interrupt outcome unknown"}, nil, nil
}

func TestWorkerServiceStopRecovery(t *testing.T) {
	t.Run("frozen_behavior_and_version_reject_changes", func(t *testing.T) {
		s, run := neutralAuditService(t)
		t.Cleanup(func() { _ = s.Close() })
		fixed := *s.workflowRecord(run).ExecutionProfile
		if fixed.ServiceStopBehavior != executionbackend.StopExecutionOnServiceStop || !VerifyFixedExecutionProfile(fixed, s.runtime.Directory) {
			t.Fatal("worker behavior was not frozen")
		}
		for _, value := range []executionbackend.ServiceStopBehavior{"", "edited", executionbackend.ContinueExecutionOnServiceStop} {
			changed := fixed
			changed.ServiceStopBehavior = value
			changed.Digest = fixedExecutionProfileDigest(changed)
			if VerifyFixedExecutionProfile(changed, s.runtime.Directory) {
				t.Fatalf("unsupported worker behavior %q accepted even with a recomputed digest", value)
			}
		}
		fixed.Version--
		fixed.Digest = fixedExecutionProfileDigest(fixed)
		if VerifyFixedExecutionProfile(fixed, s.runtime.Directory) {
			t.Fatal("old frozen worker version was accepted")
		}
	})
	t.Run("original_stop_target_settles_without_repeating_rpc", func(t *testing.T) {
		s, run, result := stoppedWorkerFixture(t)
		e := s.executionRecord(run)
		id, err := s.startWorkerOperation(run, "stop_worker_observation", "worker-stop-observation:"+e.ExecutionID+":"+e.CurrentTurnID, &executionbackend.StopInput{TurnID: e.CurrentTurnID})
		require(t, err)
		require(t, s.setOperationState(id, "outcome_unknown", id, nil, "original connection refused"))
		require(t, s.commitWorker(run, id))
		port, _, err := s.lifecycleForRun(run)
		require(t, err)
		calls := 0
		handled, err := s.reconcileWorkerStops(context.Background(), run, stopProofLifecycle{LifecyclePort: port, calls: &calls})
		require(t, err)
		if !handled || calls != 1 || s.operationByKey("worker-stop-observation:"+e.ExecutionID+":"+e.CurrentTurnID).Status != "confirmed" {
			t.Fatal("original stop receipt was omitted or not committed")
		}
		require(t, s.applyWorkerRecovery(result))
		if s.executionRecord(run).RecoveryState != "interrupted" || run.Decision == nil || len(run.Decision.Request.Options) != 1 || len(s.domain.Attempts) != 1 {
			t.Fatal("verified stop failed to expose one original-session continuation")
		}
		handled, err = s.reconcileWorkerStops(context.Background(), run, stopProofLifecycle{LifecyclePort: port, calls: &calls})
		require(t, err)
		if handled || calls != 1 {
			t.Fatal("confirmed stop was reconciled again")
		}
	})
	t.Run("writer_absence_is_required", func(t *testing.T) {
		s, run, result := stoppedWorkerFixture(t)
		result.receipt.State.WritersAbsent = false
		require(t, s.applyWorkerRecovery(result))
		if s.executionRecord(run).RecoveryState != "unverified" || len(run.Decision.Request.Options) != 0 || len(s.domain.Attempts) != 1 {
			t.Fatal("root exit was treated as proof that all writers exited")
		}
	})
	t.Run("tree_exit_does_not_confirm_unknown_business_start", func(t *testing.T) {
		s, run, result := stoppedWorkerFixture(t)
		result.receipt.Disposition = executionbackend.OperationUnknown
		result.receipt.State.TurnStarted = false
		require(t, s.applyWorkerRecovery(result))
		if s.executionRecord(run).RecoveryState != "unverified" || len(run.Decision.Request.Options) != 0 {
			t.Fatal("worker exit bypassed its unknown business input")
		}
	})
	t.Run("unknown_publication_blocks_and_poll_is_stable", func(t *testing.T) {
		s, run, result := stoppedWorkerFixture(t)
		_, err := s.startWorkerOperation(run, "publish_local_worker", "local-publish:"+run.RunID+":"+result.execution.CurrentTurnID, map[string]string{"run_id": run.RunID})
		require(t, err)
		require(t, s.applyWorkerRecovery(result))
		if s.executionRecord(run).RecoveryState != "unverified" || len(run.Decision.Request.Options) != 0 {
			t.Fatal("unknown publication was omitted from the displayed recovery gate")
		}
		before, events := recordFingerprint(s.domain), len(s.pendingEvents)
		for i := 0; i < 2; i++ {
			require(t, s.applyWorkerRecovery(result))
		}
		if before != recordFingerprint(s.domain) || events != len(s.pendingEvents) {
			t.Fatal("identical unknown probes changed decisions, versions, or activity")
		}
	})
	t.Run("unknown_answer_keeps_original_request_without_recursion", func(t *testing.T) {
		s, run := waitingDecisionService(t)
		t.Cleanup(func() { _ = s.Close() })
		e := s.executionRecord(run)
		e.RecoveryRequired, e.RecoveryState = true, "unverified"
		s.setDecisionStatus(run, "answer_saved")
		id, err := s.startWorkerOperation(run, "reply_execution_question", "worker-question-answer:"+e.ExecutionID+":"+run.Decision.ID, map[string]string{"execution_id": e.ExecutionID})
		require(t, err)
		require(t, s.waitWorkerFlow(run, "original answer delivery remains unknown", id))
		if run.Decision.Status != "answer_saved" || s.operationByKey("worker-question-answer:"+e.ExecutionID+":"+run.Decision.ID).Status != "in_flight" {
			t.Fatal("unknown original answer was replaced or resent")
		}
	})
}

// Recovery can validate generic identity/epochs, but native resource facts must
// remain in the worker implementation. This also guards dead-observer fallbacks.
func TestWorkerServiceStopBoundary(t *testing.T) {
	for _, path := range []string{"service_shutdown.go", "worker_lifecycle_recovery.go", "worker_recovery.go", "reliable/recovery_policy.go"} {
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		require(t, err)
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if selector, ok := call.Fun.(*ast.SelectorExpr); ok && (selector.Sel.Name == "InspectProcess" || selector.Sel.Name == "WorkerTreeAbsent" || strings.Contains(selector.Sel.Name, "AppServer")) {
				t.Errorf("%s directly calls implementation-specific recovery %s", path, selector.Sel.Name)
			}
			return true
		})
	}
}

func TestWorkerLateReplyReleasesOnlyItsOriginalUncertainty(t *testing.T) {
	for _, remainingPublication := range []bool{false, true} {
		t.Run(map[bool]string{false: "reply_resolved", true: "publication_still_unknown"}[remainingPublication], func(t *testing.T) {
			s, run := neutralAuditService(t)
			t.Cleanup(func() { _ = s.Close() })
			workflow, e := s.workflowRecords(run)
			e.CurrentTurnID, e.CurrentCommandID = "00000000-0000-0000-0000-000000000001", newID()
			e.Status, workflow.Status = "active", "waiting_input"
			setNeutralBinding(t, e, "retained-endpoint", "generation", workerFlowMockSession)
			e.PolicyResources = neutralPolicyReceipt(workflow.ExecutionProfile.Digest, e.Workspace.Worktree, e)
			fixed := *workerSessionContext(run, e)
			fixed.ProjectID = s.Config.identity
			request := neutralDecisionFixture(t, fixed, "original-call", []executionbackend.Question{{ID: "choice", Header: "Choice", Question: "A?", AllowFreeText: true}})
			decision, err := WriteDecisionRequest(run, request)
			require(t, err)
			s.setRunDecision(run, decision)
			record := s.decisionRecord(run)
			record.Status = "answer_saved"
			answer := executionmodel.DecisionAnswer{DecisionID: record.DecisionID, RunID: run.RunID, Answers: map[string][]string{"choice": {"A"}}, AnsweredBy: "operator", AnsweredAt: now()}
			record.Answer = mustJSON(answer)
			s.refreshRun(run)
			replyID, err := s.startWorkerOperation(run, "reply_execution_question", "worker-question-answer:"+e.ExecutionID+":"+run.Decision.ID, &executionbackend.FeedbackInput{DecisionID: run.Decision.ID, QuestionID: request.QuestionRoute.QuestionID, Answer: &executionbackend.Answer{Answers: answer.Answers}})
			require(t, err)
			require(t, s.setOperationState(replyID, "outcome_unknown", request.QuestionRoute.QuestionID, nil, "response lost"))
			require(t, s.commitWorker(run, replyID))
			if remainingPublication {
				_, err = s.startWorkerOperation(run, "publish_local_worker", "local-publish:"+run.RunID+":"+e.CurrentTurnID, map[string]string{"run_id": run.RunID})
				require(t, err)
			}
			e.RecoveryRequired, e.RecoveryState = true, "unverified"
			fail := true
			s.lifecycleResolver = transientQualificationResolver{fail: &fail}
			require(t, s.commitWorker(run))
			beforeAttempts := len(s.domain.Attempts)
			success := true
			event := WorkerEvent{Fact: executionbackend.EventFact{ToolReply: &executionbackend.ToolReplyFact{CallID: request.QuestionRoute.QuestionID, ThreadID: e.SessionID, TurnID: e.CurrentTurnID, Status: "completed", Success: &success, Answers: []map[string][]string{answer.Answers}}}}
			require(t, s.eventWorkerFlowFor(run, event))
			if e.RecoveryState != "unverified" {
				t.Fatal("failed worker qualification was bypassed")
			}
			s.workers[run.RunID] = &Worker{}
			require(t, s.resumeWorkerFlow(context.Background()))
			expected := "running"
			if remainingPublication {
				expected = "unverified"
			}
			if e.RecoveryState != expected || s.operationByKey("worker-question-answer:"+e.ExecutionID+":"+record.DecisionID).Status != "confirmed" || len(s.domain.Attempts) != beforeAttempts {
				t.Fatalf("late original reply qualification: state=%s want=%s", e.RecoveryState, expected)
			}
		})
	}
}

type transientQualificationLifecycle struct {
	*neutralLifecycle
	fail *bool
}

func (p transientQualificationLifecycle) VerifyManualRecovery(ctx context.Context, snapshot executionbackend.LifecycleSnapshot) (executionbackend.LifecycleRecoveryEvidence, error) {
	if *p.fail {
		*p.fail = false
		return executionbackend.LifecycleRecoveryEvidence{}, errors.New("temporary observation failure")
	}
	return p.neutralLifecycle.VerifyManualRecovery(ctx, snapshot)
}

type transientQualificationResolver struct{ fail *bool }

func (r transientQualificationResolver) ResolveLifecycle(setup executionbackend.LifecycleSetup) (executionbackend.LifecyclePort, error) {
	return transientQualificationLifecycle{neutralLifecycle: &neutralLifecycle{Setup: setup}, fail: r.fail}, nil
}

type unavailableOwnerResolver struct{}

func (unavailableOwnerResolver) ResolveLifecycle(executionbackend.LifecycleSetup) (executionbackend.LifecyclePort, error) {
	return nil, errors.New("later resolver failure")
}
func TestWorkerPreparationStopDoesNotJournalUnsentBusinessTurn(t *testing.T) {
	s, run := neutralAuditService(t)
	t.Cleanup(func() { _ = s.Close() })
	_, e := s.workflowRecords(run)
	e.Workspace = executionmodel.WorkspaceEvidence{Repository: s.Config.identity, Worktree: run.WorkspaceIntent.Path, Branch: run.WorkspaceIntent.Branch, Base: run.WorkspaceIntent.BaseCommit}
	e.PolicyResources = neutralPolicyReceipt(s.workflowRecord(run).ExecutionProfile.Digest, e.Workspace.Worktree, e)
	cmd := exec.Command(os.Args[0], "--fake", "sleep")
	owned, err := resourceprocess.StartManaged(cmd)
	require(t, err)
	defer cmd.Wait()
	defer owned.Close()
	input := &executionbackend.TurnInput{CommandID: newID(), Prompt: "original input not yet dispatched", Purpose: "business"}
	e.PendingCommandID = input.CommandID
	_, err = s.startWorkerOperation(run, "prepare_worker_session", "worker-session:"+e.ExecutionID+":"+input.CommandID, input)
	require(t, err)
	fixed, err := s.workerRecords(run)
	require(t, err)
	request, err := s.originalLifecycleRequest(run, fixed)
	require(t, err)
	// Cache the legitimate preparation owner before its actual process receipt.
	_, _, err = s.lifecycleForRun(run)
	require(t, err)
	identity := owned.Identity()
	receipt := executionbackend.LifecycleReceipt{Operation: request.Operation, Disposition: executionbackend.OperationConfirmed, Process: &identity, Handle: owned, State: executionbackend.LifecycleStateFacts{SessionID: workerFlowMockSession, SessionGeneration: "original-session-generation"}}
	s.State.Service.Status = "stopping"
	_, proceed, err := s.applyPreparedSessionReceipt(run, request, receipt, nil)
	require(t, err)
	if proceed || s.operationByKey("worker-turn:"+e.ExecutionID+":"+input.CommandID) != nil {
		t.Fatal("stopping preparation created an unsent in-flight business turn")
	}
	s.lifecycleResolver = unavailableOwnerResolver{}
	if err := s.shutdown(); err == nil {
		t.Fatal("resolver failure was not reported")
	}
	deadline := time.Now().Add(5 * time.Second)
	for resourceprocess.InspectProcess(&identity) != "process absent" && !owned.WritersAbsent() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if resourceprocess.InspectProcess(&identity) != "process absent" && !owned.WritersAbsent() {
		t.Fatal("accepted preparation owner was lost after resolver failure")
	}
}

type shutdownAckPort struct {
	executionbackend.LifecyclePort
	entered, release chan struct{}
}

func (p shutdownAckPort) Execute(ctx context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, executionbackend.LifecycleObserver, error) {
	close(p.entered)
	<-p.release
	return executionbackend.LifecycleReceipt{Operation: request.Operation, Disposition: executionbackend.OperationConfirmed}, nil, ctx.Err()
}

func TestWorkerShutdownDrainsCommittedDeliveryAck(t *testing.T) {
	s, run := waitingDecisionService(t)
	t.Cleanup(func() { _ = s.Close() })
	e := s.executionRecord(run)
	e.PendingDeliveryID = "original-committed-delivery"
	require(t, s.commitWorker(run))
	port, fixed, err := s.lifecycleForRun(run)
	require(t, err)
	input := executionbackend.LifecycleInput{Ack: &executionbackend.AcknowledgeInput{DeliveryID: e.PendingDeliveryID}}
	op, _, err := s.originalLifecycleOperation(run, string(executionbackend.LifecycleAcknowledge), "worker-delivery-ack:"+e.ExecutionID+":"+e.PendingDeliveryID, input.Ack)
	require(t, err)
	snapshot, err := s.lifecycleSnapshot(run, fixed.workflow, fixed.execution, fixed.attempt, nil)
	require(t, err)
	request, err := checkedLifecycleRequest(snapshot, op, executionbackend.LifecycleAcknowledge, input)
	require(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.runContext, s.signalQueue = ctx, make(chan Signal, 1)
	entered, release := make(chan struct{}), make(chan struct{})
	applied := false
	err = s.dispatchLifecycleEffect(context.Background(), run, shutdownAckPort{port, entered, release}, request, false, func(_ *reconstructiontypes.RunView, result lifecycleEffectResult) error {
		if result.err != nil {
			return result.err
		}
		applied = true
		return s.setOperationState(op.OperationID, "confirmed", "original-committed-delivery", mustJSON(result.receipt), "")
	})
	if !errors.Is(err, errLifecycleEffectPending) {
		t.Fatal(err)
	}
	<-entered
	cancel()
	close(release)
	s.State.Service.Status = "stopping"
	require(t, s.shutdown())
	if !applied || s.operationByKey(op.IdempotencyKey).Status != "confirmed" {
		t.Fatal("service cancellation lost the committed message's acknowledgement receipt")
	}
}
