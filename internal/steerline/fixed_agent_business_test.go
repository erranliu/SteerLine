package steerline

import (
	"testing"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

// One neutral implementation registers two explicit agent identities. Both
// use the same codec and ports; changing a default cannot redirect a fixed run.
func TestFixedProfileRetainsSelectedAgent(t *testing.T) {
	directory, err := newNeutralDirectory("fixture", "alternative")
	require(t, err)
	for _, agent := range []string{"fixture", "alternative"} {
		base := withNeutralProfile(Config{ExecutionPolicy: workspacePolicyFixture(), PublicationPolicy: executionPublicationFixture(), PR: hostPublicationProviderFixture()}, neutralSettings{Executable: "fixture"})
		base.ExecutionDirectory, base.Agent = directory, agent
		profile := ExecutionProfile{ID: "fixture-" + agent, Name: "Fixture " + agent, Host: "local", Agent: agent,
			ImplementationConfigs: []executionapi.ImplementationConfigInput{{ImplementationKey: neutralProfileKey, CodecVersion: 1, Payload: mustJSON(neutralSettings{Executable: "fixture"})}}}
		profile = base.bindProjectPolicy(profile)
		base.ExecutionProfiles = []ExecutionProfile{profile}
		require(t, validateExecutionProfiles(base))
		frozen := profile.Freeze()
		require(t, base.CheckFixedProfile(frozen))
		base.Agent = "changed-default"
		selected := base.ApplyFixedExecutionProfile(frozen)
		if selected.Agent != agent {
			t.Fatalf("fixed agent changed to the new default: got %q want %q", selected.Agent, agent)
		}
		entry, err := selected.ExecutionDirectory.Resolve(selected)
		require(t, err)
		if entry.Key.Agent != agent || entry.Key.Protocol != neutralProtocol {
			t.Fatal("fixed profile resolved a different registration")
		}
	}
}
