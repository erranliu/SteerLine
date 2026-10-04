package native

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erranliu/SteerLine/internal/steerline/orca_remote_worker"
)

func TestCandidateAdmissionRejectsSubstitutionBeforeResources(t *testing.T) {
	r := &orca_remote_worker.ProfileConfig{ConnectionID: "remote-connection", EnvironmentID: "remote-environment", RepositorySelector: "id:remote-repository", CoordinatorTerminal: "coordinator", ExpectedCodexVersion: SupportedCodexVersion, PollInterval: "100ms", CapabilityReportVersion: 1}
	m := CandidateManifest{Version: 1, SourceCommit: strings.Repeat("a", 40), Repository: "erranliu/SteerlineTest", Profiles: []CandidateProfile{{ID: "local", Chain: "local-codex"}, {ID: "remote", Chain: "orca-remote", Remote: r}}, Scenarios: map[string][]string{"candidates.mixed": {"local", "remote"}}}
	// Nonexistent source/auth proves the remote gate precedes local admission.
	err := m.Preflight(context.Background(), "missing-root", "missing-auth", "candidates.mixed")
	if err == nil || !strings.Contains(err.Error(), "lacks remote worker-start enforcement evidence") || !strings.Contains(err.Error(), "remote-environment") {
		t.Fatalf("remote gate lost target diagnosis: %v", err)
	}
	m.Scenarios["candidates.local"] = []string{"local", "remote"}
	if _, err := m.Selection("candidates.local"); err == nil {
		t.Fatal("local substituted for remote composition")
	}
	m.Scenarios["candidates.remote"] = []string{"remote", "remote"}
	path := filepath.Join(t.TempDir(), "manifest.json")
	raw, _ := json.Marshal(m)
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadCandidateManifest(path); err == nil {
		t.Fatal("duplicate scenario identity accepted")
	}
}
