package runnerclient

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLegacyCheckRetirementStoppedProofCoversRuns(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("STEERLINE_RUNNER_CLI_FIXTURE", "check-response")
	t.Setenv("STEERLINE_RUNNER_CLI_CALL", filepath.Join(t.TempDir(), "call"))
	c := Client{Executable: exe, StateDir: t.TempDir(), Timeout: 5 * time.Second}
	for _, tc := range []struct {
		name  string
		alter func(*StoppedVerification)
		pass  bool
	}{
		{"all", func(*StoppedVerification) {}, true},
		{"absent_runs", func(p *StoppedVerification) { p.Runs = nil }, false},
		{"empty_runs", func(p *StoppedVerification) { p.Runs = []StoppedRun{} }, true},
		{"duplicate", func(p *StoppedVerification) { p.Runs = append(p.Runs, p.Runs[0]) }, false},
		{"unsettled", func(p *StoppedVerification) { p.Runs[0].ResourcesSettled = false }, false},
		{"missing_digest", func(p *StoppedVerification) { p.Runs[0].InputDigest = "" }, false},
		{"other_home", func(p *StoppedVerification) { p.StateDir += "-other" }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := StoppedVerification{StateDir: c.StateDir, ResourcesSettled: true, Runs: []StoppedRun{{RunID: "original", InputDigest: "digest", ResourcesSettled: true}}}
			tc.alter(&p)
			raw, _ := json.Marshal(p)
			t.Setenv("STEERLINE_CHECK_RESPONSE", `{"protocol":"steerrunner-cli/v1","ok":true,"request_id":null,"data":`+string(raw)+`,"error":null}`)
			if _, err := c.VerifyStopped(context.Background()); (err == nil) != tc.pass {
				t.Fatal(err)
			}
		})
	}
}
