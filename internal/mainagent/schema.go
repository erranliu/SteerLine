package mainagent

import (
	"encoding/json"
	"sort"
)

func ResultSchema() json.RawMessage {
	str := map[string]any{"type": "string"}
	object := func(p map[string]any) map[string]any {
		keys := []string{}
		for k := range p {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return map[string]any{"type": "object", "additionalProperties": false, "properties": p, "required": keys}
	}
	array := func(v any) any { return map[string]any{"type": "array", "items": v} }
	answer := object(map[string]any{"note": str, "option_id": str, "answers": map[string]any{"type": "object", "additionalProperties": array(str)}})
	schema := object(map[string]any{
		"disposition": map[string]any{"type": "string", "enum": []string{"clarification", "revision", "needs_user", "out_of_scope"}},
		"summary":     str, "recommendation": str, "questions": array(str),
		"evidence": array(object(map[string]any{"path": str, "commit": str, "quote": str})),
		"answer":   map[string]any{"anyOf": []any{map[string]any{"type": "null"}, answer}},
		"changes":  array(object(map[string]any{"path": str, "before_sha256": str, "body": str, "reason": str})),
	})
	b, _ := json.Marshal(schema)
	return b
}
