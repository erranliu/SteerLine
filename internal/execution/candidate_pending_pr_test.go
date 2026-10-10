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
	w.StepID, w.CandidateSHA = executionmodel.StepAwaitMerge, info.HeadRevision
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
}
