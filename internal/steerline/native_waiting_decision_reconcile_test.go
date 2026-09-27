package steerline

import "testing"

func TestNativeWaitingDecisionProjectionRequiresMatchingCurrentRecords(t *testing.T) {
	newState := func() (*Runtime, *Run) {
		run := &Run{RunID: "workflow-test", ExecutionID: "execution-test", AttemptID: "attempt-test", ExecutionStatus: "running", FinishTaskStatus: "pending", Decision: &DecisionState{ID: "question-test", AttemptID: "attempt-test", Status: "pending"}}
		state := &Runtime{
			Attempts:  []StepAttemptRecord{{AttemptID: "attempt-test", ExecutionID: "execution-test", Status: "waiting_decision"}},
			Decisions: []DecisionRecord{{DecisionID: "question-test", WorkflowRunID: "workflow-test", ExecutionID: "execution-test", AttemptID: "attempt-test", Status: "pending"}},
		}
		return state, run
	}
	state, run := newState()
	reconcileNativeWaitingDecision(state, run)
	if run.FinishTaskStatus != "waiting_decision" {
		t.Fatalf("matching original question remained unanswerable: %s", run.FinishTaskStatus)
	}
	for name, change := range map[string]func(*Runtime, *Run){
		"old attempt":      func(state *Runtime, _ *Run) { state.Attempts[0].AttemptID = "old-attempt" },
		"wrong execution":  func(state *Runtime, _ *Run) { state.Decisions[0].ExecutionID = "other-execution" },
		"applied decision": func(_ *Runtime, run *Run) { run.Decision.Status = "applied" },
		"unknown run":      func(_ *Runtime, run *Run) { run.ExecutionStatus = "unknown" },
	} {
		t.Run(name, func(t *testing.T) {
			state, run := newState()
			change(state, run)
			reconcileNativeWaitingDecision(state, run)
			if run.FinishTaskStatus != "pending" {
				t.Fatalf("unrelated or uncertain state became answerable: %s", run.FinishTaskStatus)
			}
		})
	}
}
