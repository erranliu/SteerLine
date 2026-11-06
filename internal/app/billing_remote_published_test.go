//go:build runneracceptance

package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/billing"
	"github.com/erranliu/SteerLine/internal/billingconfig"
	"github.com/erranliu/SteerLine/internal/executionconfig"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstatepath"
	"github.com/google/uuid"
)

// This opt-in live test accepts a frozen external execute/1 request. It never
// copies installed state or constructs a replacement request for an unknown run.
func TestBillingRemoteUsagePublished(t *testing.T) {
	root, inputPath := os.Getenv("STEERLINE_USAGE_LIVE_ROOT"), os.Getenv("STEERLINE_USAGE_LIVE_INPUT")
	if root == "" || inputPath == "" || !filepath.IsAbs(root) || !strings.HasPrefix(strings.ToLower(filepath.Clean(root)), `c:\steerlinedebug\`) {
		t.Fatal("explicit new isolated live root and request required")
	}
	if _, err := os.Lstat(root); !os.IsNotExist(err) {
		t.Fatal("live root must not exist", err)
	}
	raw, err := os.ReadFile(inputPath)
	require(t, err)
	var input runnercontract.RunInput
	require(t, json.Unmarshal(raw, &input))
	if input.Workflow.ID != "execute" || input.Workflow.Version != 1 || input.Execution.Adapter != "orca-terminal" || input.Execution.Target.Kind != "orca_environment" || input.Execution.Target.Environment == "" || !strings.HasPrefix(strings.ToLower(filepath.Clean(input.Repository.LocalPath)), `c:\steerlinedebug\`) {
		t.Fatal("frozen request must target a new isolated remote execute/1 workspace")
	}
	require(t, os.MkdirAll(root, 0700))
	home := filepath.Join(root, "home")
	require(t, os.MkdirAll(home, 0700))
	save := func(name string, v any) {
		t.Helper()
		body, e := json.MarshalIndent(v, "", "  ")
		require(t, e)
		require(t, os.WriteFile(filepath.Join(root, name+".json"), body, 0600))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	selection := runnerclient.Selection{Executable: os.Getenv("STEERRUNNER_EXE"), Version: os.Getenv("STEERRUNNER_VERSION"), SHA256: os.Getenv("STEERRUNNER_SHA256")}
	cli, version, err := selection.Open(ctx, runnerstatepath.Directory(home), 30*time.Second)
	require(t, err)
	state := cli.StateDir
	runID := ""
	started := false
	start := func(policy json.RawMessage) {
		t.Helper()
		cli.InstanceID = ""
		cli.UsagePolicy = policy
		info, e := cli.StartService(ctx, version)
		require(t, e)
		started = true
		cli.InstanceID = info.InstanceID
		catalog, e := cli.Catalog(ctx, version)
		require(t, e)
		cli = cli.WithCatalog(catalog)
		save("effective-policy-"+info.InstanceID, info)
	}
	stopService := func() {
		t.Helper()
		id := cli.InstanceID
		request := uuid.NewString()
		require(t, cli.StopService(ctx, id, request))
		_, e := cli.VerifyServiceStopped(ctx, id, request)
		require(t, e)
		started = false
	}
	t.Cleanup(func() {
		clean, c := context.WithTimeout(context.Background(), 2*time.Minute)
		defer c()
		ok := true
		diagnostics := []string{}
		if !started && runID != "" {
			ok = false
			diagnostics = append(diagnostics, "service unavailable; original Run settlement remains unverified")
		}
		if started && runID != "" {
			inspection, e := cli.Inspect(clean, "run-id", runID)
			if e == nil && inspection.Run != nil {
				body, _ := json.Marshal(map[string]uint64{"expected_revision": inspection.Run.Revision})
				request := runnerclient.Request{Protocol: runnerclient.Protocol, RequestID: uuid.NewString(), Input: body}
				_, e = cli.Call(clean, "stop", []string{"--run-id", runID}, &request)
			}
			if e != nil {
				ok = false
				diagnostics = append(diagnostics, e.Error())
			}
			deadline := time.Now().Add(40 * time.Second)
			for e == nil {
				v, readErr := cli.Inspect(clean, "run-id", runID)
				if readErr != nil {
					ok = false
					diagnostics = append(diagnostics, readErr.Error())
					break
				}
				if v.Run != nil && v.Run.StopStatus == "stopped" {
					break
				}
				if time.Now().After(deadline) {
					ok = false
					diagnostics = append(diagnostics, "original run stop unverified")
					break
				}
				time.Sleep(time.Second)
			}
		}
		if started {
			request := uuid.NewString()
			e := cli.StopService(clean, cli.InstanceID, request)
			if e == nil {
				e = cli.VerifyServiceSettlement(clean, cli.InstanceID, request)
			}
			if e != nil {
				ok = false
				diagnostics = append(diagnostics, e.Error())
			}
		}
		save("cleanup", map[string]any{"verified": ok, "run_id": runID, "state_dir": state, "diagnostics": diagnostics})
		if !ok {
			t.Error("live cleanup remains unverified", diagnostics)
		}
	})
	initial, _ := json.Marshal(map[string]any{"enabled": true, "remote_log_access": map[string]any{"mode": "disabled", "grants": []any{}}})
	start(initial)
	submitID := uuid.NewString()
	receipt, err := cli.Submit(ctx, runnerclient.Request{Protocol: runnerclient.Protocol, RequestID: submitID, Input: raw})
	if err != nil {
		// Only reconcile this exact original request; never submit another ID.
		inspected, e := cli.Inspect(ctx, "request-id", submitID)
		if e != nil || inspected.Run == nil {
			t.Fatal("original submit remains unverified", err, e)
		}
		runID = inspected.Run.RunID
	} else {
		runID = receipt.RunID
	}
	save("original-request", map[string]any{"request_id": submitID, "run_id": runID, "input_path": inputPath, "exe_sha256": selection.SHA256, "state_dir": state})
	t.Logf("isolated Run=%s state=%s evidence=%s", runID, state, root)
	var missing runnercontract.UsageSnapshot
	var original runnerclient.Run
	deadline := time.Now().Add(3 * time.Minute)
	for {
		v, e := cli.Inspect(ctx, "run-id", runID)
		if e == nil && v.Run != nil {
			original = *v.Run
		}
		u, e := cli.RunUsage(ctx, runID)
		if e == nil {
			missing = u
		}
		blocked := false
		for _, issue := range missing.Issues {
			blocked = blocked || issue.Code == "REMOTE_LOG_ACCESS_NOT_CONFIGURED"
		}
		if blocked && len(missing.AccessRequirements) > 0 && original.Final != nil && original.Final.Verified {
			break
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			save("missing-usage", missing)
			save("original-run", original)
			t.Fatal("missing authorized source requirement or original final", original.Status, original.Diagnostic, missing.Issues)
		}
		time.Sleep(time.Second)
	}
	save("missing-usage", missing)
	save("original-run", original)
	grants := []executionconfig.UsageGrant{}
	for _, r := range missing.AccessRequirements {
		if r.Environment != input.Execution.Target.Environment || r.SourceRef == "" || !executionconfig.SafeUsageRoot(r.SessionRoot) {
			t.Fatal("unverified remote access requirement", r)
		}
		grants = append(grants, executionconfig.UsageGrant{Environment: r.Environment, HostID: r.HostID, SessionRoots: []string{r.SessionRoot}})
	}
	policy, _ := json.Marshal(map[string]any{"enabled": true, "remote_log_access": map[string]any{"mode": "temporary", "grants": grants}})
	_, err = executionconfig.DecodeUsagePolicy(policy)
	require(t, err)
	stale := cli
	stopService()
	start(policy)
	if _, e := stale.RunUsage(ctx, runID); e == nil {
		t.Fatal("old instance silently rebound")
	}
	var usage runnercontract.UsageSnapshot
	deadline = time.Now().Add(90 * time.Second)
	for {
		u, e := cli.RunUsage(ctx, runID)
		if e == nil {
			usage = u
		}
		if usage.Tokens != nil && usage.Tokens.Total != nil && *usage.Tokens.Total > 0 && usage.Coverage.Finalized && usage.AccessCleanupStatus != "pending" {
			break
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			save("authorized-usage", usage)
			t.Fatal("authorized remote usage did not converge", e, usage.Issues)
		}
		time.Sleep(time.Second)
	}
	save("authorized-usage", usage)
	events, err := cli.RunUsageEvents(ctx, usage)
	require(t, err)
	save("usage-events", events)
	fixed, err := cli.Call(ctx, "run-usage", []string{"--run-id", runID, "--snapshot-revision", fmt.Sprint(usage.Revision)}, nil)
	require(t, err)
	var again runnercontract.UsageSnapshot
	require(t, json.Unmarshal(fixed.Data, &again))
	if again.Tokens == nil || again.Tokens.Total == nil || *again.Tokens.Total != *usage.Tokens.Total || again.Revision != usage.Revision {
		t.Fatal("fixed snapshot changed")
	}
	inspected, err := cli.Inspect(ctx, "run-id", runID)
	require(t, err)
	if inspected.Run.InputDigest != original.InputDigest || inspected.Run.Execution.SessionID != original.Execution.SessionID || inspected.Run.Execution.TurnID != original.Execution.TurnID {
		t.Fatal("authorization restart changed original execution")
	}
	// Use conspicuously synthetic rates; missing request-time metadata stays unknown.
	prices := billing.Policy{Source: "synthetic live arithmetic fixture, not provider prices", Currency: "USD", Basis: billing.Basis, Rules: []billing.Rule{}}
	seen := map[string]bool{}
	for _, e := range events {
		if e.Provider == nil || e.Model == nil {
			continue
		}
		key := *e.Provider + "/" + *e.Model
		if seen[key] {
			continue
		}
		seen[key] = true
		prices.Rules = append(prices.Rules, billing.Rule{ID: fmt.Sprint(len(prices.Rules) + 1), Provider: *e.Provider, Model: *e.Model, From: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), Rates: billing.Rates{Input: "1", Read: "0.1", Write: "0", Output: "2"}})
	}
	_, err = (billingconfig.Store{Home: home}).Save("*", prices)
	require(t, err)
	view, err := readRunnerUsage(ctx, cli, "isolated-live-candidate", runID, home)
	require(t, err)
	if view.Usage == nil || view.Usage.Tokens == nil || view.Usage.Tokens.Total == nil || *view.Usage.Tokens.Total != *usage.Tokens.Total || view.CostReason != "" {
		t.Fatal("SteerLine lost independently acquired tokens", view)
	}
	save("steerline-view", view)
	stopService()
	start(initial)
	retained, err := cli.RunUsage(ctx, runID)
	require(t, err)
	if retained.Tokens == nil || retained.Tokens.Total == nil || *retained.Tokens.Total != *usage.Tokens.Total {
		t.Fatal("withdrawing permit erased known usage")
	}
	save("withdrawn-usage", retained)
	t.Logf("same Run=%s responses=%d total=%d; explicit grant/restart/dedup/withdrawal passed", runID, usage.Coverage.CountedResponses, *usage.Tokens.Total)
}
