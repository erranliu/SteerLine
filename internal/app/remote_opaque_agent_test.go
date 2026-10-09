package app

import (
	"github.com/erranliu/SteerLine/internal/execution"
	remote "github.com/erranliu/SteerLine/internal/execution/adapters/orcaremote"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	"testing"
	"time"
)

func TestRemoteOrcaAgentIsOpaqueAndHasNoWorkerCodec(t *testing.T) {
	directory, err := NewBuiltinDirectory(NewProcessHost(time.Second))
	require(t, err)
	payload, err := remote.EncodeProfileConfig(remote.ProfileConfig{Executable: "orca", Shell: "powershell.exe", WaitTimeout: "2m", ConnectionID: "peer", EnvironmentID: "env", RepositorySelector: "id:repo", PollInterval: "1s", CapabilityReportVersion: 1})
	require(t, err)
	for _, agent := range []string{"codex", "claude", "custom-agent"} {
		entry, err := directory.Resolve(execution.Config{Host: "orca_supervised", Agent: agent})
		require(t, err)
		if entry.Key.Agent != agent || len(entry.ProfileCodecKeys) != 1 || entry.ProfileCodecKeys[0] != remote.ProfileCodecKey {
			t.Fatalf("agent selected provider implementation: %+v", entry)
		}
		fixed, err := directory.ResolveIdentity(entry.Key)
		require(t, err)
		if fixed.Key != entry.Key {
			t.Fatal("opaque agent fixed identity changed")
		}
		_, _, err = directory.NormalizeProfile([]executionbackend.ImplementationConfigInput{{ImplementationKey: remote.ProfileCodecKey, CodecVersion: remote.ProfileCodecVersion, Payload: payload}}, "orca_supervised", agent, "project", "profile", "accepted")
		require(t, err)
	}
	if _, err := directory.Resolve(execution.Config{Host: "local", Agent: "custom-agent"}); err == nil {
		t.Fatal("remote opaque routing escaped into local implementation")
	}
}
