package app

import (
	"testing"

	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstate"
)

func TestRunnerResumeReceiptUsesOriginalCallerRequestAndFinalOutcome(t *testing.T) {
	req := api.ControlRequest{RequestID: "caller-request", Action: "runner_resume"}
	action := runnerstate.Action{
		CallerRequestID: req.RequestID, Command: "resume", State: "accepted",
		Request: runnercontract.Request{RequestID: "separate-runner-request"},
	}
	candidate := runnerstate.Candidate{
		RunID: "run", InputDigest: "input",
		Projection: &runnercontract.Run{
			RunID: "run", InputDigest: "input", Revision: 10, Status: "blocked", ResumeRequired: true,
			Diagnostic: "original history changed",
			Execution:  runnercontract.Execution{AttemptID: "attempt"},
		},
	}
	for _, tc := range []struct {
		state, wantStatus, wantDiagnostic string
	}{
		{"pending", "pending", ""},
		{"failed", "failed", "original history changed"},
		{"completed", "confirmed", ""},
	} {
		t.Run(tc.state, func(t *testing.T) {
			action.ActionStatus = tc.state
			candidate.Actions = []runnerstate.Action{action}
			receipt := api.CommandReceipt{Status: "pending", DecisionAnswer: &api.DecisionAnswer{SourceAttemptID: "attempt"}}
			if !applyRunnerAnswerAction(&receipt, candidate, req) || receipt.Status != tc.wantStatus || receipt.ResultError != tc.wantDiagnostic {
				t.Fatalf("unexpected original receipt: %+v", receipt)
			}
			foreign := req
			foreign.RequestID = action.Request.RequestID
			if applyRunnerAnswerAction(&receipt, candidate, foreign) {
				t.Fatal("Runner request ID matched a product request")
			}
		})
	}
	action.ActionStatus = "failed"
	newer := action
	newer.Request.RequestID, newer.CallerRequestID = "new-runner-request", "new-caller-request"
	candidate.Actions = []runnerstate.Action{action, newer}
	receipt := api.CommandReceipt{Status: "pending", DecisionAnswer: &api.DecisionAnswer{SourceAttemptID: "attempt"}}
	if !applyRunnerAnswerAction(&receipt, candidate, req) || receipt.Status != "failed" || receipt.ResultError != "" {
		t.Fatalf("newer resume diagnostic was assigned to the original request: %+v", receipt)
	}
}
