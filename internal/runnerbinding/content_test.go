package runnerbinding

import (
	"encoding/json"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"testing"
)

func TestContentBindingPreservesOwnerChecks(t *testing.T) {
	id := "run"
	facts := Content{Request: runnercontract.Request{RequestID: "submit"}, Project: "project", ObjectKey: "object", ObjectID: "id", WorkflowID: "generate", WorkflowVersion: 1, RequireDigest: true}
	view := runnercontract.Inspection{Run: &runnercontract.Run{RunID: id, InputDigest: "digest", Revision: 4, Workflow: runnercontract.Workflow{ID: "generate", Version: 1}, Correlation: map[string]string{"steerline_project": "project", "object": "id"}}, Command: &runnercontract.Command{Command: "submit", RequestID: "submit", RunID: &id}}
	if err := ValidateContent(facts, view); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"request", "project", "object", "workflow", "run", "digest", "revision", "empty-digest"} {
		t.Run(field, func(t *testing.T) {
			raw, _ := json.Marshal(view)
			var changed runnercontract.Inspection
			_ = json.Unmarshal(raw, &changed)
			bound := facts
			switch field {
			case "request":
				changed.Command.RequestID = "foreign"
			case "project":
				changed.Run.Correlation["steerline_project"] = "foreign"
			case "object":
				changed.Run.Correlation["object"] = "foreign"
			case "workflow":
				changed.Run.Workflow.Version = 2
			case "run":
				bound.RunID = "foreign"
			case "digest":
				bound.InputDigest = "foreign"
			case "revision":
				bound.Revision = 5
			case "empty-digest":
				changed.Run.InputDigest = ""
			}
			if err := ValidateContent(bound, changed); err == nil {
				t.Fatal("foreign observation accepted")
			}
		})
	}
	view.Run.InputDigest = ""
	facts.RequireDigest = false
	if err := ValidateContent(facts, view); err != nil {
		t.Fatal("generation owner semantics tightened", err)
	}
}

func TestContentBindingStopEnvelopeIsImmutableUntilDefinitiveRejection(t *testing.T) {
	request := ContentStop("original", 7)
	if string(request.Input) != `{"expected_revision":7}` || request.Protocol != runnercontract.Protocol {
		t.Fatal(request)
	}
	for _, state := range []string{"unknown", "accepted"} {
		if NeedsContentStop(request, state, "") {
			t.Fatal("uncertain or accepted envelope replaced")
		}
	}
	if !NeedsContentStop(request, "rejected", "") || !NeedsContentStop(nil, "", "") || NeedsContentStop(nil, "", "stopped") {
		t.Fatal("stop decision differs")
	}
}
