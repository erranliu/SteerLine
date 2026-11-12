package executionconfig

import (
	"encoding/json"
	"testing"

	"github.com/erranliu/SteerLine/internal/runnercontract"
)

func TestSteerAgentUsesOnlyConfiguredLocalCodexTransport(t *testing.T) {
	remote := Profile{Execution: runnercontract.ExecutionInput{Adapter: "codex-app-server", Target: runnercontract.ExecutionTarget{Kind: "orca_environment"}, Settings: json.RawMessage(`{"executable":"remote"}`)}}
	local := Profile{Execution: runnercontract.ExecutionInput{Adapter: "codex-app-server", Target: runnercontract.ExecutionTarget{Kind: "local"}, Settings: json.RawMessage(`{"executable":"local-codex.exe","model":"worker-model","access_mode":"full_access"}`)}}
	if SteerAgentBase([]Profile{remote}) != nil {
		t.Fatal("remote transport became local fallback")
	}
	got := SteerAgentBase([]Profile{remote, local})
	if got == nil || string(got.Settings) != string(local.Execution.Settings) {
		t.Fatal("configured local transport was lost")
	}
	got.Settings[0] = 'x'
	if !json.Valid(local.Execution.Settings) {
		t.Fatal("shared execution settings were mutated")
	}
}

func TestSteerAgentDefaultsAndExplicitSelection(t *testing.T) {
	got, err := SteerAgentSettings(nil)
	if err != nil || got.Model != "gpt-6.1-sol" || got.ReasoningEffort != "high" {
		t.Fatal(got, err)
	}
	for _, bad := range []SteerAgent{{Model: "", ReasoningEffort: "high"}, {Model: " padded ", ReasoningEffort: "high"}, {Model: "model\ncommand", ReasoningEffort: "high"}, {Model: "model", ReasoningEffort: "unknown"}} {
		if _, err := SteerAgentSettings(&bad); err == nil {
			t.Fatal("invalid model settings accepted", bad)
		}
	}
	explicit := SteerAgent{Model: "selected", ReasoningEffort: "medium"}
	got, err = SteerAgentSettings(&explicit)
	if err != nil || got != explicit {
		t.Fatal("explicit model was replaced", got, err)
	}
}

func TestSteerAgentConfigurationKeepsExistingValueSemantics(t *testing.T) {
	for _, raw := range []string{`{}`, `{"steer_agent":null}`, `{"steer_agent":{"model":"chosen","reasoning_effort":"medium"}}`} {
		var c File[Pin]
		if err := Decode([]byte(raw), &c); err != nil {
			t.Fatal(raw, err)
		}
		got, err := SteerAgentSettings(c.SteerAgent)
		if err != nil {
			t.Fatal(raw, err)
		}
		want := SteerAgent{Model: "gpt-6.1-sol", ReasoningEffort: "high"}
		if c.SteerAgent != nil {
			want = SteerAgent{Model: "chosen", ReasoningEffort: "medium"}
		}
		if got != want {
			t.Fatal(raw, got)
		}
	}
	for _, raw := range []string{`{"continuation_suggestion":null}`, `{"main_agent":null}`, `{"main_agent":{"model":"chosen","reasoning_effort":"medium"}}`, `{"steer_agent":{"model":"chosen","reasoning_effort":"medium","extra":true}}`, `{"steer_agent":{"continuation":{"suggest":{}}}}`} {
		var c File[Pin]
		if err := Decode([]byte(raw), &c); err == nil {
			t.Fatal("unknown or obsolete configuration accepted", raw)
		}
	}
	for _, raw := range []string{`{"steer_agent":{}}`, `{"steer_agent":{"model":"chosen"}}`, `{"steer_agent":{"reasoning_effort":"high"}}`} {
		var c File[Pin]
		if err := Decode([]byte(raw), &c); err != nil {
			t.Fatal(err)
		}
		if _, err := SteerAgentSettings(c.SteerAgent); err == nil {
			t.Fatal("incomplete value accepted", raw)
		}
	}
}
