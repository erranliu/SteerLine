package driver

import (
	"testing"

	"github.com/erranliu/SteerLine/internal/runnerclient"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"github.com/erranliu/SteerLine/internal/testmanagement"
)

func TestTMGenerationResetRequiresOriginalSettlement(t *testing.T) {
	g := testmanagement.Generation{Project: "project", Runner: runnercontract.BuildPin{StateDir: "original"}, State: "unknown"}
	proof := map[string]runnerclient.StoppedRun{"run": {RunID: "run", InputDigest: "digest", ResourcesSettled: true}}
	check := func() error { return generationResetSafety(g, "project", "original", proof) }
	if check() == nil {
		t.Fatal("unbound unknown submission erased")
	}
	g.RunID, g.InputDigest = "run", "digest"
	if err := check(); err != nil {
		t.Fatal(err)
	}
	for _, alter := range []func(*testmanagement.Generation){
		func(v *testmanagement.Generation) { v.Project = "other" },
		func(v *testmanagement.Generation) { v.Runner.StateDir = "other" },
		func(v *testmanagement.Generation) { v.InputDigest = "different" },
		func(v *testmanagement.Generation) { v.RunID = "different" },
	} {
		bad := g
		alter(&bad)
		if generationResetSafety(bad, "project", "original", proof) == nil {
			t.Fatal("unrelated proof accepted")
		}
	}
	g.State, g.RunID, g.InputDigest = "rejected", "", ""
	if err := check(); err != nil {
		t.Fatal("definite rejection needs no Run", err)
	}
}
