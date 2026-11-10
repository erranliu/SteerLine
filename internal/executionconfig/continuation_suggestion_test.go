package executionconfig

import (
	"encoding/json"
	"testing"

	"github.com/erranliu/SteerLine/internal/runnercontract"
)

func TestContinuationSuggestionUsesOnlyConfiguredLocalCodexTransport(t *testing.T) {
	remote := Profile{Execution: runnercontract.ExecutionInput{Adapter: "codex-app-server", Target: runnercontract.ExecutionTarget{Kind: "orca_environment"}, Settings: json.RawMessage(`{"executable":"remote"}`)}}
	local := Profile{Execution: runnercontract.ExecutionInput{Adapter: "codex-app-server", Target: runnercontract.ExecutionTarget{Kind: "local"}, Settings: json.RawMessage(`{"executable":"local-codex.exe","model":"worker-model","access_mode":"full_access"}`)}}
	if ContinuationSuggestionBase([]Profile{remote}) != nil {
		t.Fatal("remote transport became local fallback")
	}
	got := ContinuationSuggestionBase([]Profile{remote, local})
	if got == nil || string(got.Settings) != string(local.Execution.Settings) {
		t.Fatal("configured local transport was lost")
	}
	got.Settings[0] = 'x'
	if !json.Valid(local.Execution.Settings) {
		t.Fatal("shared execution settings were mutated")
	}
}

func TestContinuationSuggestionDefaultsAndExplicitSelection(t *testing.T) {
	got, err := ContinuationSuggestionSettings(nil)
	if err != nil || got.Model != "gpt-6.1-sol" || got.ReasoningEffort != "high" {
		t.Fatal(got, err)
	}
	for _, bad := range []ContinuationSuggestion{{Model: "", ReasoningEffort: "high"}, {Model: " padded ", ReasoningEffort: "high"}, {Model: "model\ncommand", ReasoningEffort: "high"}, {Model: "model", ReasoningEffort: "unknown"}} {
		if _, err := ContinuationSuggestionSettings(&bad); err == nil {
			t.Fatal("invalid model settings accepted", bad)
		}
	}
	explicit := ContinuationSuggestion{Model: "selected", ReasoningEffort: "medium"}
	got, err = ContinuationSuggestionSettings(&explicit)
	if err != nil || got != explicit {
		t.Fatal("explicit model was replaced", got, err)
	}
}

func TestContinuationSuggestionConfigurationKeepsExistingValueSemantics(t *testing.T) {
	for _, raw := range []string{`{}`, `{"continuation_suggestion":null}`, `{"continuation_suggestion":{"model":"chosen","reasoning_effort":"medium"}}`} {
		var c File[Pin]
		if err := Decode([]byte(raw), &c); err != nil {
			t.Fatal(raw, err)
		}
		got, err := ContinuationSuggestionSettings(c.ContinuationSuggestion)
		if err != nil {
			t.Fatal(raw, err)
		}
		want := ContinuationSuggestion{Model: "gpt-6.1-sol", ReasoningEffort: "high"}
		if c.ContinuationSuggestion != nil {
			want = ContinuationSuggestion{Model: "chosen", ReasoningEffort: "medium"}
		}
		if got != want {
			t.Fatal(raw, got)
		}
	}
	for _, raw := range []string{`{"main_agent":null}`, `{"main_agent":{"model":"chosen","reasoning_effort":"medium"}}`, `{"continuation_suggestion":{"model":"chosen","reasoning_effort":"medium","extra":true}}`, `{"steer_agent":{"continuation":{"suggest":{}}}}`} {
		var c File[Pin]
		if err := Decode([]byte(raw), &c); err == nil {
			t.Fatal("unknown or obsolete configuration accepted", raw)
		}
	}
	for _, raw := range []string{`{"continuation_suggestion":{}}`, `{"continuation_suggestion":{"model":"chosen"}}`, `{"continuation_suggestion":{"reasoning_effort":"high"}}`} {
		var c File[Pin]
		if err := Decode([]byte(raw), &c); err != nil {
			t.Fatal(err)
		}
		if _, err := ContinuationSuggestionSettings(c.ContinuationSuggestion); err == nil {
			t.Fatal("incomplete value accepted", raw)
		}
	}
}
