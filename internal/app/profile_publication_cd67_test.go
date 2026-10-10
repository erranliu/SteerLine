package app

import (
	"encoding/json"
	"testing"

	"github.com/erranliu/SteerLine/internal/execution"
	"github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	remote "github.com/erranliu/SteerLine/internal/execution/adapters/orcatransport"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func mixedPublicationProfiles() []ExecutionProfile {
	localCodex, _ := codexworker.EncodeProfileConfig(codexworker.ProfileConfig{Executable: "codex.exe"})
	orcaOptions, _ := json.Marshal(remote.Profile{Executable: "orca.exe", RepositorySelector: "id:repo", WaitTimeout: "30s"})
	return []ExecutionProfile{
		{ID: "local", Name: "Local", Host: "local", Agent: "codex", ImplementationConfigs: []executionbackend.ImplementationConfigInput{{ImplementationKey: codexworker.ProfileCodecKey, CodecVersion: 2, Payload: localCodex}}},
		{ID: "orca", Name: "Orca", Host: "orca", Agent: "codex", ImplementationConfigs: []executionbackend.ImplementationConfigInput{{ImplementationKey: remote.ProfileKey, CodecVersion: remote.ProfileVersion, Payload: orcaOptions}}},
	}
}

func TestSimplifiedWorkerOnlyPublication(t *testing.T) {
	for _, publisher := range []string{"host_service", "execution_profile", ""} {
		if (executionbackend.PublicationPolicy{Publisher: publisher}).Validate() == nil {
			t.Fatalf("retired publisher accepted: %q", publisher)
		}
	}
	if err := (executionbackend.PublicationPolicy{Publisher: executionbackend.PublisherWorker}).Validate(); err != nil {
		t.Fatal(err)
	}
	var profile execution.ExecutionProfile
	if err := decodeStrictJSON([]byte(`{"id":"local","name":"Local","publication_policy":{"publisher":"worker"}}`), &profile); err == nil {
		t.Fatal("per-profile publication override accepted")
	}
}
