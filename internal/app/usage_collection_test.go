package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/runnerclient"
)

func TestUsageCollectionOnlyReadsOwnedEffectiveService(t *testing.T) {
	exe, err := os.Executable()
	require(t, err)
	state := filepath.Join(t.TempDir(), "never-created")
	cli := runnerclient.Client{Executable: exe, StateDir: state, InstanceID: "owned", BuildVersion: "fixture", ResourceDigest: "resource"}
	info := runnerclient.ServiceInfo{Protocol: runnerclient.Protocol, Status: "ready", StateDir: state, InstanceID: cli.InstanceID, Version: cli.BuildVersion, ResourceDigest: cli.ResourceDigest, UsagePolicy: json.RawMessage(`{"enabled":true}`), UsagePolicyDigest: "digest", UsagePolicyRevision: "owned"}
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
			out, queryErr := d.usageCollection(context.Background())
			if (queryErr == nil) != (mode == "ready") || queryErr == nil && (out.Status != "ready" || !out.Policy.Enabled) {
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
