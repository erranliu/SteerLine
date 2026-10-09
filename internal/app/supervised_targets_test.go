package app

import (
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/execution"
	orcaworker "github.com/erranliu/SteerLine/internal/execution/adapters/orca"
	remote "github.com/erranliu/SteerLine/internal/execution/adapters/orcaremote"
	b "github.com/erranliu/SteerLine/internal/execution/backend"
)

func TestSupervisedTargetsShareRegistrationAndRetiredLocalIsDisconnected(t *testing.T) {
	d, err := NewBuiltinDirectory(NewProcessHost(time.Second))
	require(t, err)
	entry, err := d.Resolve(execution.Config{Host: "orca_supervised", Agent: "codex"})
	require(t, err)
	for _, target := range []string{"local", "environment"} {
		p := remote.ProfileConfig{Target: target, Executable: "orca", Shell: "pwsh", WaitTimeout: "2m", PollInterval: "1s", CapabilityReportVersion: 1}
		if target == "environment" {
			p.ConnectionID = "pair"
			p.EnvironmentID = "env"
			p.RepositorySelector = "id:repo"
		}
		payload, err := remote.EncodeProfileConfig(p)
		require(t, err)
		data, _, err := d.NormalizeProfile([]b.ImplementationConfigInput{{ImplementationKey: remote.ProfileCodecKey, CodecVersion: remote.ProfileCodecVersion, Payload: payload}}, "orca_supervised", "codex", "project", "profile", "accepted")
		require(t, err)
		setup := b.LifecycleSetup{ProfileData: data, ProfileID: "profile", AcceptedConfigVersion: "accepted", RepositoryPath: "C:/repo", ProcessHost: NewProcessHost(time.Second)}
		setup.Snapshot.Ref = b.ExecutionRef{Backend: b.BackendOrca, Protocol: b.ProtocolOrcaSupervised, Agent: b.AgentSpec{Name: "codex"}, Target: (remote.Config{Settings: remote.Settings{Target: target, ConnectionID: p.ConnectionID, EnvironmentID: p.EnvironmentID}}).Target()}
		setup.Snapshot.Session.ProjectID = "project"
		port, err := entry.Lifecycle(setup)
		require(t, err)
		if _, ok := port.(*remote.ExecutionLifecycle); !ok {
			t.Fatal("target selected a second lifecycle")
		}
		if entry.LocalWorkspace || port.PolicyCapabilities().ServiceStopBehavior != remote.ServiceStopBehavior {
			t.Fatal("local target inherited retired local behavior")
		}
	}
	if _, err := d.Resolve(execution.Config{Host: "orca", Agent: "codex"}); err == nil {
		t.Fatal("retired local host still registered")
	}
	if _, err := d.ResolveIdentity(execution.ExecutionRegistrationKey{Backend: b.BackendOrca, Agent: "codex", Protocol: b.ProtocolOrcaTerminalV2}); err == nil {
		t.Fatal("retired frozen identity still registered")
	}
	legacy, _ := orcaworker.EncodeProfileConfig(orcaworker.ProfileConfig{Executable: "orca", Shell: "pwsh", WaitTimeout: "2m"})
	if _, _, err := d.NormalizeProfile([]b.ImplementationConfigInput{{ImplementationKey: orcaworker.ProfileCodecKey, CodecVersion: 1, Payload: legacy}}, "orca_supervised", "codex", "project", "profile", "accepted"); err == nil {
		t.Fatal("old local codec silently converted to supervised")
	}
	if err := validateCatalogRef(b.ExecutionRef{Backend: b.BackendOrca, Protocol: b.ProtocolOrcaTerminalV2, External: b.ProcessIdentity{Kind: "orca", Terminal: "terminal", RuntimeID: "runtime", Incarnation: "inc"}}); err == nil {
		t.Fatal("retired resource accepted by app")
	}
}
