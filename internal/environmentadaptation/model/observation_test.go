package model

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/runnercontract"
)

func TestProjectEnvironmentBlockedObservationPreservesLifecycle(t *testing.T) {
	const cause = "cannot query Orca repository inventory: context deadline exceeded; Could not connect to the remote Orca runtime."
	for _, tc := range []struct {
		name, runStatus, stopStatus, state    string
		cancelled, timedOut, settled, blocked bool
	}{
		{"rejected preparation", "blocked", "", "running", false, false, false, true},
		{"normal execution", "running", "", "running", false, false, false, false},
		{"waiting input", "waiting_input", "", "running", false, false, false, false},
		{"stopping", "blocked", "", "stopping", true, false, false, false},
		{"total timeout", "blocked", "", "running", false, true, false, false},
		{"stop verified", "blocked", "stopped", "unverified", false, false, false, false},
		{"settled", "blocked", "stopped", "failed", false, false, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := Record{ID: "activity", State: tc.state, Cancelled: tc.cancelled, Rounds: []Round{{Settled: tc.settled, TimedOut: tc.timedOut, Run: &runnercontract.Run{Status: tc.runStatus, StopStatus: tc.stopStatus, Phase: "operation_rejected", Diagnostic: cause}}}}
			before, _ := json.Marshal(r)
			if r.Blocked() != tc.blocked || r.DiagnosticText() != cause || r.Status() != "not_ready" || r.Settled() != tc.settled {
				t.Fatal("observation lost the cause or changed lifecycle", r)
			}
			_, stage, reason := View(Current{ActivityID: r.ID}, r, "", nil)
			if tc.blocked && (stage != "blocked" || reason != cause) {
				t.Fatal("blocked Run hidden until settlement", stage, reason)
			}
			after, _ := json.Marshal(r)
			if string(before) != string(after) {
				t.Fatal("read mutated the activity")
			}
		})
	}
}

func TestProjectEnvironmentDiagnosticKeepsOwnedCauses(t *testing.T) {
	r := Record{State: "running", Rounds: []Round{{Run: &runnercontract.Run{Diagnostic: "Run connection timeout"}}}}
	if r.DiagnosticText() != "Run connection timeout" {
		t.Fatal(r.DiagnosticText())
	}
	r.Rounds[0].Diagnostic = "round collection unavailable"
	if r.DiagnosticText() != "round collection unavailable" {
		t.Fatal(r.DiagnosticText())
	}
	r.Diagnostic = "observation unavailable"
	r.Rounds[0].SubmissionDiagnostic = "original submit timeout"
	if text := r.DiagnosticText(); !strings.Contains(text, r.Diagnostic) || !strings.Contains(text, r.Rounds[0].SubmissionDiagnostic) {
		t.Fatal("owned diagnostic overwritten by Run", text)
	}
	r.State, r.Kind = "ready", "adaptation"
	r.Rounds[0].Settled = true
	r.Diagnostic = "passed"
	if r.DiagnosticText() != "passed" {
		t.Fatal("historical failure leaked into success", r.DiagnosticText())
	}
}
