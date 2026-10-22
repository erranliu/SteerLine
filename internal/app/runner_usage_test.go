package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	cli := runnerclient.Client{Executable: exe, StateDir: t.TempDir(), RunUsageSupported: true, RunCostSupported: true}
	zero, one, count := int64(0), int64(1), int64(225453)
	u := runnercontract.UsageSnapshot{Schema: "run-usage/v1", RunID: "original", Revision: 7, Status: "partial", Scope: "main_session", Tokens: &runnercontract.UsageTokens{Total: &count, CacheWrite: &zero}, ObservedAt: time.Date(2026, 10, 22, 6, 32, 8, 0, time.UTC)}
	u.Coverage.ExpectedSources, u.Coverage.VerifiedSources, u.Coverage.CountedResponses = &one, 1, 11
	amount := "0.05327400"
	p := runnercontract.CostEstimate{Schema: "run-cost-estimate/v1", RunID: u.RunID, Status: "partial", UsageRevision: u.Revision, Scope: u.Scope, UsageStatus: u.Status, Currency: "USD", Covered: &amount, Priced: 10, Unpriced: 1, Basis: "api_equivalent"}
	for _, mode := range []string{"normal", "no-price", "bad-cost", "bad-usage", "unsupported", "cost-unsupported", "relative-price"} {
		t.Run(mode, func(t *testing.T) {
			client, usage, cost, policy := cli, u, p, filepath.Join(t.TempDir(), "prices.json")
			switch mode {
			case "no-price":
				policy = ""
			case "bad-cost":
				cost.UsageRevision++
			case "bad-usage":
				usage.RunID = "foreign"
			case "unsupported":
				client.RunUsageSupported = false
			case "cost-unsupported":
				client.RunCostSupported = false
			case "relative-price":
				policy = "prices.json"
			}
			raw, _ := json.Marshal(map[string]any{"run-usage": usage, "estimate-run-cost": cost})
			t.Setenv("STEERLINE_USAGE_QUERY_FIXTURE", string(raw))
			calls := filepath.Join(t.TempDir(), "calls")
			t.Setenv("STEERLINE_USAGE_QUERY_CALLS", calls)
			view, err := readRunnerUsage(context.Background(), client, "candidate", "original", policy)
			if mode == "bad-usage" {
				if err == nil {
					t.Fatal("foreign usage accepted")
				}
				return
			}
			require(t, err)
			if view.RunID != "candidate" || view.ExecutionID != "original" {
				t.Fatal("candidate identity changed")
			}
			if mode == "unsupported" {
				if view.Reason != "USAGE_UNSUPPORTED" || view.Usage != nil {
					t.Fatal(view)
				}
				if _, err := os.Stat(calls); !os.IsNotExist(err) {
					t.Fatal("unsupported capability invoked CLI")
				}
				return
			}
			if view.Usage == nil || view.Usage.Tokens.Total == nil || *view.Usage.Tokens.Total != count || view.Usage.Tokens.Reasoning != nil || *view.Usage.Tokens.CacheWrite != 0 || !view.Usage.ObservedAt.Equal(u.ObservedAt) {
				t.Fatal("token/time truth lost", view)
			}
			if mode == "normal" {
				if view.Cost == nil || view.Cost.Total != nil || *view.Cost.Covered != amount {
					t.Fatal(view)
				}
			} else if view.Cost != nil || view.CostReason == "" {
				t.Fatal("pricing failure hid cause", view)
			}
			wire, _ := os.ReadFile(calls)
			if strings.Contains(string(wire), "submit") || strings.Contains(string(wire), "service") {
				t.Fatal("query created work")
			}
			if mode == "no-price" && strings.Contains(string(wire), "estimate-run-cost") {
				t.Fatal("unconfigured cost invoked CLI")
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
