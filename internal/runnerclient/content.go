package runnerclient

import (
	"errors"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

func (c *Catalog) SupportsManagedConversation() bool {
	if c == nil || c.conversation == nil || c.Capabilities.WorkspaceWrite == nil {
		return false
	}
	lease := c.Capabilities.WorkspaceWrite
	return lease.Workflow == "conversation/2" && lease.Acquire == "workspace acquire" &&
		lease.Release == "workspace release" && lease.RunProperty == "workspace_write" && lease.SurvivesRestart
}

func (c *Catalog) ValidateManagedConversation(input runnercontract.ConversationRunInput) error {
	if !c.SupportsManagedConversation() || input.Workflow.ID != "conversation" || input.Workflow.Version != 2 ||
		input.Execution.Adapter != "codex-app-server" || input.Execution.Target.Kind != "local" {
		return errors.New("Runner does not support managed local conversation/2")
	}
	if err := c.ValidateConfiguration(input.Execution, input.Workflow); err != nil {
		return err
	}
	return validatePublicValue(c.conversation, input)
}

func (c *Catalog) ValidateGenerate(input runnercontract.GenerateRunInput) error {
	if input.Workflow.ID != "generate" || input.Workflow.Version != 1 || input.Execution.Adapter != "codex-app-server" || input.Execution.Target.Kind != "local" {
		return errors.New("generate/1 requires local Codex")
	}
	if err := c.ValidateConfiguration(input.Execution, input.Workflow); err != nil {
		return err
	}
	return validatePublicValue(c.generate, input)
}
