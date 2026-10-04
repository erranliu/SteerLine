package execution

import (
	"context"
	"errors"
	"testing"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
)

type candidateResumeFailureResolver struct {
	runID string
	err   error
}

func (r candidateResumeFailureResolver) ResolveLifecycle(setup executionbackend.LifecycleSetup) (executionbackend.LifecyclePort, error) {
	if setup.Snapshot.Session.WorkflowRunID == r.runID {
		return nil, r.err
	}
	return mcResolver{}.ResolveLifecycle(setup)
}

func TestCandidateResumeErrorDoesNotStarveSibling(t *testing.T) {
	f := newMCFixture(t)
	mcStartTurn(t, f.s, f.first)
	mcFinishTurn(t, f.s, f.first)
	receipt, _ := f.start(t, "candidate-b")
	first, sibling := f.s.find(f.first.RunID), f.s.find(receipt.WorkflowRunID)
	f.s.runs = []*contracts.Run{first, sibling}
	failure := errors.New("original candidate lifecycle unavailable")
	f.s.lifecycleResolver = candidateResumeFailureResolver{runID: first.RunID, err: failure}
	err := f.s.resumeWorkerFlow(context.Background())
	if !errors.Is(err, failure) {
		t.Fatalf("original candidate diagnostic lost: %v", err)
	}
	if f.s.workers[sibling.RunID] == nil {
		t.Fatal("first candidate error prevented its healthy sibling from resuming")
	}
	if f.s.executionRecord(first).SessionID == f.s.executionRecord(sibling).SessionID {
		t.Fatal("healthy sibling resumed the failed candidate session")
	}
	mcFinishTurn(t, f.s, sibling)
}
