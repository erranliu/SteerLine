package sqlite

import (
	"encoding/json"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

// Synthetic protocol facts for transaction fixtures; real mapping is verified against the fixed Runner EXE.
func fixtureExecutionIdentity(execution runnercontract.ExecutionInput) runnercontract.ExecutionIdentity {
	context := execution
	var settings map[string]json.RawMessage
	_ = json.Unmarshal(execution.Settings, &settings)
	if execution.Adapter == "codex-app-server" {
		delete(settings, "model")
		delete(settings, "reasoning_effort")
	} else if raw, ok := settings["agent_launch"]; ok {
		var launch map[string]json.RawMessage
		_ = json.Unmarshal(raw, &launch)
		delete(launch, "model")
		delete(launch, "reasoning_effort")
		if len(launch) == 0 {
			delete(settings, "agent_launch")
		} else {
			settings["agent_launch"], _ = json.Marshal(launch)
		}
	}
	context.Settings, _ = json.Marshal(settings)
	return runnercontract.ExecutionIdentity{Contract: runnercontract.ExecutionIdentityContract, SourceExecutionDigest: runnercontract.ConfigurationValueDigest(execution), Context: context}
}
