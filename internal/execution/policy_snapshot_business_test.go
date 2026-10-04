package execution

import (
	"testing"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

func TestProjectPolicyFrozenSnapshotRejectsDamageAndPreservesAuthority(t *testing.T) {
	c := workerContextConfigFixture()
	setNeutralProfile(t, &c, neutralSettings{Executable: "fixture-agent", Model: "original"})
	original := c.projectExecutionProfile().Freeze(ConfigurationProjectIdentity(c))
	if !neutralVerifyFixedExecutionProfile(original) {
		t.Fatal("valid neutral fixed snapshot rejected")
	}
	c.ExecutionPolicy.Network = "deny"
	setNeutralProfile(t, &c, neutralSettings{Executable: "fixture-agent", Model: "changed"})
	restored := c.ApplyFixedExecutionProfile(original)
	if restored.ExecutionPolicy.Network != "allow" || neutralProfileSettings(restored).Model != "original" {
		t.Fatal("current configuration rewrote original authority")
	}
	for _, mutate := range []func(*executionmodel.FixedExecutionProfile){
		func(p *executionmodel.FixedExecutionProfile) { p.Version = 1 }, func(p *executionmodel.FixedExecutionProfile) { p.ExecutionPolicy.Network = "deny" }, func(p *executionmodel.FixedExecutionProfile) { p.ImplementationData[0].Payload[0] = '[' }, func(p *executionmodel.FixedExecutionProfile) { p.AcceptedConfigVersion = "edited" }, func(p *executionmodel.FixedExecutionProfile) { p.PolicyDigest = "edited" },
	} {
		candidate := original
		candidate.ImplementationData = CloneImplementationData(original.ImplementationData)
		mutate(&candidate)
		if neutralVerifyFixedExecutionProfile(candidate) {
			t.Fatal("damaged fixed snapshot accepted")
		}
	}
}
