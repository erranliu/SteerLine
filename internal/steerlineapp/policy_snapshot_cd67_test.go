package steerlineapp

import (
	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	"testing"
)

func TestProjectPolicyFrozenSnapshotRejectsDamageAndPreservesAuthority(t *testing.T) {
	c := Config{PublicationPolicy: executionPublicationFixture(), PR: hostPublicationProviderFixture(), ExecutionPolicy: workspacePolicyFixture(), Repository: "repository", Branch: "main", Host: "local", Agent: "codex"}
	setTestCodexProfile(t, &c, codexworker.ProfileConfig{Executable: "codex"})
	original := unitProjectProfile(c).Freeze()
	if !testVerifyFixedExecutionProfile(original) {
		t.Fatalf("valid snapshot rejected: %+v", original)
	}
	c.ExecutionPolicy.Network = "deny"
	codex := codexProfileSettings(c)
	codex.WindowsSandbox = "unelevated"
	setTestCodexProfile(t, &c, codex)
	restored := c.ApplyFixedExecutionProfile(original)
	if restored.ExecutionPolicy.Network != "allow" || codexProfileSettings(restored).WindowsSandbox != "elevated" {
		t.Fatal("current config rewrote existing authority")
	}
	for _, mutate := range []func(*FixedExecutionProfile){
		func(p *FixedExecutionProfile) { p.Version = 1 }, func(p *FixedExecutionProfile) { p.ExecutionPolicy.Network = "deny" }, func(p *FixedExecutionProfile) { p.ImplementationData[0].Payload[0] = '[' }, func(p *FixedExecutionProfile) { p.AcceptedConfigVersion = "edited" }, func(p *FixedExecutionProfile) { p.PolicyDigest = "edited" },
	} {
		copy := original
		mutate(&copy)
		if testVerifyFixedExecutionProfile(copy) {
			t.Fatalf("damaged snapshot accepted: %+v", copy)
		}
	}
}
