package runnerclient

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoppedVerificationKeepsRunDiagnosticsAndGate(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	t.Setenv("STEERLINE_RUNNER_CLI_FIXTURE", "check-response")
	t.Setenv("STEERLINE_RUNNER_CLI_CALL", filepath.Join(dir, "call"))
	c := Client{Executable: exe, StateDir: dir}
	for _, pass := range []bool{false, true} {
		proof := StoppedVerification{StateDir: dir, ResourcesSettled: pass, Runs: []StoppedRun{{RunID: "original", InputDigest: "input", ResourcesSettled: pass}}}
		if !pass {
			proof.Runs[0].Diagnostic = "run original: original terminal is still active"
		}
		raw, _ := json.Marshal(proof)
		t.Setenv("STEERLINE_CHECK_RESPONSE", `{"protocol":"steerrunner-cli/v1","ok":true,"request_id":null,"data":`+string(raw)+`,"error":null}`)
		got, err := c.VerifyStopped(context.Background())
		if (err == nil) != pass || got.ResourcesSettled != pass {
			t.Fatal(got, err)
		}
		if !pass && !strings.Contains(err.Error(), proof.Runs[0].Diagnostic) {
			t.Fatal("per-run diagnostic lost", err)
		}
	}
}
