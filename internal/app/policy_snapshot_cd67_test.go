package app

import (
	"testing"

	"github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

func TestProjectPolicyFrozenSnapshotRejectsDamageAndPreservesAuthority(t *testing.T) {
	c := Config{PublicationPolicy: executionPublicationFixture(), PR: hostPublicationProviderFixture(), Repository: "repository", Branch: "main", Host: "local", Agent: "codex"}
	setTestCodexProfile(t, &c, codexworker.ProfileConfig{Executable: "codex"})
	original := unitProjectProfile(c).Freeze()
	if !testVerifyFixedExecutionProfile(original) {
		t.Fatalf("valid snapshot rejected: %+v", original)
	}
	codex := codexProfileSettings(c.executionConfig())
	codex.Model = "changed-model"
	setTestCodexProfile(t, &c, codex)
	restored := c.ApplyFixedExecutionProfile(original)
	if codexProfileSettings(restored.executionConfig()).Model == "changed-model" {
		t.Fatal("current config rewrote existing authority")
	}
	for _, mutate := range []func(*executionmodel.FixedExecutionProfile){
		func(p *executionmodel.FixedExecutionProfile) { p.Version = 1 }, func(p *executionmodel.FixedExecutionProfile) { p.ImplementationVersion = "edited" }, func(p *executionmodel.FixedExecutionProfile) { p.ImplementationData[0].Payload[0] = '[' }, func(p *executionmodel.FixedExecutionProfile) { p.AcceptedConfigVersion = "edited" }, func(p *executionmodel.FixedExecutionProfile) { p.Digest = "edited" },
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
