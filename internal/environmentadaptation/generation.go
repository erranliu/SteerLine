package environmentadaptation

import _ "embed"

//go:embed prompts/materials.md
var MaterialPrompt string

const GenerationSchema = `{"type":"object","additionalProperties":false,"required":["document","script","gaps"],"properties":{"document":{"type":"string","minLength":1,"maxLength":262144},"script":{"type":"string","minLength":1,"maxLength":262144},"gaps":{"type":"array","items":{"type":"string"}}}}`
