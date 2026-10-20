package runnerclient

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/erranliu/SteerLine/internal/runnercontract"
)

func TestRunnerCatalogAcceptsWorkflowsWithoutFixedPromptSteps(t *testing.T) {
	raw, err := os.ReadFile("testdata/workflow-capabilities.json")
	if err != nil {
		t.Fatal(err)
	}
	var caps Capabilities
	if err := json.Unmarshal(raw, &caps); err != nil {
		t.Fatal(err)
	}
	// Content workflows accept caller messages instead of fixed prompt steps.
	// Their presence must not block a project's development profile.
	for _, id := range []string{"conversation", "generate"} {
		caps.Workflows = append(caps.Workflows, runnercontract.WorkflowChoice{
			ID: id, Version: 1, SupportedAdapters: []string{"codex-app-server"},
		})
	}
	if _, err := compileCatalog(caps); err != nil {
		t.Fatalf("legitimate content catalog blocked development: %v", err)
	}
}

// The frozen public fixture was captured by I1 from Runner 529b6c3 + the
// run_configuration increment. It is test data, never a runtime capability.
func TestRunnerWorkflowContract(t *testing.T) {
	raw, err := os.ReadFile("testdata/workflow-capabilities.json")
	if err != nil {
		t.Fatal(err)
	}
	var caps Capabilities
	if err := json.Unmarshal(raw, &caps); err != nil {
		t.Fatal(err)
	}
	cat, err := compileCatalog(caps)
	if err != nil {
		t.Fatal(err)
	}
	e := ExecutionInput{Adapter: "codex-app-server", Target: ExecutionTarget{Kind: "local"}, Settings: json.RawMessage(`{"model":"explicit"}`)}
	for _, tc := range []struct {
		name, raw string
		valid     bool
	}{
		{"ordinary", `{"id":"development","version":2}`, true},
		{"audit inherit effort", `{"id":"development-audited","version":2,"audit_model":{"model":"audit"}}`, true},
		{"audit explicit effort", `{"id":"development-audited","version":2,"audit_model":{"model":"audit","reasoning_effort":"high"}}`, true},
		{"missing audit", `{"id":"development-audited","version":2}`, false},
		{"audit on plain", `{"id":"development","version":2,"audit_model":{"model":"audit"}}`, false},
		{"unknown version", `{"id":"development","version":3}`, false},
		{"unknown override", `{"id":"development-audited","version":2,"audit_model":{"model":"audit"},"prompt_overrides":{"foreign.step":"text"}}`, false},
		{"explicit null audit", `{"id":"development-audited","version":2,"audit_model":null}`, false},
		{"explicit null effort", `{"id":"development-audited","version":2,"audit_model":{"model":"audit","reasoning_effort":null}}`, false},
		{"explicit null overrides", `{"id":"development","version":2,"prompt_overrides":null}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var w Workflow
			if err := json.Unmarshal([]byte(tc.raw), &w); err != nil {
				t.Fatal(err)
			}
			if err := cat.ValidateConfiguration(e, w); (err == nil) != tc.valid {
				t.Fatalf("valid=%v error=%v", tc.valid, err)
			}
		})
	}
	caps.Workflows = append(caps.Workflows, caps.Workflows[0])
	if _, err := compileCatalog(caps); err == nil {
		t.Fatal("duplicate published choice accepted")
	}
	caps.Workflows = caps.Workflows[:len(caps.Workflows)-1]
	caps.ConfigurationSchemas.RunConfiguration = json.RawMessage(`{"$ref":"file:///private/contract.json"}`)
	if _, err := compileCatalog(caps); err == nil {
		t.Fatal("external contract loader accepted")
	}
	for _, raw := range []string{`{"id":"development","version":2,"unexpected":true}`, `{"id":"development-audited","version":2,"audit_model":{"model":"audit","unexpected":true}}`} {
		var workflow Workflow
		if json.Unmarshal([]byte(raw), &workflow) == nil {
			t.Fatal("workflow JSON codec swallowed an unknown field", raw)
		}
	}
}

func TestRunnerNativeOutcomeContract(t *testing.T) {
	for _, outcome := range []string{"unreported", "unknown", "failed", "cancelled", "succeeded"} {
		raw := `{"status":"pr_ready","execution":{"native_outcome":"` + outcome + `","final_available":true},"final":{"native_outcome":"` + outcome + `","verified":true,"text":"original"}}`
		var run Run
		if err := json.Unmarshal([]byte(raw), &run); err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(run)
		if err != nil {
			t.Fatal(err)
		}
		var restored Run
		if err = json.Unmarshal(encoded, &restored); err != nil {
			t.Fatal(err)
		}
		if restored.Status != "pr_ready" || restored.Execution.NativeOutcome != outcome || restored.Final == nil || restored.Final.NativeOutcome != outcome || !restored.Final.Verified {
			t.Fatal("caller reinterpreted Runner business or native facts", restored)
		}
	}
}
