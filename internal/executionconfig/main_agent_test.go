package executionconfig

import (
	"encoding/json"
	"testing"

	"github.com/erranliu/SteerLine/internal/runnercontract"
)

func TestMainAgentUsesOnlyConfiguredLocalCodexTransport(t *testing.T) {
	remote := Profile{Execution: runnercontract.ExecutionInput{Adapter: "codex-app-server", Target: runnercontract.ExecutionTarget{Kind: "orca_environment"}, Settings: json.RawMessage(`{"executable":"remote"}`)}}
	local := Profile{Execution: runnercontract.ExecutionInput{Adapter: "codex-app-server", Target: runnercontract.ExecutionTarget{Kind: "local"}, Settings: json.RawMessage(`{"executable":"local-codex.exe","model":"worker-model","access_mode":"full_access"}`)}}
	if MainAgentBase([]Profile{remote}) != nil {
		t.Fatal("remote transport became local fallback")
	}
	got := MainAgentBase([]Profile{remote, local})
	if got == nil || string(got.Settings) != string(local.Execution.Settings) {
		t.Fatal("configured local transport was lost")
	}
	got.Settings[0] = 'x'
	if !json.Valid(local.Execution.Settings) {
		t.Fatal("shared execution settings were mutated")
	}
}

func TestMainAgentDefaultsAndExplicitSelection(t *testing.T) {
	got, err := MainAgentSettings(nil)
	if err != nil || got.Model != "gpt-6.1-sol" || got.ReasoningEffort != "high" {
		t.Fatal(got, err)
	}
	for _, bad := range []MainAgent{{Model: "", ReasoningEffort: "high"}, {Model: " padded ", ReasoningEffort: "high"}, {Model: "model\ncommand", ReasoningEffort: "high"}, {Model: "model", ReasoningEffort: "unknown"}} {
		if _, err := MainAgentSettings(&bad); err == nil {
			t.Fatal("invalid model settings accepted", bad)
		}
	}
	explicit := MainAgent{Model: "selected", ReasoningEffort: "medium"}
	got, err = MainAgentSettings(&explicit)
	if err != nil || got != explicit {
		t.Fatal("explicit model was replaced", got, err)
	}
}
