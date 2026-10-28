package runnerclient

import (
	"errors"
	"github.com/erranliu/SteerLine/internal/prompts"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"strings"
)

// WithTestProgressContext preserves published/user prompts. Only implementation
// and audit execution receive the frozen tests and writer; report/publication
// turns receive a read-only boundary for the test-owned progress channel.
func (c *Catalog) WithTestProgressContext(input Workflow, snapshot []byte, referencedWriter bool) (Workflow, error) {
	if c == nil || len(snapshot) == 0 {
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
		if state.Kind != "execute" && state.Kind != "report" {
			continue
		}
		writesTests := state.Kind == "execute" && (state.Contract == "implementation" || state.Contract == "audit")
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
		assembled, err := prompts.TestingStep(body, snapshot, referencedWriter, writesTests)
		if err != nil {
			return input, err
		}
		out.PromptOverrides[state.PromptKey] = assembled
		implementation = implementation || writesTests && state.Contract == "implementation"
		audit = audit || writesTests && state.Contract == "audit"
	}
	if !implementation || choice.AuditModelRequired && !audit {
		return input, errors.New("selected Runner does not publish implementation prompt bodies")
	}
	return out, nil
}
