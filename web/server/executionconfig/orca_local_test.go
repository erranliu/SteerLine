package executionconfig

import (
	"context"
	"encoding/json"
	cfg "github.com/erranliu/SteerLine/internal/executionconfig"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"testing"
)

func TestLocalOrcaConfiguration(t *testing.T) {
	s, _ := fixture(t)
	snap, err := s.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range snap.Combinations {
		if c.Agent == "dsh" {
			t.Fatal("retired agent inferred from permissive schema")
		}
	}
	for _, agent := range []string{"codex", "pi"} {
		selection := runnercontract.ExecutionSelection{Adapter: "orca-terminal", Target: runnercontract.ExecutionTarget{Kind: "local"}, Agent: agent, AccessMode: "full_access"}
		result, err := s.Compose(context.Background(), ComposeRequest{SavedDigest: snap.SavedDigest, Workflow: runnercontract.Workflow{ID: "development", Version: 2}, Selection: selection})
		if err != nil || result.Execution == nil {
			t.Fatal(agent, err)
		}
	}
	// Retired fields fail the fixed release's real schema; no helper deployment gate.
	cat, err := s.contract(context.Background(), snap.Config.Runner)
	if err != nil {
		t.Fatal(err)
	}
	p := cfg.Profile{Execution: runnercontract.ExecutionInput{Adapter: "orca-terminal", Target: runnercontract.ExecutionTarget{Kind: "local"}, Settings: json.RawMessage(`{"access_mode":"full_access","agent_launch":{"runtime_executable":"C:/fake.exe","runtime_sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}`)}, Workflow: runnercontract.Workflow{ID: "development", Version: 2}}
	if cat.Validate(p) == nil {
		t.Fatal("retired helper accepted")
	}
}
