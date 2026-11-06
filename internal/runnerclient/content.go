package runnerclient

import (
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
