package testmanagement

import "encoding/json"

// The output values are the exact IDs from the frozen project catalogue.
// Descriptions and source citations belong to their original material, not IDs.
func generationOutputSchema(c Catalogue) json.RawMessage {
	items := map[string]any{"type": "string"}
	tests := map[string]any{"type": "array", "uniqueItems": true, "items": items}
	if len(c.Tests) == 0 {
		tests["maxItems"] = 0
	} else {
		ids := make([]string, 0, len(c.Tests))
		for _, test := range c.Tests {
			ids = append(ids, test.ID)
		}
		items["enum"] = ids
	}
	schema, _ := json.Marshal(map[string]any{
		"type": "object", "required": []string{"tests"}, "additionalProperties": false,
		"properties": map[string]any{"tests": tests},
	})
	return schema
}
