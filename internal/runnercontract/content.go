package runnercontract

import "encoding/json"

// GenerateInput is the published generate/1 payload; it carries no development fields.
type GenerateInput struct {
	Instruction  string          `json:"instruction"`
	Materials    []Document      `json:"materials"`
	OutputSchema json.RawMessage `json:"output_schema"`
}
type GenerateRunInput struct {
	Title       string            `json:"title"`
	Correlation map[string]string `json:"correlation,omitempty"`
	Workflow    Workflow          `json:"workflow"`
	Execution   ExecutionInput    `json:"execution"`
	Generate    GenerateInput     `json:"generate"`
}
type ContentResult struct {
	Status     string          `json:"status"`
	Text       string          `json:"text,omitempty"`
	JSON       json.RawMessage `json:"json,omitempty"`
	Diagnostic string          `json:"diagnostic,omitempty"`
	Source     Final           `json:"source"`
}
