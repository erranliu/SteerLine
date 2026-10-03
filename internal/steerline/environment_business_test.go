package steerline

import (
	"context"
	"errors"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"testing"
	"time"
)

func answerNeutralEnvironment(t *testing.T, s *Service, run *Run, option, note string) {
	t.Helper()
	decision := run.Decision
	if decision == nil {
		t.Fatal("no host decision")
	}
	s.State.Service.Ready = true
	require(t, s.publishCheckpoint())
	answer := DecisionAnswer{DecisionID: decision.ID, RunID: run.RunID, OptionID: option, Note: note, AnsweredBy: "operator", ExpectedDecisionVersion: decision.Version, ExpectedRunRevision: run.ControlRevision, ServiceInstanceID: s.State.Service.InstanceID, SourceAttemptID: run.AttemptID, ExpectedRecoveryVersion: run.RecoveryVersion}
	commands := CommandService{Store: s.domainStore, Project: s.Config.Identity()}
	command, _, err := commands.AnswerDecisionWithID(context.Background(), newID(), decision.ID, decision.Version, decisionAnswerKey(answer), answer.AnsweredBy, mustJSON(answer))
	require(t, err)
	require(t, s.processCommandsAndDecisions(context.Background()))
	saved, err := s.domainStore.CommandByIdempotency(context.Background(), s.Config.Identity(), command.IdempotencyKey)
	require(t, err)
	if saved.Status != "accepted" {
		t.Fatalf("host action rejected: %s / %s", saved.Status, saved.Result)
	}
}
func neutralPendingPreparation(t *testing.T) (*Service, *Run, *neutralPreparationResolver) {
	t.Helper()
	s, run, resolver := neutralPreparationService(t, "success")
	resolver.turn.Settled = false
	resolver.turn.Outcome = ""
	return s, run, resolver
}
func TestWorkerEnvironmentUnidentifiedAgentKeepsOriginalDeadline(t *testing.T) {
	s, run, resolver := neutralPendingPreparation(t)
	e := s.executionRecord(run)
	original := *e.Environment.OriginalTerminal
	e.Process = nil
	nonce := e.Environment.Preparation.Nonce
	e.Environment.Preparation.Deadline = now().Add(-time.Second)
	deadline := e.Environment.Preparation.Deadline
	require(t, s.commitWorker(run))
	require(t, s.advanceWorkerPreparation(context.Background(), run))
	if e.Environment.Blocker == nil || e.Environment.Blocker.Fact.Reason != "preparation_unverified" || len(resolver.inputs) != 0 || e.Environment.Preparation.Nonce != nonce || !e.Environment.Preparation.Deadline.Equal(deadline) || *e.Environment.OriginalTerminal != original {
		t.Fatal("unidentified startup bypassed/reset its original deadline or dispatched work")
	}
}
func TestWorkerEnvironmentER2HostWaitPreservesQuestionAndPR(t *testing.T) {
	s, run, resolver := neutralPreparationService(t, "success")
	e := s.executionRecord(run)
	require(t, s.finishWorkerPreparation(context.Background(), run, e, &resolver.turn, "succeeded"))
	worker := s.workers[run.RunID]
	require(t, s.eventForRun(run, <-worker.Events))
	workflow, e := s.workflowRecords(run)
	resolver.turn = contracts.HostTurnFacts{TurnID: e.CurrentTurnID, TaskID: "fixture-business", Purpose: "business", HasDispatch: true, Deadline: now().Add(2 * time.Minute)}
	resolver.questionPending = true
	fixed := workerSessionContext(run, e)
	question, err := WriteDecisionRequest(run, neutralDecisionFixture(t, *fixed, "original-question", []executionapi.Question{{ID: "business", Question: "Business question", AllowFreeText: true}}))
	require(t, err)
	s.setRunDecision(run, question)
	require(t, s.commitWorker(run))
	resolver.fact = contracts.HostFact{Reason: "host_wait", Confidence: "suspected", Source: "fixture-host", Turn: e.CurrentTurnID}
	require(t, s.observeWorkerEnvironment(context.Background(), run))
	if e.Environment.Blocker == nil || e.Environment.Blocker.Fact.Confidence != "suspected" || workflow.Phase != "waiting_decision" || e.Environment.DeferredDecisionID != question.ID {
		t.Fatal("host evidence or original question lost")
	}
	count := len(s.State.Decisions)
	require(t, s.observeWorkerEnvironment(context.Background(), run))
	if len(s.State.Decisions) != count || len(resolver.feedback) != 0 {
		t.Fatal("host wait created duplicate decision or injected input")
	}
	note := "  保留原问题，不要重派任务。  "
	answerNeutralEnvironment(t, s, run, "", note)
	workflow, e = s.workflowRecords(run)
	if len(resolver.feedback) != 0 || len(e.Environment.DeferredFeedback) != 1 || run.Decision.Status != "pending" {
		t.Fatal("host wait sent deferred feedback or blocked further center actions")
	}
	resolver.fact = contracts.HostFact{}
	answerNeutralEnvironment(t, s, run, "recheck_environment", "")
	e = s.executionRecord(run)
	if len(resolver.feedback) != 1 || resolver.feedback[0] != note || len(e.Environment.DeferredFeedback) != 0 || run.Decision == nil || run.Decision.Request.QuestionRoute == nil || run.Decision.Request.QuestionRoute.QuestionID != "original-question" {
		t.Fatalf("deferred feedback lost original text/question: feedback=%q queue=%+v decision=%+v", resolver.feedback, e.Environment.DeferredFeedback, run.Decision)
	}
	workflow.Phase = "waiting_pr_merge"
	e.Environment.Blocker = nil
	resolver.fact = contracts.HostFact{Reason: "host_wait", Confidence: "suspected", Source: "fixture-host"}
	require(t, s.commitWorker(run))
	require(t, s.observeWorkerEnvironment(context.Background(), run))
	if workflow.Phase != "waiting_pr_merge" {
		t.Fatal("late host diagnostic reopened PR merge waiting")
	}
}
func TestWorkerEnvironmentER3RecheckKeepsOriginalDispatch(t *testing.T) {
	s, run, resolver := neutralPendingPreparation(t)
	e := s.executionRecord(run)
	original := *e.Process
	turn := resolver.turn
	nonce, deadline := e.Environment.Preparation.Nonce, e.Environment.Preparation.Deadline
	resolver.fact = contracts.HostFact{Reason: "host_wait", Confidence: "confirmed", Source: "fixture-host", Turn: turn.TurnID}
	require(t, s.observeWorkerEnvironment(context.Background(), run))
	answerNeutralEnvironment(t, s, run, "recheck_environment", "")
	answerNeutralEnvironment(t, s, run, "recheck_environment", "")
	if len(resolver.inputs) != 0 {
		t.Fatal("repeated recheck duplicated original probe")
	}
	resolver.fact = contracts.HostFact{}
	answerNeutralEnvironment(t, s, run, "recheck_environment", "")
	e = s.executionRecord(run)
	if *e.Process != original || resolver.turn != turn || len(resolver.inputs) != 0 || e.Environment.Preparation.Nonce != nonce || !e.Environment.Preparation.Deadline.Equal(deadline) {
		t.Fatal("recheck replaced original resource, input, nonce or deadline")
	}
	resolver.fact = contracts.HostFact{Reason: "identity_mismatch", Confidence: "confirmed", Source: "fixture-original-identity", Turn: turn.TurnID}
	require(t, s.observeWorkerEnvironment(context.Background(), run))
	if e.Environment.Blocker == nil || e.Environment.Blocker.Fact.Reason != "identity_mismatch" {
		t.Fatal("replacement identity accepted")
	}
}
func TestWorkerEnvironmentRestartRequiresConfirmationBeforeBusinessInput(t *testing.T) {
	s, run, resolver := neutralPendingPreparation(t)
	config := s.Config
	id := run.RunID
	require(t, s.Close())
	reopened, err := newNeutralService(config)
	require(t, err)
	defer reopened.Close()
	reopened.lifecycleResolver = resolver
	run = reopened.find("8")
	if run == nil || run.RunID != id {
		t.Fatal("restart lost original run")
	}
	require(t, reopened.markServiceRecoveryGate())
	e := reopened.executionRecord(run)
	resolver.turn.Settled = true
	resolver.turn.Outcome = "succeeded"
	require(t, reopened.finishWorkerPreparation(context.Background(), run, e, &resolver.turn, "succeeded"))
	if !e.RecoveryRequired || !e.Environment.Preparation.Verified || len(resolver.inputs) != 0 || e.TurnNumber != 0 || e.Environment.Blocker == nil || e.Environment.Blocker.Fact.Reason != "service_recovery_required" {
		t.Fatal("restart sent business input or lost the actionable manual gate")
	}
	answerNeutralEnvironment(t, reopened, run, "recheck_environment", "")
	e = reopened.executionRecord(run)
	if e.RecoveryRequired || !e.Environment.Ready || len(resolver.inputs) != 1 {
		t.Fatal("fresh manual confirmation did not continue original execution exactly once")
	}
	reopened.shutdown()
}
func TestWorkerEnvironmentER4DeadlineRestartAndLostReceipt(t *testing.T) {
	s, run, resolver := neutralPendingPreparation(t)
	e := s.executionRecord(run)
	nonce, deadline := e.Environment.Preparation.Nonce, e.Environment.Preparation.Deadline
	original := *e.Process
	// An unknown original side effect is reconciled by its exact saved request.
	request := executionapi.TurnInput{CommandID: resolver.turn.TurnID, Prompt: "original fixture preparation"}
	resolver.loseReceipt = true
	lifecycle, _, err := s.lifecycleForRun(run)
	require(t, err)
	candidate := executionapi.LifecycleCandidate{Action: executionapi.LifecyclePrepareSession, Input: executionapi.LifecycleInput{Turn: &request}}
	err = s.executeLifecycleStep(context.Background(), run, lifecycle, candidate)
	if !errors.Is(err, errLifecycleNeedsReconciliation) {
		t.Fatalf("lost receipt did not retain unknown intent: %v", err)
	}
	originalOp := resolver.originalRequest.Operation.ID
	digest := resolver.originalRequest.Operation.PayloadDigest
	require(t, s.executeLifecycleStep(context.Background(), run, lifecycle, candidate))
	op := snapshotOperation(&s.domain, originalOp)
	if op == nil || op.Status != "confirmed" || op.PayloadDigest != digest || resolver.reconciles != 1 || len(resolver.inputs) != 0 {
		t.Fatal("original receipt reconciliation changed request or reinjected input")
	}
	config := s.Config
	require(t, s.Close())
	reopened, err := newNeutralService(config)
	require(t, err)
	defer reopened.Close()
	reopened.lifecycleResolver = resolver
	run = reopened.find("8")
	e = reopened.executionRecord(run)
	if e.Environment.Preparation.Nonce != nonce || !e.Environment.Preparation.Deadline.Equal(deadline) || *e.Process != original {
		t.Fatal("restart reset original preparation/resource")
	}
	e.Environment.Preparation.Deadline = now().Add(-time.Second)
	require(t, reopened.commitWorker(run))
	require(t, reopened.observeWorkerEnvironment(context.Background(), run))
	if e.Environment.Blocker == nil || e.Environment.Blocker.Fact.Reason != "preparation_unverified" {
		t.Fatal("expired preparation has no actionable decision")
	}
	resolver.turn.Settled = true
	resolver.turn.Outcome = "succeeded"
	require(t, reopened.finishWorkerPreparation(context.Background(), run, e, &resolver.turn, "succeeded"))
	if len(resolver.inputs) != 0 || !e.Environment.Preparation.Verified {
		t.Fatal("late success auto-dispatched or lost complete facts")
	}
	answerNeutralEnvironment(t, reopened, run, "recheck_environment", "")
	e = reopened.executionRecord(run)
	if len(resolver.inputs) != 1 || !e.Environment.Ready {
		t.Fatal("fresh recheck did not accept original complete facts")
	}
	business := resolver.inputs[0]
	resolver.turn = contracts.HostTurnFacts{TurnID: business, TaskID: "fixture-business", Purpose: "business", HasDispatch: true, Deadline: now().Add(-time.Second), IntentAt: now().Add(-3 * time.Minute)}
	require(t, reopened.observeWorkerEnvironment(context.Background(), run))
	if e.Environment.Blocker == nil || e.Environment.Blocker.Fact.Reason != "business_start_unverified" {
		t.Fatal("business start deadline lost original input")
	}
	worker := reopened.workers[run.RunID]
	require(t, reopened.eventForRun(run, <-worker.Events))
	answerNeutralEnvironment(t, reopened, run, "recheck_environment", "")
	e = reopened.executionRecord(run)
	if e.Environment.Blocker != nil || reopened.workflowRecord(run).Phase != "executing" || len(resolver.inputs) != 1 || e.CurrentTurnID != business {
		t.Fatal("business start reconciliation repeated or replaced input")
	}
	reopened.shutdown()
}
func TestEnvironmentRecheckCannotClearNewFaultObservedDuringPoll(t *testing.T) {
	s, run, resolver := neutralPendingPreparation(t)
	resolver.fact = contracts.HostFact{Reason: "host_wait", Confidence: "confirmed", Source: "fixture-host"}
	require(t, s.observeWorkerEnvironment(context.Background(), run))
	e := s.executionRecord(run)
	old := e.Environment.Blocker.ID
	resolver.fact = contracts.HostFact{}
	resolver.newFault = true
	answerNeutralEnvironment(t, s, run, "recheck_environment", "")
	e = s.executionRecord(run)
	if e.Environment.Blocker == nil || e.Environment.Blocker.ID == old || e.Environment.Blocker.Fact.Reason != "tool_initialization_failed" || run.Decision == nil || run.Decision.Status != "pending" || len(resolver.inputs) != 0 {
		t.Fatalf("old recheck cleared newer fault: %+v / %+v", e.Environment, run.Decision)
	}
}
