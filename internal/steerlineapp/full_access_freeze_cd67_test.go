package steerlineapp

import (
	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
	"testing"
)

func TestFullAccessProfileFreezeSurvivesClaimValidation(t *testing.T) {
	for _, host := range []string{"local", "orca"} {
		policy := workspacePolicyFixture()
		policy.Filesystem.WriteScope, policy.Filesystem.GitMetadataWrite = "host", false
		c := Config{PublicationPolicy: executionPublicationFixture(), PR: hostPublicationProviderFixture(), ExecutionPolicy: policy, Repository: "repository", Branch: "main", Host: host, Agent: "codex"}
		if host == "orca" {
			c.PublicationPolicy.Publisher = "worker"
		}
		c = bindUnitDirectory(t, c)
		var terminal *orcaworker.ProfileConfig
		if host == "orca" {
			terminal = &orcaworker.ProfileConfig{Executable: "orca", Shell: "powershell.exe", WaitTimeout: "1m"}
		}
		profile := testExecutionProfile(t, ProjectExecutionProfileID, "Full access", host, "codex", codexworker.ProfileConfig{Executable: "codex"}, terminal)
		// This explicit current policy has no native Windows sandbox. Keep its
		// exact input rather than the helper's workspace-only default.
		raw, err := codexworker.EncodeProfileConfig(codexworker.ProfileConfig{Executable: "codex"})
		require(t, err)
		profile.ImplementationConfigs[0].Payload = raw
		c = c.ApplyExecutionProfile(boundProfile(c, profile))
		bound := unitProjectProfile(c)
		original := bound.Freeze()
		if bound.Digest() != original.Digest {
			t.Fatalf("%s selection digest differs: selected=%q frozen=%q availability=%q policy=%+v inputs=%+v", host, bound.Digest(), original.Digest, unitProjectProfile(c).StaticUnavailableReason(), c.ExecutionPolicy, unitProjectProfile(c).ImplementationConfigs)
		}
		if !testVerifyFixedExecutionProfile(original) {
			t.Fatalf("%s full-access snapshot rejected at claim: %+v", host, original)
		}
		restored := c.ApplyFixedExecutionProfile(original)
		if restored.ExecutionDirectory != c.ExecutionDirectory {
			t.Fatal("frozen profile lost its injected execution directory")
		}
		if restored.ExecutionPolicy.Filesystem.WriteScope != "host" || codexProfileSettings(restored).WindowsSandbox != "" {
			t.Fatal("frozen full-access authority was not restored")
		}
		original.ImplementationData[0].Payload[0] = '['
		if testVerifyFixedExecutionProfile(original) {
			t.Fatal("full-access snapshot accepted a changed Windows sandbox mode")
		}
	}
}
