package runnerbinding

import (
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/runnerstate"
	"path/filepath"
	"testing"
)

func TestRunnerNamespaceBindings(t *testing.T) {
	pin := runnercontract.BuildPin{StateDir: t.TempDir(), SHA256: "new"}
	old := pin
	old.SHA256 = "old"
	c := runnerstate.Candidate{RunnerBuild: old, StateDir: pin.StateDir, RunID: "original", InputDigest: "original"}
	if err := ValidateRestartBindings(pin, []runnerstate.Candidate{c}); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*runnerstate.Candidate){func(c *runnerstate.Candidate) { c.StateDir = filepath.Join(pin.StateDir, "foreign") }, func(c *runnerstate.Candidate) { c.InputDigest = "" }, func(c *runnerstate.Candidate) { c.RunnerBuild.StateDir = "foreign" }} {
		bad := c
		change(&bad)
		if ValidateBuildBindings(pin, []runnerstate.Candidate{bad}) == nil {
			t.Fatal("foreign binding admitted")
		}
	}
	if SameBuild(pin, old) {
		t.Fatal("provenance comparison lost original build")
	}
}
