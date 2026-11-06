package app

import (
	"encoding/json"
	nativefixture "github.com/erranliu/SteerLine/internal/testkit/protocol"
	"testing"

	env "github.com/erranliu/SteerLine/internal/environmentadaptation/model"
	"github.com/erranliu/SteerLine/internal/executionconfig"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

// Cross-owner assertions live at the application composition boundary. The
// pure environment model must not import global execution configuration.
func TestEnvironmentContextAndGlobalTargetBoundaries(t *testing.T) {
	codex := runnercontract.ExecutionInput{Adapter: "codex-app-server", Target: runnercontract.ExecutionTarget{Kind: "local"}, Settings: json.RawMessage(`{"executable":"codex","model":"one","reasoning_effort":"medium","access_mode":"full_access"}`)}
	orca := runnercontract.ExecutionInput{Adapter: "orca-terminal", Target: runnercontract.ExecutionTarget{Kind: "local"}, Settings: json.RawMessage(`{"transport":{"executable":"orca"},"agent_launch":{"executable":"codex"},"access_mode":"full_access"}`)}
	if executionconfig.Target(codex) == "" {
		t.Fatal("local target bypass")
	}
	if executionconfig.Target(codex) == executionconfig.Target(orca) {
		t.Fatal("local adapters shared environment or occupancy")
	}
	for _, target := range []struct {
		adapter     string
		kind        string
		environment string
		valid       bool
	}{
		{"codex-app-server", "local", "", true},
		{"orca-terminal", "local", "", true},
		{"orca-terminal", "orca_environment", "remote-id", true},
		{"orca-terminal", "orca_environment", "", false},
		{"codex-app-server", "orca_environment", "remote-id", false},
		{"codex-app-server", "local", "unexpected-local-alias", false},
		{"orca-terminal", "local", "unexpected-local-alias", false},
		{"unknown", "local", "", false},
		{"codex-app-server", "unknown", "", false},
	} {
		execution := codex
		execution.Adapter = target.adapter
		execution.Target = runnercontract.ExecutionTarget{Kind: target.kind, Environment: target.environment}
		_, err := env.Context(execution, nativefixture.ExecutionIdentity(execution))
		if (err == nil) != target.valid || (executionconfig.Target(execution) != "") != target.valid {
			t.Fatalf("context/global target acceptance changed for %+v: %v", target, err)
		}
	}
}
