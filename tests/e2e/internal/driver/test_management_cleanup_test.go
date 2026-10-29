//go:build e2e

package driver

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/erranliu/SteerLine/internal/app"
	"github.com/erranliu/SteerLine/internal/runnerclient"
	feedback "github.com/erranliu/SteerLine/internal/testkit/feedback"
)

// Postflight on one already settled owned live package; never runs a model or
// restarts a service. Historical proof.json is left unchanged.
func TestDebugTestManagementOwnedBranchCleanup(t *testing.T) {
	root := os.Getenv("STEERLINE_TM_CLEANUP_ROOT")
	if root == "" {
		t.Skip("requires a settled test-management package")
	}
	d, err := openDebugRelease(root)
	if err != nil {
		t.Fatal(err)
	}
	read := func(name string, target any) {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(root, "evidence", "test-management", name))
		if err != nil {
			t.Fatal(err)
		}
		if err = json.Unmarshal(raw, target); err != nil {
			t.Fatal(err)
		}
	}
	var proof feedback.TestManagementProof
	var run runnerclient.Run
	read("proof.json", &proof)
	read("candidate-run.json", &run)
	if proof.Validate() != nil || run.RunID != proof.CandidateRun {
		t.Fatal("original live proof incomplete")
	}
	out := filepath.Join(root, "evidence", "test-management-owned-cleanup")
	if err = os.MkdirAll(out, 0700); err != nil {
		t.Fatal(err)
	}
	raw, err := d.tmCommand(out, "verify-stop", d.binary, "verify-stop", "--runner-exe", filepath.Join(root, "bin", "steerrunner.exe"))
	if err != nil {
		t.Fatal(err)
	}
	var stopped app.ApplicationStopVerification
	if err = json.Unmarshal(raw, &stopped); err != nil {
		t.Fatal(err)
	}
	matched := false
	for _, original := range stopped.Runner.Runs {
		if original.RunID == run.RunID && original.InputDigest == run.InputDigest && original.ResourcesSettled {
			matched = true
		}
	}
	if !stopped.Runner.ResourcesSettled || !matched {
		t.Fatal("original Run settlement unverified")
	}
	if err = d.tmCleanupOwnedRefs(out, proof.Commit, run.Execution.Branch, run.RunID); err != nil {
		t.Fatal(err)
	}
}
