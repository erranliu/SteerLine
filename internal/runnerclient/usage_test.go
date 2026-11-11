package runnerclient

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/runnercontract"
)

func usageFixture() runnercontract.UsageSnapshot {
	v := runnercontract.UsageSnapshot{Schema: "run-usage/v1", RunID: "original", Revision: 7, Status: "available", Scope: "main_session", ObservedAt: time.Now().UTC()}
	one := int64(1)
	v.Coverage.ExpectedSources, v.Coverage.VerifiedSources, v.Coverage.CountedResponses, v.Coverage.Finalized = &one, 1, 1, true
	return v
}

func TestRunUsageSnapshotAndCostBinding(t *testing.T) {
	c := takeoverCLI(t)
	c.InstanceID, c.BuildVersion, c.ResourceDigest = "original-instance", "fixture", strings.Repeat("a", 64)
	c.FullBindingCommands = []string{"run-usage", "run-usage-events"}
	u := usageFixture()
	for _, change := range []string{"valid", "foreign-run", "revision", "scope", "status", "negative", "unknown-tokens"} {
		v := u
		switch change {
		case "foreign-run":
			v.RunID = "foreign"
		case "revision":
			v.Revision = 0
		case "scope":
			v.Scope = "all"
		case "status":
			v.Status = "new"
		case "negative":
			n := int64(-1)
			v.Tokens = &runnercontract.UsageTokens{Total: &n}
		case "unknown-tokens":
			v.Status = "unavailable"
		}
		fixtureResponse(t, v)
		actual, err := c.RunUsage(context.Background(), "original")
		if (err == nil) != (change == "valid" || change == "unknown-tokens") {
			t.Fatal(change, actual, err)
		}
	}
	page := runnercontract.UsagePage{RunID: u.RunID, Revision: u.Revision, Events: []runnercontract.UsageEvent{{ID: "event", RunID: u.RunID, Decision: "counted"}}}
	for _, change := range []string{"valid", "foreign-run", "revision", "duplicate", "count-mismatch", "cursor-loop"} {
		v := page
		v.Events = append([]runnercontract.UsageEvent(nil), page.Events...)
		switch change {
		case "foreign-run":
			v.RunID = "foreign"
		case "revision":
			v.Revision++
		case "duplicate":
			v.Events = append(v.Events, v.Events[0])
		case "count-mismatch":
			v.Events = nil
		case "cursor-loop":
			v.NextCursor = "same"
		}
		fixtureResponse(t, v)
		_, err := c.RunUsageEvents(context.Background(), u)
		if (err == nil) != (change == "valid") {
			t.Fatal(change, err)
		}
	}
	wire, err := os.ReadFile(os.Getenv("STEERLINE_RUNNER_CLI_CALL"))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"run-usage-events\n", "--snapshot-revision\n7\n", "--expected-instance-id\noriginal-instance\n", "--expected-binding\n", "--stdin--\n"} {
		if !strings.Contains(string(wire), expected) {
			t.Fatal("missing exact query/binding", expected, string(wire))
		}
	}
	if strings.Contains(string(wire), "--input") {
		t.Fatal("query sent a mutation envelope")
	}
}

func TestRunUsageCapabilityAndStartupPolicy(t *testing.T) {
	var catalog Catalog
	if err := json.Unmarshal([]byte(`{"run_usage":{"schema":"run-usage/v1","query_only":true,"same_snapshot":true,"events_schema":"run-usage-events/v1","max_events":500,"configured_model_attribution":"frozen_stage_input","observed_model_field":"model"}}`), &catalog.Capabilities); err != nil {
		t.Fatal(err)
	}
	c := takeoverCLI(t).WithCatalog(&catalog)
	if !c.RunUsageSupported || !c.RunUsageEventsSupported || !c.ConfiguredUsageSupported {
		t.Fatal("public capability not consumed")
	}
	c = c.WithCatalog(&Catalog{})
	if c.RunUsageSupported || c.RunUsageEventsSupported || c.ConfiguredUsageSupported {
		t.Fatal("absent capability inherited")
	}
	c.UsagePolicy = json.RawMessage(`{"enabled":true}`)
	info := ServiceInfo{Protocol: Protocol, Status: "ready", StateDir: c.StateDir, InstanceID: "new", Version: "fixture", ResourceDigest: "resources", ExecutableSHA256: c.SHA256}
	fixtureResponse(t, info)
	if _, err := c.StartServiceWithStops(context.Background(), Version{Version: info.Version, ResourceDigest: info.ResourceDigest}, nil); err != nil {
		t.Fatal(err)
	}
	wire, _ := os.ReadFile(os.Getenv("STEERLINE_RUNNER_CLI_CALL"))
	parts := strings.SplitN(string(wire), "--stdin--\n", 2)
	var input runnercontract.ServiceStartInput
	if len(parts) != 2 || json.Unmarshal([]byte(parts[1]), &input) != nil || string(input.UsagePolicy) != string(c.UsagePolicy) || input.PendingStops == nil {
		t.Fatal("startup policy missing or altered", string(wire))
	}
}

func TestRunUsageMixedScopesKeepMainSummary(t *testing.T) {
	c := takeoverCLI(t)
	u := usageFixture()
	u.ModelCallCoverage = &runnercontract.ModelCallCoverage{InternalRecorded: 1, InternalCovered: 1}
	internal := runnercontract.UsageEvent{ID: "internal", RunID: u.RunID, Scope: "runner_internal", CallID: "call", CallSource: "llm_parser", SourceResultID: "result", CommandID: "command", AttemptID: "attempt", CoverageStatus: "covered", Decision: "counted", ConfiguredModel: "parser-model"}
	page := runnercontract.UsagePage{RunID: u.RunID, Revision: u.Revision, Events: []runnercontract.UsageEvent{{ID: "main", RunID: u.RunID, Decision: "counted"}, internal}}
	fixtureResponse(t, page)
	events, err := c.RunUsageEvents(context.Background(), u)
	if err != nil || len(events) != 2 {
		t.Fatal("internal polluted main counter", events, err)
	}
	for _, change := range []string{"unknown-scope", "duplicate-call", "covered-count", "uncovered-tokens"} {
		t.Run(change, func(t *testing.T) {
			v := page
			v.Events = append([]runnercontract.UsageEvent(nil), page.Events...)
			switch change {
			case "unknown-scope":
				v.Events[1].Scope = "everything"
			case "duplicate-call":
				other := internal
				other.ID = "different-id"
				v.Events = append(v.Events, other)
			case "covered-count":
				v.Events[1].CoverageStatus = "uncovered"
			case "uncovered-tokens":
				v.Events[1].CoverageStatus = "uncovered"
				v.Events[1].Decision = "not_counted"
				one := int64(1)
				v.Events[1].Tokens.Total = &one
			}
			fixtureResponse(t, v)
			if _, err := c.RunUsageEvents(context.Background(), u); err == nil {
				t.Fatal("invalid internal source accepted")
			}
		})
	}
}
