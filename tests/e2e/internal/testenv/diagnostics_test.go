//go:build e2e

package testenv

import (
	"encoding/json"
	"github.com/erranliu/SteerLine/internal/runnerbridge"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	"strings"
	"testing"
)

func TestNativeDiagnosticsKeepPublicIdentityWithoutPrivatePayloads(t *testing.T) {
	var snap Snapshot
	snap.Runner.Candidates = []runnerbridge.Candidate{{ID: "candidate", RunID: "original", Projection: &runnerclient.Run{RunID: "original"}}}
	raw, err := json.Marshal(publicNativeDiagnostics(snap))
	if err != nil || !strings.Contains(string(raw), "original") {
		t.Fatalf("missing original identity: %v", err)
	}
	for _, forbidden := range []string{`"submit"`, `"input"`, "auth.json", "config.toml", "state_dir", "stateDir"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("private payload escaped: %s", forbidden)
		}
	}
}
