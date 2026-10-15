package execution

import (
	"context"
	"testing"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

func TestCandidateOpenPRPollPreservesOriginalPendingInput(t *testing.T) {
	f := newMCFixture(t)
	s, run := f.s, f.first
	mcStartTurn(t, s, run)
	mcFinishTurn(t, s, run)
	info := f.bind(t, run, 81, "open")
	w, e := s.workflowRecords(run)
	w.StepID = executionmodel.StepAwaitMerge
	require(t, s.offerWorkerPRFeedback(run, info.URL))
	command := newID()
	e.PendingCommandID = command
	w.Status = "waiting_input"
	s.refreshRun(run)
	require(t, s.commitWorker(run))
	decisionID := run.Decision.ID
	require(t, s.pollWorkerFlowPR(context.Background(), run, nativeTaskRecord(s.domain.Tasks, run.TaskID), w))
	w, e = s.workflowRecords(run)
	if w.Status != "waiting_input" || e.PendingCommandID != command || w.PRURL != info.URL || run.Decision.ID != decisionID {
		t.Fatal("actual provider poll restored merge phase/replaced Decision while original input was pending")
	}
	// A bound PR is still monitored while a feedback start is in progress.
	w.StepID, w.Status, e.Status = executionmodel.StepImplement, "running", "active"
	s.refreshRun(run)
	require(t, s.commitWorker(run))
	require(t, s.pollWorkerFlowPR(context.Background(), run, nativeTaskRecord(s.domain.Tasks, run.TaskID), w))
	if w.StepID != executionmodel.StepImplement || w.Status != "running" || e.PendingCommandID != command {
		t.Fatal("open bound PR replaced the active feedback stage")
	}
	inputs, turns := len(w.StageInputs), e.TurnNumber
	f.proof(t, run, 81, "merged")
	require(t, s.pollWorkerFlowPR(context.Background(), run, nativeTaskRecord(s.domain.Tasks, run.TaskID), w))
	w, e = s.workflowRecords(run)
	if nativeTaskRecord(s.domain.Tasks, run.TaskID).Status != "closed" || len(s.domain.Deliveries) != 1 || e.PendingCommandID != command || len(w.StageInputs) != inputs || e.TurnNumber != turns {
		t.Fatal("verified merge waited for feedback or dispatched another input")
	}
}

func TestCandidateOpenPRPollPreservesSavedFeedback(t *testing.T) {
	f := newMCFixture(t)
	s, run := f.s, f.first
	mcStartTurn(t, s, run)
	mcFinishTurn(t, s, run)
	info := f.bind(t, run, 81, "open")
	w, e := s.workflowRecords(run)
	w.StepID, w.Status = executionmodel.StepAwaitMerge, "waiting_external"
	e.PendingCommandID, e.PendingTurnID = "", ""
	s.setDecisionStatus(run, "superseded")
	s.setRunDecision(run, nil)
	require(t, s.offerWorkerPRFeedback(run, info.URL))
	s.setDecisionStatus(run, "answer_saved")
	s.refreshRun(run)
	require(t, s.commitWorker(run))
	decisionID, turns := run.Decision.ID, e.TurnNumber
	for i := 0; i < 2; i++ {
		require(t, s.pollWorkerFlowPR(context.Background(), run, nativeTaskRecord(s.domain.Tasks, run.TaskID), s.workflowRecord(run)))
		if run.Decision.ID != decisionID || run.Decision.Status != "answer_saved" || s.executionRecord(run).TurnNumber != turns {
			t.Fatal("PR polling replaced the saved answer before its original feedback delivery")
		}
	}
}

func TestCandidateClosedPRPollDeliversLaterMerge(t *testing.T) {
	f := newMCFixture(t)
	s, run := f.s, f.first
	mcStartTurn(t, s, run)
	// This fixture runs the existing session in the current service, rather than
	// leaving a report queued behind the prior-service continuation gate.
	e := s.executionRecord(run)
	e.OwnerServiceInstanceID, e.RecoveryRequired = s.State.Service.InstanceID, false
	require(t, s.commitWorker(run))
	mcFinishTurn(t, s, run)
	if s.executionRecord(run).PendingCommandID != "" {
		t.Fatal("fixture still has an original pending input")
	}
	f.bind(t, run, 81, "closed_unmerged")
	// Replace the neutral worker fixture's own question with the ordinary PR
	// feedback prompt; production polling must retain real worker questions.
	s.setDecisionStatus(run, "superseded")
	s.setRunDecision(run, nil)
	require(t, s.offerWorkerPRFeedback(run, s.workflowRecord(run).PRURL))
	require(t, s.pollWorkerFlowPR(context.Background(), run, nativeTaskRecord(s.domain.Tasks, "8"), s.workflowRecord(run)))
	if run.WorkflowStatus != "waiting_input" {
		t.Fatal("closed PR did not await explicit handling")
	}
	inputs, turns := len(s.workflowRecord(run).StageInputs), s.executionRecord(run).TurnNumber
	f.proof(t, run, 81, "merged")
	require(t, s.pollWorkerFlowPR(context.Background(), run, nativeTaskRecord(s.domain.Tasks, "8"), s.workflowRecord(run)))
	if run.WorkflowStatus != "completed" || len(s.domain.Deliveries) != 1 || len(s.workflowRecord(run).StageInputs) != inputs || s.executionRecord(run).TurnNumber != turns {
		t.Fatal("provider-confirmed merge required another worker input or lost delivery")
	}
}
