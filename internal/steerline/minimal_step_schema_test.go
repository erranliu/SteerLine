package steerline

import (
	"encoding/json"
	"testing"
)

func TestMinimalStepResultSchemaUsesStrictStructuredOutputShape(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal(MinimalStepResultSchema, &schema); err != nil {
		t.Fatal(err)
	}
	checkStrictStructuredOutputNode(t, schema, "$", true)
}

func checkStrictStructuredOutputNode(t *testing.T, schema map[string]any, path string, root bool) {
	t.Helper()
	if _, hasEnum := schema["enum"]; hasEnum {
		if _, hasType := schema["type"]; !hasType {
			t.Errorf("%s has enum without type", path)
		}
	}
	types := schemaTypeSet(schema["type"])
	if _, isObject := types["object"]; isObject {
		if root && schema["type"] != "object" {
			t.Errorf("root schema type must be object, got %#v", schema["type"])
		}
		if schema["additionalProperties"] != false {
			t.Errorf("%s must set additionalProperties=false", path)
		}
		properties, ok := schema["properties"].(map[string]any)
		if !ok {
			return
		}
		requiredValues, _ := schema["required"].([]any)
		required := make(map[string]bool, len(requiredValues))
		for _, value := range requiredValues {
			if name, ok := value.(string); ok {
				required[name] = true
			}
		}
		for name, raw := range properties {
			if !required[name] {
				t.Errorf("%s.%s is not required", path, name)
			}
			child, ok := raw.(map[string]any)
			if !ok {
				t.Errorf("%s.%s is not a schema object", path, name)
				continue
			}
			checkStrictStructuredOutputNode(t, child, path+"."+name, false)
		}
		for name := range required {
			if _, exists := properties[name]; !exists {
				t.Errorf("%s.required contains unknown property %q", path, name)
			}
		}
	}
	if _, isArray := types["array"]; isArray {
		items, ok := schema["items"].(map[string]any)
		if !ok {
			t.Errorf("%s array has no item schema", path)
			return
		}
		checkStrictStructuredOutputNode(t, items, path+"[]", false)
	}
}

func schemaTypeSet(raw any) map[string]bool {
	result := map[string]bool{}
	switch value := raw.(type) {
	case string:
		result[value] = true
	case []any:
		for _, item := range value {
			if value, ok := item.(string); ok {
				result[value] = true
			}
		}
	}
	return result
}
