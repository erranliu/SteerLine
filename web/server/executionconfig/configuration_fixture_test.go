package executionconfig

import (
	"context"
	"encoding/json"
	"testing"

	cfg "github.com/erranliu/SteerLine/internal/executionconfig"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	protocol "github.com/erranliu/SteerLine/internal/testkit/protocol"
)

// This peer supplies protocol facts for HTTP/store tests. The production native
// mapping is exercised separately against a pinned, independently built EXE.
func configurationFixture(ctx context.Context, _ cfg.Pin, in runnercontract.ExecutionConfigurationRequest) (runnercontract.ExecutionConfigurationResult, error) {
	if in.Operation != "compose" {
		return protocol.ExecutionConfigurationFixture(ctx, in)
	}
	selected := in.Selection
	execution := runnercontract.ExecutionInput{Adapter: selected.Adapter, Target: selected.Target, Settings: json.RawMessage(`{"access_mode":"full_access"}`)}
	if in.BaseExecution != nil && in.BaseExecution.Adapter == selected.Adapter && in.BaseExecution.Target == selected.Target {
		description, _ := protocol.ExecutionConfigurationFixture(ctx, runnercontract.ExecutionConfigurationRequest{Operation: "describe", Execution: in.BaseExecution})
		if description.View.Agent == selected.Agent {
			execution = *in.BaseExecution
		}
	}
	var settings map[string]any
	_ = json.Unmarshal(execution.Settings, &settings)
	fields := settings
	if selected.Adapter == "orca-terminal" {
		settings["agent"] = selected.Agent
		fields = map[string]any{}
		if old, ok := settings["agent_launch"].(map[string]any); ok {
			fields = old
		}
		settings["agent_launch"] = fields
	}
	if selected.Model != nil {
		fields["model"] = *selected.Model
	}
	if selected.ReasoningEffort != nil {
		fields["reasoning_effort"] = *selected.ReasoningEffort
	}
	for _, key := range in.Unset {
		delete(fields, key)
	}
	execution.Settings, _ = json.Marshal(settings)
	identity := protocol.ExecutionIdentity(execution)
	return runnercontract.ExecutionConfigurationResult{Execution: &execution, View: *selected, Identity: &identity}, nil
}
func editFixture(t *testing.T, s *Store, c Config) EditRequest {
	t.Helper()
	current, raw, err := s.read()
	if err != nil {
		t.Fatal(err)
	}
	out := EditRequest{SavedDigest: cfg.Digest(raw), Config: c, ProfileEdits: []ProfileEdit{}}
	for _, p := range c.Profiles {
		unchanged := false
		baseID := ""
		for _, old := range current.Profiles {
			if old.ID == p.ID {
				unchanged = sameExecution(old, p)
				baseID = old.ID
				break
			}
		}
		if unchanged {
			continue
		}
		if baseID == "" {
			for _, old := range current.Profiles {
				if old.Execution.Adapter == p.Execution.Adapter && old.Execution.Target == p.Execution.Target {
					baseID = old.ID
					break
				}
			}
		}
		result, _ := protocol.ExecutionConfigurationFixture(context.Background(), runnercontract.ExecutionConfigurationRequest{Operation: "describe", Execution: &p.Execution})
		unset := []string{}
		if result.View.Model == nil {
			unset = append(unset, "model")
		}
		if result.View.ReasoningEffort == nil {
			unset = append(unset, "reasoning_effort")
		}
		out.ProfileEdits = append(out.ProfileEdits, ProfileEdit{ProfileID: p.ID, BaseProfileID: baseID, Selection: result.View, Unset: unset})
	}
	return out
}
