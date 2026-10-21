package app

import (
	"encoding/json"
	"testing"

	"github.com/erranliu/SteerLine/internal/hostcontract"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstate"
	"github.com/erranliu/SteerLine/internal/storage/ports"
	"github.com/erranliu/SteerLine/internal/taskrunner"
)

func TestRunnerDeliveryReadProjection(t *testing.T) {
	for _, status := range []string{"running", "waiting_input", "reconciling", "pr_ready", "settling", "completed", "cancelled"} {
		t.Run(status, func(t *testing.T) {
			r := &runnercontract.Run{RunID: "runner", Revision: 12, InputDigest: "fixed", Status: status, Step: "audit.code", Phase: "original_phase", Diagnostic: "original diagnostic", Controls: runnercontract.Controls{CanStop: true}, Execution: runnercontract.Execution{AttemptID: "attempt", SessionID: "session", TurnID: "turn", NativeOutcome: "succeeded"}, Final: &runnercontract.Final{AttemptID: "attempt", SessionID: "session", TurnID: "turn", Verified: true}, ContentResult: &runnercontract.ContentResult{Status: "succeeded"}, PR: &runnercontract.PRFact{State: "open"}}
			r.ContentResult.Source = *r.Final
			c := runnerstate.Candidate{ID: "candidate", RunID: r.RunID, Revision: 2, InputDigest: r.InputDigest, Participation: "active", SubmitState: "accepted", Submit: runnercontract.Request{RequestID: "submit-original"}, Projection: r, Actions: []runnerstate.Action{{Command: "reply", Request: runnercontract.Request{RequestID: "reply-original"}, State: "accepted", ActionStatus: "completed"}}}
			before, _ := json.Marshal(c)
			rt := taskrunner.Project(ports.RunnerSnapshot{Candidates: []runnerstate.Candidate{c}}, hostcontract.ServiceCheckpoint{})
			page := productServicePage(&rt)
			v := page.Runs[0].RunnerDelivery
			if v == nil || !v.Available || v.Revision != 12 || v.Status != status || v.StopStatus != "" || v.ContentStatus != "succeeded" || v.PRState != "open" || !v.FinalVerified || !v.Controls.CanStop || v.Commands[1].RequestID != "reply-original" || v.Commands[1].ActionStatus != "completed" {
				t.Fatalf("independent facts lost or completion inferred: %+v", v)
			}
			v.Commands[1].ActionStatus = "consumer mutation"
			v.Controls.CanStop = false
			if rt.Runs[0].RunnerDelivery.Commands[1].ActionStatus != "completed" || !rt.Runs[0].RunnerDelivery.Controls.CanStop {
				t.Fatal("product response aliases the sampled view")
			}
			after, _ := json.Marshal(c)
			if string(before) != string(after) {
				t.Fatal("read projection mutated source")
			}
		})
	}
}

func TestRunnerDeliveryRejectsForeignAndStaleFacts(t *testing.T) {
	c := runnerstate.Candidate{ID: "candidate", RunID: "runner", InputDigest: "fixed", Participation: "active", SubmitState: "unknown", Projection: &runnercontract.Run{RunID: "foreign", Revision: 4, InputDigest: "fixed", Status: "completed", StopStatus: "stopped", Controls: runnercontract.Controls{CanResume: true}}}
	for _, kind := range []string{"foreign-run", "wrong-input", "missing-input", "missing-revision", "stale-final", "stale-content", "stopped-participation", "absent"} {
		t.Run(kind, func(t *testing.T) {
			r := *c.Projection
			v := c
			v.Projection = &r
			switch kind {
			case "wrong-input":
				r.RunID, r.InputDigest = "runner", "other"
			case "missing-input":
				r.RunID, r.InputDigest = "runner", ""
			case "missing-revision":
				r.RunID, r.Revision = "runner", 0
			case "stale-final":
				r.RunID = "runner"
				r.Execution = runnercontract.Execution{AttemptID: "a", SessionID: "s", TurnID: "current"}
				r.Final = &runnercontract.Final{Verified: true, AttemptID: "a", SessionID: "s", TurnID: "old"}
			case "stale-content":
				r.RunID = "runner"
				r.Execution = runnercontract.Execution{AttemptID: "a", SessionID: "s", TurnID: "current"}
				r.ContentResult = &runnercontract.ContentResult{Status: "succeeded", Source: runnercontract.Final{Verified: true, AttemptID: "a", SessionID: "s", TurnID: "old"}}
			case "stopped-participation":
				r.RunID, v.StopRequested = "runner", true
			case "absent":
				v.Projection = nil
			}
			rt := taskrunner.Project(ports.RunnerSnapshot{Candidates: []runnerstate.Candidate{v}}, hostcontract.ServiceCheckpoint{})
			summary := productServicePage(&rt).Runs[0].RunnerDelivery
			if kind == "stale-final" {
				if !summary.Available || summary.FinalVerified {
					t.Fatal("old final became current", summary)
				}
			} else if kind == "stale-content" {
				if !summary.Available || summary.ContentStatus != "" {
					t.Fatal("old content became current", summary)
				}
			} else if kind == "stopped-participation" {
				if summary.Controls.CanResume {
					t.Fatal("stopped candidate advertised input", summary)
				}
			} else if summary.Available || summary.StopStatus != "" || summary.Controls.CanResume {
				t.Fatal("unverified projection granted facts or controls", summary)
			}
		})
	}
}
