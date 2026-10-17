package runnerclient

import (
	"encoding/json"
	"os"
	"testing"
)

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
		{"ordinary", `{"id":"development","version":1}`, true},
		{"audit inherit effort", `{"id":"development-audited","version":1,"audit_model":{"model":"audit"}}`, true},
		{"audit explicit effort", `{"id":"development-audited","version":1,"audit_model":{"model":"audit","reasoning_effort":"high"}}`, true},
		{"missing audit", `{"id":"development-audited","version":1}`, false},
		{"audit on plain", `{"id":"development","version":1,"audit_model":{"model":"audit"}}`, false},
		{"unknown version", `{"id":"development","version":2}`, false},
		{"unknown override", `{"id":"development-audited","version":1,"audit_model":{"model":"audit"},"prompt_overrides":{"foreign.step":"text"}}`, false},
		{"explicit null audit", `{"id":"development-audited","version":1,"audit_model":null}`, false},
		{"explicit null effort", `{"id":"development-audited","version":1,"audit_model":{"model":"audit","reasoning_effort":null}}`, false},
		{"explicit null overrides", `{"id":"development","version":1,"prompt_overrides":null}`, false},
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
	for _, raw := range []string{`{"id":"development","version":1,"unexpected":true}`, `{"id":"development-audited","version":1,"audit_model":{"model":"audit","unexpected":true}}`} {
		var workflow Workflow
		if json.Unmarshal([]byte(raw), &workflow) == nil {
			t.Fatal("workflow JSON codec swallowed an unknown field", raw)
		}
	}
}
