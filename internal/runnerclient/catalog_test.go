package runnerclient

import (
	"encoding/json"
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
