package executionconfig

import (
	"encoding/json"

	"github.com/erranliu/SteerLine/internal/runnercontract"
)

// Target identifies the shared execution target, independently of project,
// agent and model settings. Its representation also scopes target occupancy.
func Target(e runnercontract.ExecutionInput) string {
	if (e.Adapter != "codex-app-server" && e.Adapter != "orca-terminal") || (e.Target.Kind != "local" && e.Target.Kind != "orca_environment") || e.Target.Kind == "local" && e.Target.Environment != "" || e.Target.Kind == "orca_environment" && (e.Adapter != "orca-terminal" || e.Target.Environment == "") {
		return ""
	}
	b, _ := json.Marshal(struct {
		Adapter string
		Target  runnercontract.ExecutionTarget
	}{e.Adapter, e.Target})
	return string(b)
}

type Environment struct {
	Adapter    string                         `json:"adapter"`
	Target     runnercontract.ExecutionTarget `json:"target"`
	ProfileIDs []string                       `json:"profile_ids"`
}

// Environments projects saved profiles in their original order. It does not
// probe targets or assign execution or project readiness.
func Environments(profiles []Profile) []Environment {
	result := []Environment{}
	type target struct {
		adapter string
		value   runnercontract.ExecutionTarget
	}
	groups := map[target]int{}
	for _, profile := range profiles {
		key := target{profile.Execution.Adapter, profile.Execution.Target}
		index, ok := groups[key]
		if !ok {
			index = len(result)
			groups[key] = index
			result = append(result, Environment{Adapter: key.adapter, Target: key.value, ProfileIDs: []string{}})
		}
		result[index].ProfileIDs = append(result[index].ProfileIDs, profile.ID)
	}
	return result
}
