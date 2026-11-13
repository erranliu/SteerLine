package manualverification

// Schema constrains generated documents. Decode also enforces byte limits,
// path and URL semantics, and unique check IDs.
func Schema() map[string]any {
	object := func(properties map[string]any, required ...string) map[string]any {
		return map[string]any{"type": "object", "additionalProperties": false, "properties": properties, "required": required}
	}
	text := func(min, max int) map[string]any {
		return map[string]any{"type": "string", "minLength": min, "maxLength": max}
	}
	array := func(min, max int, item map[string]any) map[string]any {
		return map[string]any{"type": "array", "minItems": min, "maxItems": max, "items": item}
	}
	id := text(1, 32)
	id["pattern"] = checkID.String()
	return object(map[string]any{
		"version": map[string]any{"type": "integer", "const": 1},
		"launch": object(map[string]any{
			"program": text(1, 256), "args": array(0, 64, text(0, 2048)),
			"cwd": text(1, 512), "url": text(1, 1024),
		}, "program", "args"),
		"checks": array(1, 32, object(map[string]any{
			"id": id, "title": text(1, 120), "steps": array(1, 20, text(1, 1024)), "expected": text(1, 2048),
		}, "id", "title", "steps", "expected")),
	}, "version", "launch", "checks")
}
