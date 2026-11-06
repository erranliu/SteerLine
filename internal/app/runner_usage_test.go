package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/billing"
	"github.com/erranliu/SteerLine/internal/billingconfig"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

// A subprocess fixture exposes only public query payloads, with no native state.
func runUsageResponseFixture() bool {
	raw := os.Getenv("STEERLINE_USAGE_QUERY_FIXTURE")
	if raw == "" || len(os.Args) < 2 {
		return false
	}
	var responses map[string]json.RawMessage
	if json.Unmarshal([]byte(raw), &responses) != nil {
		os.Exit(2)
	}
	data, ok := responses[os.Args[1]]
	if !ok {
		return false
	}
	file, err := os.OpenFile(os.Getenv("STEERLINE_USAGE_QUERY_CALLS"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		os.Exit(3)
	}
	_, _ = file.WriteString(strings.Join(os.Args[1:], "\n") + "\n")
	_ = file.Close()
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"protocol": runnerclient.Protocol, "ok": true, "request_id": nil, "data": data, "error": nil})
	return true
}

func TestRunUsageCallerIsolation(t *testing.T) {
	exe, err := os.Executable()
	require(t, err)
	cli := runnerclient.Client{Executable: exe, StateDir: t.TempDir(), RunUsageSupported: true, RunUsageEventsSupported: true}
	one, zero, count := int64(1), int64(0), int64(1000000)
	provider, model := "synthetic", "model"
	at := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	u := runnercontract.UsageSnapshot{Schema: "run-usage/v1", RunID: "original", Revision: 7, Status: "partial", Scope: "main_session", Tokens: &runnercontract.UsageTokens{InputTotal: &count, InputUncached: &count, CacheRead: &zero, CacheWrite: &zero, Output: &zero, Total: &count}, ObservedAt: at}
	u.Coverage.ExpectedSources = &one
	u.Coverage.VerifiedSources = 1
	u.Coverage.CountedResponses = 1
	p := runnercontract.UsagePage{RunID: u.RunID, Revision: u.Revision, Events: []runnercontract.UsageEvent{{ID: "response", RunID: u.RunID, Decision: "counted", Provider: &provider, Model: &model, MetadataBasis: "observed", TimeBasis: "request", CallTime: &at, Tokens: *u.Tokens}}}
	for _, mode := range []string{"normal", "finalized", "metadata-incomplete", "metadata-conflict", "details-unsupported", "no-price", "bad-page", "bad-usage", "unsupported", "invalid-price"} {
		t.Run(mode, func(t *testing.T) {
			client, usage, page, home := cli, u, p, t.TempDir()
			page.Events = append([]runnercontract.UsageEvent(nil), p.Events...)
			if mode != "no-price" {
				_, err := (billingconfig.Store{Home: home}).Save("*", billing.Policy{Source: "synthetic", Currency: "USD", Basis: billing.Basis, Rules: []billing.Rule{{ID: "r", Provider: provider, Model: model, From: at.Add(-time.Hour), Rates: billing.Rates{Input: "2", Read: "0", Write: "0", Output: "0"}}}})
				require(t, err)
			}
			switch mode {
			case "finalized":
				usage.Status, usage.Coverage.Finalized = "available", true
			case "metadata-incomplete":
				page.Events[0].Model, page.Events[0].MetadataBasis = nil, "unknown"
				page.Events[0].MetadataIssues = []runnercontract.UsageIssue{{Category: "metadata", Code: "SOURCE_METADATA_INCOMPLETE"}}
			case "metadata-conflict":
				page.Events[0].MetadataIssues = []runnercontract.UsageIssue{{Category: "metadata", Code: "SOURCE_METADATA_CONFLICT"}}
			case "details-unsupported":
				client.RunUsageEventsSupported = false
			case "bad-page":
				page.Revision++
			case "bad-usage":
				usage.RunID = "foreign"
			case "unsupported":
				client.RunUsageSupported = false
			case "invalid-price":
				require(t, os.WriteFile(filepath.Join(home, "billing.json"), []byte(`{"schema":"old"}`), 0600))
			}
			raw, _ := json.Marshal(map[string]any{"run-usage": usage, "run-usage-events": page})
			t.Setenv("STEERLINE_USAGE_QUERY_FIXTURE", string(raw))
			calls := filepath.Join(t.TempDir(), "calls")
			t.Setenv("STEERLINE_USAGE_QUERY_CALLS", calls)
			view, err := readRunnerUsage(context.Background(), client, "candidate", "original", home)
			if mode == "bad-usage" {
				if err == nil {
					t.Fatal("foreign usage accepted")
				}
				return
			}
			require(t, err)
			if mode == "unsupported" {
				if view.Reason != "USAGE_UNSUPPORTED" || view.Usage != nil {
					t.Fatal(view)
				}
				return
			}
			if view.Usage == nil || view.Usage.Tokens.Total == nil || *view.Usage.Tokens.Total != count {
				t.Fatal("Token truth lost", view)
			}
			if mode == "normal" {
				if view.Cost == nil || view.Cost.Total != nil || view.Cost.Covered == nil || *view.Cost.Covered != "2.000000000000000000" {
					t.Fatal(view)
				}
			} else if mode == "finalized" {
				if view.Cost == nil || view.Cost.Status != "estimated" || view.Cost.Total == nil || *view.Cost.Total != "2.000000000000000000" {
					t.Fatal("final total missing", view)
				}
			} else if strings.HasPrefix(mode, "metadata-") {
				want := "MODEL_UNKNOWN"
				if mode == "metadata-conflict" {
					want = "PRICING_METADATA_CONFLICT"
				}
				if view.Cost == nil || view.Cost.Status != "unavailable" || len(view.Cost.Issues) == 0 || view.Cost.Issues[0] != want {
					t.Fatal("metadata cause lost", view)
				}
			} else if view.Cost != nil || view.CostReason == "" {
				t.Fatal("pricing error hidden", view)
			}
			wire, _ := os.ReadFile(calls)
			if strings.Contains(string(wire), "estimate-run-cost") || strings.Contains(string(wire), "submit") || strings.Contains(string(wire), "service") {
				t.Fatal("query crossed authority", string(wire))
			}
		})
	}
}

func TestRunUsageCandidateBinding(t *testing.T) {
	config, _ := nativeTaskFixture(t)
	t.Setenv("STEERLINE_TEST_RUNNER_SERVICE_VERSION", "unit-fixture")
	prepareStoppedRunRepository(t, config)
	require(t, os.MkdirAll(EtcDir(), 0700))
	require(t, os.WriteFile(filepath.Join(EtcDir(), "one.json"), projectConfigJSON(t, config), 0600))
	directory, runtime := startPresetDirectory(t)
	defer func() { require(t, directory.Close()) }()
	candidate := seedStoppedPublicRun(t, runtime, config)
	cli := runtime.runner.client
	stateDir := cli.StateDir
	cli.RunUsageSupported = true
	bound := runtime.config
	bound.runtimeRunner = &cli
	reader := &clientReader{config: bound, project: bound.Identity()}
	if _, err := reader.RunUsage(context.Background(), candidate.ID, "foreign"); err == nil {
		t.Fatal("foreign Run queried")
	}
	cli.StateDir += "-other"
	if _, err := reader.RunUsage(context.Background(), candidate.ID, candidate.RunID); err == nil {
		t.Fatal("foreign Runner directory queried")
	}
	if _, err := reader.RunUsage(context.Background(), "missing", candidate.RunID); err == nil {
		t.Fatal("missing candidate accepted")
	}
	cli.StateDir, cli.RunUsageSupported = stateDir, false
	view, err := reader.RunUsage(context.Background(), candidate.ID, candidate.RunID)
	require(t, err)
	if view.RunID != candidate.ID || view.ExecutionID != candidate.RunID || view.Reason != "USAGE_UNSUPPORTED" {
		t.Fatal("valid original candidate was not read", view)
	}
}
