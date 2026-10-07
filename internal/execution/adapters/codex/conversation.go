package codexworker

import (
	"encoding/json"
)

func nonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func rawObject(raw json.RawMessage) map[string]json.RawMessage {
	var result map[string]json.RawMessage
	_ = json.Unmarshal(raw, &result)
	if result == nil {
		return map[string]json.RawMessage{}
	}
	return result
}

func stringField(fields map[string]json.RawMessage, name string) string {
	var value string
	_ = json.Unmarshal(fields[name], &value)
	return value
}

func firstNonEmpty(values ...string) string { return nonEmpty(values...) }
