package app

import (
	"context"
	"encoding/json"
	"github.com/erranliu/SteerLine/internal/executionconfig"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstate"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/runnerclient"
)

func TestUsageCollectionOnlyReadsOwnedEffectiveService(t *testing.T) {
	exe, err := os.Executable()
	require(t, err)
	state := filepath.Join(t.TempDir(), "never-created")
	cli := runnerclient.Client{Executable: exe, StateDir: state, InstanceID: "owned", BuildVersion: "fixture", ResourceDigest: "resource"}
	info := runnerclient.ServiceInfo{Protocol: runnerclient.Protocol, Status: "ready", StateDir: state, InstanceID: cli.InstanceID, Version: cli.BuildVersion, ResourceDigest: cli.ResourceDigest, UsagePolicy: json.RawMessage(`{"enabled":true,"remote_log_access":{"mode":"disabled","grants":[]}}`), UsagePolicyDigest: "digest", UsagePolicyRevision: "owned"}
	for _, mode := range []string{"ready", "foreign-instance", "foreign-state", "foreign-build", "missing-policy", "offline", "unbound"} {
		t.Run(mode, func(t *testing.T) {
			current, client := info, cli
			switch mode {
			case "foreign-instance":
				current.InstanceID = "replacement"
			case "foreign-state":
				current.StateDir += "-other"
			case "foreign-build":
				current.Version = "other"
			case "missing-policy":
				current.UsagePolicyRevision = ""
			case "offline":
				current.Status = "not_running"
			case "unbound":
				client.InstanceID = ""
			}
			body, _ := json.Marshal(map[string]any{"service": current})
			t.Setenv("STEERLINE_USAGE_QUERY_FIXTURE", string(body))
			calls := filepath.Join(t.TempDir(), "calls")
			t.Setenv("STEERLINE_USAGE_QUERY_CALLS", calls)
			d := &ApplicationDirectory{runner: applicationRunner{client: client}}
			out, queryErr := d.usageCollection(context.Background(), false)
			if (queryErr == nil) != (mode == "ready") || queryErr == nil && (out.Status != "ready" || out.Policy.RemoteLogAccess.Mode != "disabled" || len(out.Requirements) != 0) {
				t.Fatal(out, queryErr)
			}
			wire, _ := os.ReadFile(calls)
			if mode == "unbound" && len(wire) != 0 || mode != "unbound" && (!strings.HasPrefix(string(wire), "service\nstatus\n") || strings.Contains(string(wire), "run-usage") || strings.Contains(string(wire), "start")) {
				t.Fatal("status query crossed its read-only boundary", string(wire))
			}
			if _, err := os.Stat(state); !os.IsNotExist(err) {
				t.Fatal("query created a Runner state directory", err)
			}
		})
	}
}

func TestUsageCollectionBudgetPreservesVerifiedRequirements(t *testing.T) {
	exe, err := os.Executable()
	require(t, err)
	cli := runnerclient.Client{Executable: exe, StateDir: t.TempDir()}
	u := runnercontract.UsageSnapshot{Schema: "run-usage/v1", RunID: "original", Revision: 1, Scope: "main_session", Status: "unavailable", ObservedAt: time.Now().UTC(), AccessRequirements: []runnercontract.UsageAccessRequirement{{SourceRef: "verified", Environment: "paired", HostID: "sha256:" + strings.Repeat("a", 64), SessionRoot: `C:\Users\fixture\.codex\sessions\2026\11\05`, ObservedAt: time.Now().UTC()}}}
	body, _ := json.Marshal(map[string]any{"run-usage": u})
	t.Setenv("STEERLINE_USAGE_QUERY_FIXTURE", string(body))
	calls := filepath.Join(t.TempDir(), "calls")
	t.Setenv("STEERLINE_USAGE_QUERY_CALLS", calls)
	candidates := []runnerstate.Candidate{{RunID: "original", StateDir: cli.StateDir}}
	candidates = append(candidates, runnerstate.Candidate{RunID: "beyond-budget", StateDir: cli.StateDir})
	out := executionconfig.UsageCollection{Status: "ready"}
	// Earlier projects used 63 reads; exercise the final allowed and first denied read.
	reads := 63
	if readUsageRequirements(context.Background(), cli, candidates, &reads, &out) || reads != 64 || out.Diagnostic == "" || out.Status != "ready" || len(out.Requirements) != 1 || out.Requirements[0].SourceRef != "verified" {
		t.Fatal("budget discarded verified facts", out, reads)
	}
	wire, err := os.ReadFile(calls)
	require(t, err)
	if strings.Count(string(wire), "run-usage\n") != 1 {
		t.Fatal("query budget not enforced")
	}
}
