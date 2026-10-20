package runnerclient

import (
	"encoding/json"
	"errors"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

func (c *Catalog) ValidateGenerate(input runnercontract.GenerateRunInput) error {
	if input.Workflow.ID != "generate" || input.Workflow.Version != 1 || input.Execution.Adapter != "codex-app-server" || input.Execution.Target.Kind != "local" {
		return errors.New("generate/1 requires local Codex")
	}
	if err := c.ValidateConfiguration(input.Execution, input.Workflow); err != nil {
		return err
	}
	return validatePublicValue(c.generate, input)
}

// GenerationExecution reuses only the declared native preferences. Development
// permissions, publication authentication and arbitrary settings are not copied.
func GenerationExecution(input ExecutionInput) (ExecutionInput, error) {
	if input.Adapter != "codex-app-server" || input.Target.Kind != "local" || input.Target.Environment != "" {
		return ExecutionInput{}, errors.New("generation requires a local Codex execution profile")
	}
	var values map[string]json.RawMessage
	if err := json.Unmarshal(input.Settings, &values); err != nil {
		return ExecutionInput{}, err
	}
	settings := map[string]json.RawMessage{"access_mode": json.RawMessage(`"read_only"`)}
	for _, key := range []string{"executable", "model", "reasoning_effort"} {
		if v, ok := values[key]; ok {
			settings[key] = v
		}
	}
	raw, err := json.Marshal(settings)
	return ExecutionInput{Adapter: input.Adapter, Target: input.Target, Settings: raw}, err
}
