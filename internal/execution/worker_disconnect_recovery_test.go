package execution

import (
"github.com/erranliu/SteerLine/internal/execution/reliable"
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"

	b "github.com/erranliu/SteerLine/internal/execution/backend"
	m "github.com/erranliu/SteerLine/internal/execution/model"
)

func TestWorkerRecoveryFailedReportFeedbackReverifiesImplementation(t *testing.T) {
	for _, option := range []string{"continue", ""} {
		t.Run(map[bool]string{true: "continue", false: "free_text"}[option != ""], func(t *testing.T) {
			s, run, stage := workerStageFixture(t)
			t.Cleanup(func() { _ = s.Close() })
			require(t, acceptTestStage(t, s, run, stage, "implemented", true))
			startQueuedStage(t, s, run)
			w, e := s.workflowRecords(run)
			op, err := s.startWorkerOperation(run, "start_worker_turn", "worker-turn:"+e.ExecutionID+":"+e.CurrentCommandID, b.TurnInput{CommandID: e.CurrentCommandID, Prompt: w.StageInputs[1].Prompt})
			require(t, err)
			require(t, s.setOperationState(op, "confirmed", e.CurrentTurnID, nil, ""))
			require(t, s.commitWorker(run, op))
			require(t, acceptTestStage(t, s, run, stage, "", false))
			request, decisionID := run.Decision.Request, run.Decision.ID
			// The shared feedback entry receives an already qualified, authorized
			// original resource. Both answer shapes must choose fresh implement.
			e.RecoveryRequired, e.RecoveryState = false, "completed"
			require(t, s.commitWorker(run))
			if out, err := exec.Command("git", "-C", s.Config.Repository, "worktree", "add", "-b", e.Workspace.Branch, e.Workspace.Worktree, "HEAD").CombinedOutput(); err != nil {
				t.Fatalf("prepare owned worktree: %v %s", err, out)
			}
			command := newID()
			require(t, s.startStageFeedback(context.Background(), run, m.DecisionAnswer{DecisionID: decisionID, OptionID: option, Note: "verify the existing work"}, command, request))
			input := stageInput(s.workflowRecord(run), command)
			if input == nil || input.StepID != m.StepImplement || input.SourceCommandID != "" || input.FeedbackDecisionID != decisionID {
				t.Fatal("failed report feedback bypassed implementation verification")
			}
		})
	}
}

type recoveryStagePort struct {
	*stageTestPort
	recovery b.LifecycleRecoveryEvidence
}

type pendingResultPort struct{ *stageTestPort }

func TestWorkerRecoveryObservationWaitShowsReasonWithoutSettlingTurn(t *testing.T) {
	s, run, _ := workerStageFixture(t)
	e := s.executionRecord(run)
	diagnostic := "original native observation is unavailable"
	require(t, s.eventWorkerFlowFor(run, WorkerEvent{RunID: run.RunID, AttemptID: run.AttemptID, InputWait: true, Diagnostic: diagnostic,
		Activity: m.ActivityEvent{Source: "fixture", Kind: "execution", Status: "observing", Title: "Unverified original observation"},
		Fact:     b.EventFact{Kind: b.EventStatus, Source: "fixture", SourceEventID: "original-observation-wait", SourceIdentity: e.SessionID, SourceSegment: e.CurrentCommandID, SessionID: e.SessionID}}))
	if e.Error == nil || !strings.Contains(*e.Error, diagnostic) || e.CurrentTurnID == e.LastStoppedTurnID || len(s.workflowRecord(run).StageResults) != 0 {
		t.Fatal("observation gap was hidden or fabricated a completed original result")
	}
}

func TestWorkerRecoveryCompletionDoesNotInventSessionUnavailability(t *testing.T) {
	s, run, stage := workerStageFixture(t)
	e := s.executionRecord(run)
	e.RecoveryRequired, e.RecoveryState, e.RecoveryVersion = true, "completed", 2
	e.RecoverySessionAvailable = ptr(true)
	require(t, s.commitWorker(run))
	evidence := workerStageEvidence(s, run, "original implementation finished", true)
	stage.evidence = &evidence
	s.lifecycleResolver = unsupportedStageResolver{port: stage}
	require(t, s.finishWorkerFlowFor(run, nil))
	if e.RecoverySessionAvailable == nil || !*e.RecoverySessionAvailable || e.RecoveryVersion != 2 {
		t.Fatalf("result receipt without an availability flag invalidated verified recovery consent: available=%v version=%d", e.RecoverySessionAvailable, e.RecoveryVersion)
	}
}

func TestWorkerRecoveryDistinguishesPreparedRejectedAndUnknownInput(t *testing.T) {
	for _, scenario := range []string{"prepared", "rejected", "unknown_input", "unknown_session"} {
		t.Run(scenario, func(t *testing.T) {
			s, run, stage := workerStageFixture(t)
			require(t, acceptTestStage(t, s, run, stage, "original implementation", true))
			w, e := s.workflowRecords(run)
			e.Process = &b.ProcessIdentity{Kind: "fixture", PID: 42, Created: "original"}
			input := b.TurnInput{CommandID: e.PendingCommandID, Prompt: w.StageInputs[1].Prompt}
			session, err := s.startWorkerOperation(run, "prepare_worker_session", "worker-session:"+e.ExecutionID+":"+input.CommandID, input)
			require(t, err)
			status := "confirmed"
			if scenario == "unknown_session" {
				status = "outcome_unknown"
			}
			require(t, s.setOperationState(session, status, "original-session", nil, ""))
			require(t, s.commitWorker(run, session))
			if scenario == "rejected" || scenario == "unknown_input" {
				op, err := s.startWorkerOperation(run, "start_worker_turn", "worker-turn:"+e.ExecutionID+":"+input.CommandID, input)
				require(t, err)
				status = "rejected"
				if scenario == "unknown_input" {
					status = "outcome_unknown"
				}
				require(t, s.setOperationState(op, status, "original-refused-turn", nil, ""))
				e.PendingTurnID = "original-refused-turn"
				require(t, s.commitWorker(run, op))
			}
			s.markWorkerRecoveryRequired(run)
			require(t, s.waitWorkerFlow(run, "verify original input before continuing"))
			knownUnsent := scenario == "prepared" || scenario == "rejected"
			if s.workerInputNotSubmitted(run, input.CommandID) != knownUnsent || s.originalWorkerTurnEnded(run) != knownUnsent {
				t.Fatal("preparation/rejection was confused with unknown native acceptance")
			}
			port := &recoveryStagePort{stageTestPort: stage, recovery: b.LifecycleRecoveryEvidence{SessionAvailable: true, ResourceAvailable: true, SessionIdle: true}}
			fixed, err := s.workerRecords(run)
			require(t, err)
			require(t, s.reconcileWorkerRecovery(context.Background(), nil, run, port, fixed))
			allowed := run.Decision != nil && len(run.Decision.Request.Options) == 1 && run.Decision.Request.Options[0].ID == "continue"
			if allowed != knownUnsent || len(w.StageResults) != 1 || w.StageResults[0].StepID != m.StepImplement {
				t.Fatal("known unsent input has no consent, or unknown input gained consent/result")
			}
		})
	}
}

type rejectedRecoveryBindingPort struct{ *recoveryStagePort }

func (p rejectedRecoveryBindingPort) BindRecovery(context.Context, b.LifecycleSnapshot, b.LifecycleSnapshot) (b.ImplementationDataEnvelope, error) {
	return b.ImplementationDataEnvelope{}, errors.New("original session cannot be rebound")
}

func TestWorkerRecoveryRejectedBindingPreservesOriginalAttempt(t *testing.T) {
	s, run, stage := workerStageFixture(t)
	e := s.executionRecord(run)
	e.Process = &b.ProcessIdentity{Kind: "fixture", PID: 42, Created: "original"}
	e.RecoveryRequired, e.RecoveryState = true, "interrupted"
	e.RecoverySessionAvailable = ptr(true)
	require(t, s.waitWorkerFlow(run, "original process stopped"))
	port := rejectedRecoveryBindingPort{&recoveryStagePort{stageTestPort: stage, recovery: b.LifecycleRecoveryEvidence{SessionAvailable: true, SessionIdle: true, SessionRebindable: true, ProcessAbsent: true, WritersAbsent: true}}}
	s.lifecycleResolver = unsupportedStageResolver{port: port}
	before := recordFingerprint(s.domain)
	attempt, decision := run.AttemptID, run.Decision.ID
	if err := s.beginWorkerRecoveryAttempt(context.Background(), run, "continue"); err == nil {
		t.Fatal("rejected binding created a successor")
	}
	if recordFingerprint(s.domain) != before || run.AttemptID != attempt || run.Decision == nil || run.Decision.ID != decision {
		t.Fatal("rejected binding retired the original attempt or consumed consent")
	}
	// The caller persists its diagnostic. It must not accidentally persist a
	// half-created successor along with that error.
	require(t, s.waitWorkerFlow(run, "rebind rejected"))
	if len(s.domain.Attempts) != 1 || run.AttemptID != attempt || !s.executionRecord(run).RecoveryRequired {
		t.Fatal("failure diagnostic persisted an incomplete recovery attempt")
	}
}

func (p pendingResultPort) PolicyCapabilities() b.LifecyclePolicyCapabilities {
	return b.LifecyclePolicyCapabilities{ServiceStopBehavior: b.StopExecutionOnServiceStop, ResumeActiveObservation: true, SeparateTurnStart: true}
}

func TestWorkerRecoveryCommittedReceiptSettlesPendingResult(t *testing.T) {
	s, run, stage := workerStageFixture(t)
	require(t, acceptTestStage(t, s, run, stage, "original implementation", true))
	w, e := s.workflowRecords(run)
	evidence := workerStageEvidence(s, run, `{"outcome":"done","summary":"original report completed"}`, true)
	evidence.CommandID, evidence.TurnID = e.PendingCommandID, "pending-native-report"
	e.PendingTurnID = evidence.TurnID
	e.RecoveryRequired, e.RecoveryState = true, "unverified"
	e.OwnerServiceInstanceID = s.State.Service.InstanceID
	s.refreshRun(run)
	op, err := s.startWorkerOperation(run, "start_worker_turn", "worker-turn:"+e.ExecutionID+":"+evidence.CommandID, b.TurnInput{CommandID: evidence.CommandID, Prompt: w.StageInputs[1].Prompt})
	require(t, err)
	require(t, s.setOperationState(op, "confirmed", evidence.TurnID, mustJSON(map[string]string{"turn_id": evidence.TurnID}), ""))
	require(t, s.commitWorker(run, op))
	stage.evidence = &evidence
	port := pendingResultPort{stage}
	s.lifecycleResolver = unsupportedStageResolver{port: port}
	beforeOperations := len(s.domain.Operations)
	handled, err := s.resumeLifecycleWorkerFlow(context.Background(), nativeTaskRecord(s.domain.Tasks, run.TaskID), run)
	require(t, err)
	if !handled || stage.observed == 0 || len(w.StageResults) != 2 || w.StageResults[1].CommandID != evidence.CommandID || w.StepID != m.StepPublish || w.Status != "waiting_input" || len(s.domain.Operations) != beforeOperations {
		t.Fatal("committed receipt was hidden behind queued consent or sent another input")
	}
}

func (p *recoveryStagePort) VerifyManualRecovery(context.Context, b.LifecycleSnapshot) (b.LifecycleRecoveryEvidence, error) {
	return p.recovery, nil
}

func TestWorkerRecoveryResourceQualification(t *testing.T) {
	for _, c := range []struct {
		name    string
		facts   b.LifecycleRecoveryEvidence
		allowed bool
	}{
		{"native_resource_without_process", b.LifecycleRecoveryEvidence{SessionAvailable: true, ResourceAvailable: true, SessionIdle: true}, true},
		{"managed_flag_alone", b.LifecycleRecoveryEvidence{SessionAvailable: true, ManagedProcess: true, SessionIdle: true}, false},
		{"active_manual_turn", b.LifecycleRecoveryEvidence{SessionAvailable: true, ResourceAvailable: true}, false},
		{"missing_session", b.LifecycleRecoveryEvidence{ResourceAvailable: true, SessionIdle: true}, false},
		{"proven_rebind", b.LifecycleRecoveryEvidence{SessionAvailable: true, SessionIdle: true, SessionRebindable: true, ProcessAbsent: true, WritersAbsent: true}, true},
		{"writers_unknown", b.LifecycleRecoveryEvidence{SessionAvailable: true, SessionIdle: true, SessionRebindable: true, ProcessAbsent: true}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := reliable.CanContinueOriginal(c.facts); got != c.allowed {
				t.Fatalf("qualification=%v want %v", got, c.allowed)
			}
		})
	}
}

func TestWorkerRecoveryFailedReportWithoutFinal(t *testing.T) {
	s, run, stage := workerStageFixture(t)
	require(t, acceptTestStage(t, s, run, stage, "original implementation", true))
	startQueuedStage(t, s, run)
	w, e := s.workflowRecords(run)
	e.Process = &b.ProcessIdentity{Kind: "fixture", PID: 42, Created: "original"}
	s.refreshRun(run)
	port := &recoveryStagePort{stageTestPort: stage, recovery: b.LifecycleRecoveryEvidence{SessionAvailable: true, ResourceAvailable: true, SessionIdle: true}}
	s.lifecycleResolver = unsupportedStageResolver{port: port}
	fixed, err := s.workerRecords(run)
	require(t, err)
	failed := workerStageEvidence(s, run, "", false)
	failed.FinalVerified, failed.FinalSource, failed.FinalItemID = false, "", ""
	failed.Failure = "native cloud stream interrupted"
	require(t, s.acceptStageTurn(context.Background(), run, port, fixed, failed))
	if w.Status == "reconciling" || len(w.StageResults) != 2 || w.StageResults[1].Succeeded || w.StageResults[1].Failure != failed.Failure {
		t.Fatal("known failed report was lost or remained reconciling")
	}
	fixed, err = s.workerRecords(run)
	require(t, err)
	beforeRevision := s.State.Revision
	require(t, s.reconcileWorkerRecovery(context.Background(), nil, run, port, fixed))
	if run.Decision == nil || len(run.Decision.Request.Options) != 1 || run.Decision.Request.Options[0].ID != "continue" || e.RecoveryState != "completed" {
		t.Fatal("non-process resource did not obtain one continuation decision")
	}
	decision := s.decisionRecord(run)
	if s.State.Revision != beforeRevision+1 || decision.RecoveryVersion != e.RecoveryVersion || decision.RecoveryState != e.RecoveryState {
		t.Fatalf("recovery eligibility and consent were not committed together: revision %d -> %d; decision %d/%s execution %d/%s", beforeRevision, s.State.Revision, decision.RecoveryVersion, decision.RecoveryState, e.RecoveryVersion, e.RecoveryState)
	}
	id, version, count := run.Decision.ID, e.RecoveryVersion, len(s.domain.Decisions)
	require(t, s.reconcileWorkerRecovery(context.Background(), nil, run, port, fixed))
	if run.Decision.ID != id || e.RecoveryVersion != version || len(s.domain.Decisions) != count {
		t.Fatal("identical evidence changed recovery consent")
	}
	port.recovery.SessionIdle = false
	require(t, s.reconcileWorkerRecovery(context.Background(), nil, run, port, fixed))
	if e.RecoveryState != "unverified" || e.RecoveryVersion <= version || len(run.Decision.Request.Options) != 0 {
		t.Fatal("manual active turn preserved stale continuation consent")
	}
}

func TestWorkerRecoveryMissingSuccessfulFinalStaysUnverified(t *testing.T) {
	s, run, port := workerStageFixture(t)
	require(t, acceptTestStage(t, s, run, port, "original implementation", true))
	startQueuedStage(t, s, run)
	w := s.workflowRecord(run)
	fixed, err := s.workerRecords(run)
	require(t, err)
	evidence := workerStageEvidence(s, run, "", true)
	evidence.FinalVerified, evidence.FinalSource, evidence.FinalItemID = false, "", ""
	require(t, s.acceptStageTurn(context.Background(), run, port, fixed, evidence))
	if w.Status != "reconciling" || len(w.StageResults) != 1 {
		t.Fatal("missing success final became retryable failure")
	}
}

func TestWorkerRecoveryLateQualificationCannotChangeCurrentBinding(t *testing.T) {
	s, run, _ := workerStageFixture(t)
	e := s.executionRecord(run)
	e.RecoveryRequired, e.RecoveryVersion = true, 3
	require(t, s.commitWorker(run))
	base := workerQualificationResult{runID: run.RunID, attemptID: run.AttemptID, generation: e.Generation, commandID: e.CurrentCommandID, turnID: e.CurrentTurnID, version: e.RecoveryVersion,
		evidence: b.LifecycleRecoveryEvidence{SessionAvailable: true, ResourceAvailable: true, SessionIdle: true}}
	before := recordFingerprint(s.domain)
	for _, field := range []string{"attempt", "generation", "command", "turn", "version"} {
		t.Run(field, func(t *testing.T) {
			late := base
			switch field {
			case "attempt":
				late.attemptID = newID()
			case "generation":
				late.generation = newID()
			case "command":
				late.commandID = newID()
			case "turn":
				late.turnID = "different-native-turn"
			case "version":
				late.version--
			}
			require(t, s.applyWorkerQualification(late))
			if recordFingerprint(s.domain) != before {
				t.Fatal("late qualification changed the current worker or consent")
			}
		})
	}
}

func TestWorkerRecoveryLateQualificationCannotChangePendingBinding(t *testing.T) {
	for _, field := range []string{"command", "turn"} {
		t.Run(field, func(t *testing.T) {
			s, run, _ := workerStageFixture(t)
			e := s.executionRecord(run)
			e.RecoveryRequired, e.RecoveryState, e.RecoveryVersion = true, "unverified", 3
			require(t, s.commitWorker(run))
			late := workerQualificationResult{runID: run.RunID, attemptID: run.AttemptID, generation: e.Generation, commandID: e.CurrentCommandID, turnID: e.CurrentTurnID, version: e.RecoveryVersion,
				evidence: b.LifecycleRecoveryEvidence{SessionAvailable: true, ResourceAvailable: true, SessionIdle: true}}
			if field == "command" {
				e.PendingCommandID = newID()
			} else {
				e.PendingTurnID = "new-pending-native-turn"
			}
			require(t, s.commitWorker(run))
			before := recordFingerprint(s.domain)
			require(t, s.applyWorkerQualification(late))
			if recordFingerprint(s.domain) != before {
				t.Fatal("old qualification changed recovery after pending input advanced")
			}
		})
	}
}
