package runnerclient

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestSteerRunnerUsesPublishedSchema(t *testing.T) {
	var caps Capabilities
	// This is a public schema fixture, with no adapter implementation import.
	if err := json.Unmarshal([]byte(`{"configuration_schemas":{"version":1,"adapters":{"test-adapter":{"$id":"steerrunner/adapters/test/settings/v1","type":"object","additionalProperties":false,"required":["model"],"properties":{"model":{"type":"string","minLength":1}}}},"providers":{}}}`), &caps); err != nil {
		t.Fatal(err)
	}
	catalog, err := compileCatalog(caps)
	if err != nil {
		t.Fatal(err)
	}
	input := ExecutionInput{Adapter: "test-adapter", Target: ExecutionTarget{Kind: "local"}, Settings: json.RawMessage(`{"model":"selected"}`)}
	if err := catalog.ValidateExecution(input); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{`{}`, `{"model":""}`, `{"model":"selected","args":["override"]}`} {
		input.Settings = json.RawMessage(bad)
		if err := catalog.ValidateExecution(input); err == nil {
			t.Fatal("caller ignored the published schema", bad)
		}
	}
	caps.ConfigurationSchemas.Adapters["external"] = json.RawMessage(`{"$ref":"file:///private/user/config.json"}`)
	if _, err := compileCatalog(caps); err == nil || !strings.Contains(err.Error(), "self contained") {
		t.Fatal("schema validation did not reject external resolution through its loader", err)
	}
}

func TestOrcaNativePublishedConfiguration(t *testing.T) {
	raw, err := os.ReadFile("testdata/workflow-capabilities.json")
	if err != nil {
		t.Fatal(err)
	}
	var caps Capabilities
	if err = json.Unmarshal(raw, &caps); err != nil {
		t.Fatal(err)
	}
	catalog, err := compileCatalog(caps)
	if err != nil {
		t.Fatal(err)
	}
	execution := ExecutionInput{Adapter: "orca-terminal", Target: ExecutionTarget{Kind: "orca_environment", Environment: "saved"}, Settings: json.RawMessage(`{"agent_launch":{"executable":"codex","model":"selected","reasoning_effort":"high"},"access_mode":"full_access"}`)}
	if err = catalog.ValidateConfiguration(execution, Workflow{ID: "development", Version: 2}); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"runtime_executable":""`, `"runtime_sha256":""`, `"runtime_executable":"C:/helper.exe"`} {
		execution.Settings = json.RawMessage(`{"agent_launch":{"executable":"codex",` + field + `},"access_mode":"full_access"}`)
		if err = catalog.ValidateConfiguration(execution, Workflow{ID: "development", Version: 2}); err == nil {
			t.Fatal("remote helper key accepted", field)
		}
	}
}
