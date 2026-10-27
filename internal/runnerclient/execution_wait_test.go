package runnerclient

import (
	"context"
	"encoding/json"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"os"
	"testing"
	"time"
)

func TestExecutionWaitCapabilityAndResponse(t *testing.T) {
	var caps Capabilities
	schema, err := os.ReadFile("../testkit/protocol/testdata/execution-wait.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{"contract": runnercontract.ExecutionWaitContract, "same_snapshot": true, "run_property": "execution_wait", "schema": json.RawMessage(schema)})
	_ = json.Unmarshal(raw, &caps.ExecutionWait)
	cat := &Catalog{Capabilities: caps}
	// New Orca admission also requires the original atomic observation path.
	_ = json.Unmarshal([]byte(`{"contract":"steerrunner-run-observation/v1","same_snapshot":true,"max_events":100}`), &cat.Capabilities.RunObservation)
	cat.compileExecutionWait()
	if got := cat.ExecutionWaitUnavailable(ExecutionInput{Adapter: "orca-terminal"}); got != "" {
		t.Fatal(got)
	}
	if got := (&Catalog{}).ExecutionWaitUnavailable(ExecutionInput{Adapter: "codex-app-server"}); got != "" {
		t.Fatal(got)
	}
	if got := (&Catalog{}).ExecutionWaitUnavailable(ExecutionInput{Adapter: "orca-terminal"}); got == "" {
		t.Fatal("missing capability admitted Orca")
	}
	withoutObservation := *cat
	withoutObservation.Capabilities.RunObservation = nil
	if withoutObservation.ExecutionWaitUnavailable(ExecutionInput{Adapter: "orca-terminal"}) == "" {
		t.Fatal("missing same-snapshot observation admitted Orca")
	}
	c := takeoverCLI(t).WithCatalog(cat)
	c.RunObservation = true
	for _, mode := range []string{"queue", "null", "missing", "bad-stage", "bad-event"} {
		t.Run(mode, func(t *testing.T) {
			at := time.Now().UTC()
			wait := &runnercontract.ExecutionWait{Contract: runnercontract.ExecutionWaitContract, OperationID: "original-preparation", Query: "repository_inventory", Stage: "queue", ReasonCode: "orca_cli_busy", StartedAt: at, UpdatedAt: at, DeadlineAt: at.Add(90 * time.Second), MaxCalls: 5, Diagnostic: "Orca CLI channel is busy."}
			view := map[string]any{"run_id": "original", "revision": 3, "execution_wait": wait}
			if mode == "null" {
				view["execution_wait"] = nil
			}
			if mode == "missing" {
				delete(view, "execution_wait")
			}
			if mode == "bad-stage" {
				wait.Stage = "unknown"
			}
			events := []Event{}
			if mode == "bad-event" {
				events = append(events, Event{EventID: "wait", RunID: "original", Revision: 3, Type: "execution_wait_changed"})
			}
			fixtureResponse(t, map[string]any{"contract": RunObservationContract, "run": view, "event_page": EventPage{Events: events, NextCursor: "next"}})
			result, err := c.Observe(context.Background(), "original", "")
			if (err == nil) != (mode == "queue" || mode == "null") {
				t.Fatalf("%s: %v", mode, err)
			}
			if err == nil && mode == "queue" && result.Run.ExecutionWait.Contract != runnercontract.ExecutionWaitContract {
				t.Fatal(result)
			}
		})
	}
	caps.ExecutionWait.Schema = json.RawMessage(`{"$ref":"file:///private/schema.json"}`)
	cat.Capabilities = caps
	cat.compileExecutionWait()
	if cat.ExecutionWaitUnavailable(ExecutionInput{Adapter: "orca-terminal"}) == "" {
		t.Fatal("external schema read allowed")
	}
}
