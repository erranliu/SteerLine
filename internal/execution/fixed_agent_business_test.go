package execution

import (
	"testing"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

// One neutral implementation registers two explicit agent identities. Both
// use the same codec and ports; changing a default cannot redirect a fixed run.
func TestFixedProfileRetainsSelectedAgent(t *testing.T) {
	directory, err := newNeutralDirectory("fixture", "alternative")
	require(t, err)
	for _, agent := range []string{"fixture", "alternative"} {
		base := Config{PublicationPolicy: executionPublicationFixture(), PR: hostPublicationProviderFixture(), Host: "local", Agent: agent, WorkflowProtocol: executionmodel.Version}
		base.BindAcceptedConfigVersion("fixture-accepted-v1")
		profile := ExecutionProfile{ID: "fixture-" + agent, Name: "Fixture " + agent, Host: "local", Agent: agent,
			ImplementationConfigs: []executionbackend.ImplementationConfigInput{{ImplementationKey: neutralProfileKey, CodecVersion: 1, Payload: mustJSON(neutralSettings{Executable: "fixture"})}}}
		catalog := ProfileCatalog{Base: base, Directory: directory, Profiles: []ExecutionProfile{profile}, DefaultID: profile.ID, Global: true}
		require(t, catalog.Validate())
		selected, selectedProfile, err := catalog.ResolveExecutionProfile(profile.ID, "")
		require(t, err)
		frozen := selectedProfile.Freeze(directory, selected.Identity())
		if !VerifyFixedExecutionProfile(frozen, directory) {
			t.Fatal("selected profile did not freeze")
		}
		selected.Agent = "changed-default"
		selected = selected.ApplyFixedExecutionProfile(frozen, directory)
		if selected.Agent != agent {
			t.Fatalf("fixed agent changed to the new default: got %q want %q", selected.Agent, agent)
		}
		entry, err := directory.Resolve(selected)
		require(t, err)
		if entry.Key.Agent != agent || entry.Key.Protocol != neutralProtocol {
			t.Fatal("fixed profile resolved a different registration")
		}
	}
}
