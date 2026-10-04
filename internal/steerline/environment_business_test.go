package steerline

import (
	"context"
	"errors"
	"testing"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/steerline/clientcontracts"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func answerNeutralEnvironment(t *testing.T, s *Service, run *Run, option, note string) {
	t.Helper()
	if option != "open_terminal" && option != "recheck_environment" {
		t.Fatal("environment commands do not accept business text")
	}
	s.State.Service.Ready = true
	require(t, s.publishCheckpoint())
	target, err := clientcontracts.EnvironmentTarget(run)
	require(t, err)
	req := productapi.ControlRequest{RequestID: newID(), Action: option, RequestedBy: "operator", RunID: run.RunID}
	payload := productapi.ControlPayload{Environment: target}
	op := &ControlOperation{RequestID: req.RequestID, Action: option, RunID: run.RunID}
	err = s.applyControlAction(context.Background(), req, payload, op, &productapi.ControlReceipt{})
	var rejected *taskmodel.Error
	if err != nil && !(errors.As(err, &rejected) && rejected.Code == "environment_unverified") {
		require(t, err)
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

func TestSimplifiedEnvironmentSavedInputRequiresFreshAnswer(t *testing.T) {
	for _, hasInputIntent := range []bool{false, true} {
		t.Run(map[bool]string{false: "unsent_cancelled", true: "unknown_original_retained"}[hasInputIntent], func(t *testing.T) {
			s, run, resolver := neutralPreparationService(t, "success")
			e := s.executionRecord(run)
			require(t, s.finishWorkerPreparation(context.Background(), run, e, &resolver.turn, "succeeded"))
			require(t, s.eventForRun(run, <-s.workers[run.RunID].Events))
			resolver.turn = executionbackend.HostTurnFacts{TurnID: e.CurrentTurnID, Purpose: "business", HasDispatch: true, Deadline: now().Add(time.Minute)}
			question, err := WriteDecisionRequest(run, executionmodel.DecisionRequest{Question: "Review original work.", AllowFreeText: true})
			require(t, err)
			s.setRunDecision(run, question)
			require(t, s.waitWorkerFlow(run, "Review original work."))
			s.State.Service.Ready = true
			require(t, s.publishCheckpoint())
			oldID := run.Decision.ID
			originalRequest := recordFingerprint(run.Decision.Request)
			answer := executionmodel.DecisionAnswer{DecisionID: oldID, RunID: run.RunID, Note: "fresh text", AnsweredBy: "operator", ExpectedDecisionVersion: run.Decision.Version, ExpectedRunRevision: run.ControlRevision, ServiceInstanceID: s.State.Service.InstanceID, SourceAttemptID: run.AttemptID, ExpectedRecoveryVersion: run.RecoveryVersion}
			commands := newTestCommandService(s.commandStore, s.Config.Identity())
			command, _, err := commands.AnswerDecisionWithID(context.Background(), newID(), oldID, run.Decision.Version, decisionAnswerKey(answer), answer.AnsweredBy, mustJSON(answer))
			require(t, err)
			_, _, err = commands.AcceptAnswer(context.Background(), command, answer)
			require(t, err)
			require(t, s.refreshNativeSnapshot(context.Background()))
			run = s.find(run.RunID)
			if hasInputIntent {
				_, err := s.startWorkerOperation(run, "start_worker_turn", "worker-turn:"+run.ExecutionID+":"+command.CommandID, executionbackend.TurnInput{CommandID: command.CommandID, Prompt: answer.Note})
				require(t, err)
			}
			resolver.fact = executionbackend.HostFact{Reason: "host_wait", Confidence: "confirmed", Source: "fixture-host"}
			require(t, s.observeWorkerEnvironment(context.Background(), run))
			stored, err := s.commandStore.Command(context.Background(), s.Config.Identity(), command.CommandID)
			require(t, err)
			if hasInputIntent {
				if run.Decision.ID != oldID || run.Decision.Status != "answer_saved" || stored.Status != "accepted" {
					t.Fatal("blocker discarded an input whose original outcome is unknown")
				}
				return
			}
			if stored.Status != "rejected" || run.Decision.ID == oldID || run.Decision.Status != "pending" || recordFingerprint(run.Decision.Request) != originalRequest {
				t.Fatalf("unsent answer retained or original question changed: %+v / %+v", stored, run.Decision)
			}
			resolver.fact = executionbackend.HostFact{}
			answerNeutralEnvironment(t, s, run, "recheck_environment", "")
			require(t, s.applyDecisions(context.Background()))
			if len(resolver.feedback) != 0 || len(resolver.inputs) != 1 || run.Decision.Status != "pending" {
				t.Fatal("recheck delivered the cancelled answer")
			}
		})
	}
}

func TestWorkerEnvironmentER2HostWaitPreservesQuestionAndPR(t *testing.T) {
	s, run, resolver := neutralPreparationService(t, "success")
	e := s.executionRecord(run)
	require(t, s.finishWorkerPreparation(context.Background(), run, e, &resolver.turn, "succeeded"))
	worker := s.workers[run.RunID]
	require(t, s.eventForRun(run, <-worker.Events))
	workflow, e := s.workflowRecords(run)
	resolver.turn = executionbackend.HostTurnFacts{TurnID: e.CurrentTurnID, TaskID: "fixture-business", Purpose: "business", HasDispatch: true, Deadline: now().Add(2 * time.Minute)}
	resolver.questionPending = true
	fixed := workerSessionContext(run, e)
	question, err := WriteDecisionRequest(run, neutralDecisionFixture(t, *fixed, "original-question", []executionbackend.Question{{ID: "business", Question: "Business question", AllowFreeText: true}}))
	require(t, err)
	s.setRunDecision(run, question)
	require(t, s.commitWorker(run))
	resolver.fact = executionbackend.HostFact{Reason: "host_wait", Confidence: "suspected", Source: "fixture-host", Turn: e.CurrentTurnID}
	require(t, s.observeWorkerEnvironment(context.Background(), run))
	if e.Environment.Blocker == nil || e.Environment.Blocker.Fact.Confidence != "suspected" || workflow.Phase != "waiting_decision" || run.Decision == nil || run.Decision.ID != question.ID {
		t.Fatal("host evidence or original question lost")
	}
	count := len(s.State.Decisions)
	require(t, s.observeWorkerEnvironment(context.Background(), run))
	if len(s.State.Decisions) != count || len(resolver.feedback) != 0 {
		t.Fatal("host wait created duplicate decision or injected input")
	}

	note := "保留原问题，不要重派任务。"
	answer := executionmodel.DecisionAnswer{DecisionID: question.ID, RunID: run.RunID, Note: note, Answers: map[string][]string{"business": {note}}, AnsweredBy: "operator", ExpectedDecisionVersion: run.Decision.Version, ExpectedRunRevision: run.ControlRevision, ServiceInstanceID: s.State.Service.InstanceID, SourceAttemptID: run.AttemptID, ExpectedRecoveryVersion: run.RecoveryVersion}
	commands := newTestCommandService(s.commandStore, s.Config.Identity())
	command, _, err := commands.AnswerDecisionWithID(context.Background(), newID(), question.ID, question.Version, decisionAnswerKey(answer), answer.AnsweredBy, mustJSON(answer))
	require(t, err)
	require(t, s.processCommandsAndDecisions(context.Background()))
	refused, err := s.commandStore.Command(context.Background(), s.Config.Identity(), command.CommandID)
	require(t, err)
	if refused.Status != "rejected" || len(resolver.feedback) != 0 || run.Decision.ID != question.ID || run.Decision.Status != "pending" {
		t.Fatal("blocked feedback was queued/sent or replaced the original question")
	}
	resolver.fact = executionbackend.HostFact{}
	answerNeutralEnvironment(t, s, run, "recheck_environment", "")
	e = s.executionRecord(run)
	if len(resolver.feedback) != 0 || run.Decision == nil || run.Decision.Request.QuestionRoute == nil || run.Decision.Request.QuestionRoute.QuestionID != "original-question" {
		t.Fatal("recheck injected refused feedback or lost the original question")
	}
	workflow, _ = s.workflowRecords(run)
	workflow.Phase = "waiting_pr_merge"
	e.Environment.Blocker = nil
	resolver.fact = executionbackend.HostFact{Reason: "host_wait", Confidence: "suspected", Source: "fixture-host"}
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
	resolver.fact = executionbackend.HostFact{Reason: "host_wait", Confidence: "confirmed", Source: "fixture-host", Turn: turn.TurnID}
	require(t, s.observeWorkerEnvironment(context.Background(), run))
	answerNeutralEnvironment(t, s, run, "recheck_environment", "")
	answerNeutralEnvironment(t, s, run, "recheck_environment", "")
	if len(resolver.inputs) != 0 {
		t.Fatal("repeated recheck duplicated original probe")
	}
	resolver.fact = executionbackend.HostFact{}
	answerNeutralEnvironment(t, s, run, "recheck_environment", "")
	e = s.executionRecord(run)
	if *e.Process != original || resolver.turn != turn || len(resolver.inputs) != 0 || e.Environment.Preparation.Nonce != nonce || !e.Environment.Preparation.Deadline.Equal(deadline) {
		t.Fatal("recheck replaced original resource, input, nonce or deadline")
	}
	resolver.fact = executionbackend.HostFact{Reason: "identity_mismatch", Confidence: "confirmed", Source: "fixture-original-identity", Turn: turn.TurnID}
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
	request := executionbackend.TurnInput{CommandID: resolver.turn.TurnID, Prompt: "original fixture preparation"}
	resolver.loseReceipt = true
	lifecycle, _, err := s.lifecycleForRun(run)
	require(t, err)
	candidate := executionbackend.LifecycleCandidate{Action: executionbackend.LifecyclePrepareSession, Input: executionbackend.LifecycleInput{Turn: &request}}
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
		t.Fatal("expired preparation has no actionable blocker")
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
	resolver.turn = executionbackend.HostTurnFacts{TurnID: business, TaskID: "fixture-business", Purpose: "business", HasDispatch: true, Deadline: now().Add(-time.Second), IntentAt: now().Add(-3 * time.Minute)}
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
	resolver.fact = executionbackend.HostFact{Reason: "host_wait", Confidence: "confirmed", Source: "fixture-host"}
	require(t, s.observeWorkerEnvironment(context.Background(), run))
	e := s.executionRecord(run)
	old := e.Environment.Blocker.ID
	resolver.fact = executionbackend.HostFact{}
	resolver.newFault = true
	answerNeutralEnvironment(t, s, run, "recheck_environment", "")
	e = s.executionRecord(run)
	if e.Environment.Blocker == nil || e.Environment.Blocker.ID == old || e.Environment.Blocker.Fact.Reason != "tool_initialization_failed" || run.Decision != nil || len(resolver.inputs) != 0 {
		t.Fatalf("old recheck cleared newer fault: %+v / %+v", e.Environment, run.Decision)
	}
}
