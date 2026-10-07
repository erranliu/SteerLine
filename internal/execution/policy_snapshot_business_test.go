package execution

import (
	"testing"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

func TestProjectPolicyFrozenSnapshotRejectsDamageAndPreservesAuthority(t *testing.T) {
	c := workerContextConfigFixture()
	setNeutralProfile(t, &c, neutralSettings{Executable: "fixture-agent", Model: "original"})
	dependencies, _ := neutralFixtureRuntime(c)
	original := testProjectExecutionProfile(c).Freeze(dependencies.Directory, ConfigurationProjectIdentity(c))
	if !neutralVerifyFixedExecutionProfile(original) {
		t.Fatal("valid neutral fixed snapshot rejected")
	}
	setNeutralProfile(t, &c, neutralSettings{Executable: "fixture-agent", Model: "changed"})
	restored := c.ApplyFixedExecutionProfile(original, dependencies.Directory)
	if neutralProfileSettings(restored).Model != "original" {
		t.Fatal("current configuration rewrote original authority")
	}
	for _, mutate := range []func(*executionmodel.FixedExecutionProfile){
		func(p *executionmodel.FixedExecutionProfile) { p.Version = 1 }, func(p *executionmodel.FixedExecutionProfile) { p.ImplementationVersion = "edited" }, func(p *executionmodel.FixedExecutionProfile) { p.ImplementationData[0].Payload[0] = '[' }, func(p *executionmodel.FixedExecutionProfile) { p.AcceptedConfigVersion = "edited" }, func(p *executionmodel.FixedExecutionProfile) { p.Digest = "edited" },
	} {
		candidate := original
		candidate.ImplementationData = CloneImplementationData(original.ImplementationData)
		mutate(&candidate)
		if neutralVerifyFixedExecutionProfile(candidate) {
			t.Fatal("damaged fixed snapshot accepted")
		}
	}
}
