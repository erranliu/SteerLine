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
	f.proof(t, run, 81, "merged")
	require(t, s.pollWorkerFlowPR(context.Background(), run, nativeTaskRecord(s.domain.Tasks, run.TaskID), w))
	if w.Status != "waiting_input" || e.PendingCommandID != command || run.Decision.ID != decisionID || len(s.domain.Deliveries) != 0 {
		t.Fatal("actual merge consumed an original pending input or delivered it early")
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
