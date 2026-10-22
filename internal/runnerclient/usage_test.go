package runnerclient

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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
	c.FullBindingCommands = []string{"run-usage", "estimate-run-cost"}
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
	amount := "0.05327400"
	cost := runnercontract.CostEstimate{Schema: "run-cost-estimate/v1", RunID: u.RunID, UsageRevision: u.Revision, Scope: u.Scope, UsageStatus: u.Status, Basis: "api_equivalent", Status: "estimated", Currency: "USD", Covered: &amount, Total: &amount, Priced: 1}
	for _, change := range []string{"valid", "foreign-run", "revision", "basis", "not-finalized", "partial-total", "negative-price", "null-total", "zero"} {
		v, snapshot := cost, u
		switch change {
		case "foreign-run":
			v.RunID = "foreign"
		case "revision":
			v.UsageRevision++
		case "basis":
			v.Basis = "bill"
		case "not-finalized":
			snapshot.Coverage.Finalized = false
		case "partial-total":
			v.Status = "partial"
		case "negative-price":
			s := "-1"
			v.Total = &s
		case "null-total":
			v.Total = nil
		case "zero":
			s := "0.00000000"
			v.Covered, v.Total = &s, &s
		}
		fixtureResponse(t, v)
		_, err := c.EstimateRunCost(context.Background(), snapshot, filepath.Join(t.TempDir(), "price file.json"))
		if (err == nil) != (change == "valid" || change == "zero") {
			t.Fatal(change, err)
		}
	}
	wire, err := os.ReadFile(os.Getenv("STEERLINE_RUNNER_CLI_CALL"))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"estimate-run-cost\n", "--usage-revision\n7\n", "--expected-instance-id\noriginal-instance\n", "--expected-binding\n", "--stdin--\n"} {
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
	if err := json.Unmarshal([]byte(`{"run_usage":{"schema":"run-usage/v1","query_only":true},"run_cost":{"schema":"run-cost-estimate/v1","usage_contract":"run-usage/v1","query_only":true,"basis":"api_equivalent"}}`), &catalog.Capabilities); err != nil {
		t.Fatal(err)
	}
	c := takeoverCLI(t).WithCatalog(&catalog)
	if !c.RunUsageSupported || !c.RunCostSupported {
		t.Fatal("public capability not consumed")
	}
	c = c.WithCatalog(&Catalog{})
	if c.RunUsageSupported || c.RunCostSupported {
		t.Fatal("absent capability inherited")
	}
	c.UsagePolicy = json.RawMessage(`{"enabled":true,"remote_log_access":{"mode":"disabled","grants":[]}}`)
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
