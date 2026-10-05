package app

import (
	"testing"

	"github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
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
	for _, mutate := range []func(*executionmodel.FixedExecutionProfile){
		func(p *executionmodel.FixedExecutionProfile) { p.Version = 1 }, func(p *executionmodel.FixedExecutionProfile) { p.ExecutionPolicy.Network = "deny" }, func(p *executionmodel.FixedExecutionProfile) { p.ImplementationData[0].Payload[0] = '[' }, func(p *executionmodel.FixedExecutionProfile) { p.AcceptedConfigVersion = "edited" }, func(p *executionmodel.FixedExecutionProfile) { p.PolicyDigest = "edited" },
	} {
		copy := original
		copy.ImplementationData = append(copy.ImplementationData[:0:0], original.ImplementationData...)
		for i := range copy.ImplementationData {
			copy.ImplementationData[i] = copy.ImplementationData[i].Clone()
		}
		mutate(&copy)
		if testVerifyFixedExecutionProfile(copy) {
			t.Fatalf("damaged snapshot accepted: %+v", copy)
		}
		if !testVerifyFixedExecutionProfile(original) {
			t.Fatal("damage fixture changed the original snapshot for later cases")
		}
	}
}
