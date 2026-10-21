package runnerclient

import (
	"errors"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"strings"
)

// WithPromptContext keeps the selected build's published instructions and user
// overrides, adding caller data only to implementation/audit execution prompts.
// No workflow transition, report or publication instructions are replaced.
func (c *Catalog) WithPromptContext(input Workflow, context string) (Workflow, error) {
	if c == nil || context == "" {
		return input, errors.New("published workflow prompt context is unavailable")
	}
	var choice *runnercontract.WorkflowChoice
	for i := range c.Capabilities.Workflows {
		v := &c.Capabilities.Workflows[i]
		if v.ID == input.ID && v.Version == input.Version {
			choice = v
			break
		}
	}
	if choice == nil {
		return input, errors.New("selected workflow is not published")
	}
	out := input
	out.PromptOverrides = map[string]string{}
	for key, body := range input.PromptOverrides {
		out.PromptOverrides[key] = body
	}
	implementation := false
	audit := false
	for _, state := range choice.States {
		if state.Kind != "execute" || (state.Contract != "implementation" && state.Contract != "audit") {
			continue
		}
		allowed := false
		for _, key := range choice.PromptSteps {
			if key == state.PromptKey {
				allowed = true
			}
		}
		if !allowed || state.PromptKey == "" || strings.TrimSpace(state.Prompt) == "" {
			return input, errors.New("selected Runner does not publish complete execution prompt bodies")
		}
		body := input.PromptOverrides[state.PromptKey]
		if body == "" {
			body = state.Prompt
		}
		out.PromptOverrides[state.PromptKey] = body + "\n\n" + context
		implementation = implementation || state.Contract == "implementation"
		audit = audit || state.Contract == "audit"
	}
	if !implementation || choice.AuditModelRequired && !audit {
		return input, errors.New("selected Runner does not publish implementation prompt bodies")
	}
	return out, nil
}
