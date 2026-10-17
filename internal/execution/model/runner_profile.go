package model

import (
	"encoding/json"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

// RunnerProfileDisplay is a projection of the original request, never an
// adapter configuration compiler. Missing parameters remain unspecified.
func RunnerProfileDisplay(id, name, digest string, input runnercontract.RunInput) *FixedExecutionProfile {
	p := &FixedExecutionProfile{ID: id, Name: name, Digest: digest, Host: input.Execution.Target.Kind, Agent: input.Execution.Adapter,
		WorkflowID: input.Workflow.ID, WorkflowVersion: input.Workflow.Version}
	if input.Execution.Target.Environment != "" {
		p.Host = input.Execution.Target.Environment
	}
	var settings struct {
		Model           string `json:"model"`
		ReasoningEffort string `json:"reasoning_effort"`
		AccessMode      string `json:"access_mode"`
		AgentLaunch     struct {
			Model           string `json:"model"`
			ReasoningEffort string `json:"reasoning_effort"`
		} `json:"agent_launch"`
	}
	_ = json.Unmarshal(input.Execution.Settings, &settings)
	p.ExecutionModel, p.ExecutionEffort, p.AccessMode = settings.Model, settings.ReasoningEffort, settings.AccessMode
	if input.Execution.Adapter == "orca-terminal" {
		p.ExecutionModel, p.ExecutionEffort = settings.AgentLaunch.Model, settings.AgentLaunch.ReasoningEffort
	}
	if input.Workflow.AuditModel != nil {
		p.AuditModel, p.AuditEffort = input.Workflow.AuditModel.Model, input.Workflow.AuditModel.ReasoningEffort
	}
	return p
}
