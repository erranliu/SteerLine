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
		name, runStatus, stopStatus, state, stage string
		cancelled, timedOut, settled              bool
	}{
		{"rejected preparation", "blocked", "", "running", "verifying", false, false, false},
		{"normal execution", "running", "", "running", "verifying", false, false, false},
		{"waiting input", "waiting_input", "", "running", "verifying", false, false, false},
		{"stopping", "blocked", "", "stopping", "closing", true, false, false},
		{"total timeout", "blocked", "", "running", "closing", false, true, false},
		{"stop verified", "blocked", "stopped", "unverified", "closing", false, false, false},
		{"settled", "blocked", "stopped", "failed", "failed", false, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := Record{ID: "activity", State: tc.state, Cancelled: tc.cancelled, Rounds: []Round{{Settled: tc.settled, TimedOut: tc.timedOut, Run: &runnercontract.Run{Status: tc.runStatus, StopStatus: tc.stopStatus, Phase: "operation_rejected", Diagnostic: cause}}}}
			before, _ := json.Marshal(r)
			if r.DiagnosticText() != cause || r.Status() != "not_ready" || r.Settled() != tc.settled {
				t.Fatal("observation lost the cause or changed lifecycle", r)
			}
			_, stage, reason := View(Current{ActivityID: r.ID}, r, "", nil)
			if stage != tc.stage || !strings.Contains(reason, cause) {
				t.Fatal("detection phase or diagnosis differs", stage, reason)
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
	r.Rounds[0].Submission = "unknown"
	r.Rounds[0].SubmissionDiagnostic = "original submit timeout"
	if text := r.DiagnosticText(); !strings.Contains(text, r.Diagnostic) || !strings.Contains(text, r.Rounds[0].SubmissionDiagnostic) {
		t.Fatal("owned diagnostic overwritten by Run", text)
	}
	r.Rounds[0].RunID = "original-run"
	r.Diagnostic = r.Rounds[0].SubmissionDiagnostic
	if r.DiagnosticText() != r.Rounds[0].Diagnostic {
		t.Fatal("verified Run binding retained a stale submission warning", r.DiagnosticText())
	}
	r.Rounds[0].Submission = "accepted"
	r.Diagnostic = "observation unavailable"
	if r.DiagnosticText() != r.Diagnostic {
		t.Fatal("confirmed submission obscured the current diagnostic", r.DiagnosticText())
	}
	r.State, r.Kind = "ready", "adaptation"
	r.Rounds[0].Settled = true
	r.Diagnostic = "passed"
	if r.DiagnosticText() != "passed" {
		t.Fatal("historical failure leaked into success", r.DiagnosticText())
	}
}
