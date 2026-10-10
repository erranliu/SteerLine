package native

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemoteProfileOnlyUsesOrcaOwnedCodec(t *testing.T) {
	configs, err := RemoteProfileConfigs("orca", RemoteManifest{EnvironmentID: "env", ConnectionID: "peer", RepositorySelector: "id:repo"})
	if err != nil {
		t.Fatal(err)
	}
	if len(configs) != 1 || configs[0].ImplementationKey != "orca.supervised-profile" {
		t.Fatalf("provider implementation leaked into remote fixture: %+v", configs)
	}
	var fields map[string]any
	if json.Unmarshal(configs[0].Payload, &fields) != nil {
		t.Fatal("bad payload")
	}
	if fields["executable"] != "orca" {
		t.Fatal("fixture pinned an agent executable")
	}
}

func TestRemoteScenarioDoesNotReadProviderPrivateFiles(t *testing.T) {
	for _, name := range []string{filepath.Join("..", "testenv", "remote.go"), "remote.go"} {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"CODEX_HOME", "session_meta", "turn_context", "RolloutUsage", "BusinessRolloutMatches", "codex --version", "codex.worker-profile"} {
			if strings.Contains(string(raw), forbidden) {
				t.Fatalf("remote scenario reintroduced private provider dependency %q in %s", forbidden, name)
			}
		}
	}
}
