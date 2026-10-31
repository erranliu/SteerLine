//go:build e2e

package testenv

import (
	"testing"

	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstate"
)

func admissionFixture() runnerstate.Candidate {
	return runnerstate.Candidate{ID: "candidate", RunID: "original", InputDigest: "digest", Projection: &runnercontract.Run{
		RunID: "original", InputDigest: "digest", Execution: runnercontract.Execution{
			Workspace: "owned-workspace", SessionID: "original-session", TurnID: "original-turn", TurnStarted: true,
		},
	}}
}

func TestEnvironmentAdmissionStopsOnlyAfterOriginalTurnStarts(t *testing.T) {
	var probe admissionProbe
	stops := 0
	// Poll the production observer just as the admission flow does. A stop spy
	// must see no call for a created Run, allocated session, or incomplete turn.
	for step := 0; step < 5; step++ {
		c := admissionFixture()
		switch step {
		case 0:
			c.Projection = nil
		case 1:
			c.Projection.Execution.TurnStarted = false
		case 2:
			c.Projection.Execution.SessionID = ""
		case 3:
			c.Projection.Execution.TurnID = ""
		}
		ready, err := probe.observe(c)
		if err != nil {
			t.Fatal(err)
		}
		if ready {
			stops++
			if step != 4 {
				t.Fatalf("native stop issued during startup step %d", step)
			}
		}
	}
	if stops != 1 {
		t.Fatalf("original started turn must be stopped exactly once, got %d", stops)
	}
}

func TestEnvironmentAdmissionRejectsForeignOrUnknownOriginal(t *testing.T) {
	cases := map[string]func(*runnerstate.Candidate){
		"candidate":      func(c *runnerstate.Candidate) { c.ID = "foreign" },
		"run":            func(c *runnerstate.Candidate) { c.RunID = "foreign" },
		"digest":         func(c *runnerstate.Candidate) { c.InputDigest = "foreign" },
		"projection-run": func(c *runnerstate.Candidate) { c.Projection.RunID = "foreign" },
		"projection-digest": func(c *runnerstate.Candidate) {
			c.Projection.InputDigest = "foreign"
		},
		"unknown-stop":     func(c *runnerstate.Candidate) { c.Projection.StopStatus = "unknown" },
		"unsupported-stop": func(c *runnerstate.Candidate) { c.Projection.StopStatus = "unsupported" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			var probe admissionProbe
			original := admissionFixture()
			original.Projection.Execution.TurnStarted = false
			if ready, err := probe.observe(original); ready || err != nil {
				t.Fatal("original resource binding did not remain pending", ready, err)
			}
			foreign := admissionFixture()
			mutate(&foreign)
			if ready, err := probe.observe(foreign); ready || err == nil {
				t.Fatal("unsafe observation could trigger a native stop", ready, err)
			}
		})
	}
}
