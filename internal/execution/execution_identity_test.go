package execution

import (
	"encoding/json"
	"reflect"
	"testing"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
)

func TestExecutionIdentityCurrentTerminalRoundTrip(t *testing.T) {
	target := executionbackend.ExecutionTarget{ID: "orca", ConnectionID: "environment", WorktreeID: "worktree", Branch: "worker", BaseRevision: "base", ExternalRuntime: "runtime", ExecutionHost: "host"}
	process := executionbackend.ProcessIdentity{Kind: "orca_terminal", Terminal: "terminal", RuntimeID: "runtime", Incarnation: "incarnation", ExecutionHost: "host", WorktreeID: "worktree"}
	original := reconstructiontypes.Runtime{Executions: []executionmodel.ExecutionRecord{{Target: target, Process: &process}}}
	body, err := json.Marshal(original)
	require(t, err)
	var decoded reconstructiontypes.Runtime
	require(t, decode(body, &decoded))
	if !reflect.DeepEqual(original.Executions, decoded.Executions) {
		t.Fatalf("current execution identity changed: %+v", decoded.Executions)
	}
}

func TestExecutionIdentityRejectsRetiredOrcaFields(t *testing.T) {
	for owner, fields := range map[string][]string{
		"target":  {"orchestration_run", "orchestration_run_request_id", "coordinator_terminal", "external_task", "dispatch"},
		"process": {"coordinator_runtime_id", "orchestration_run", "task", "dispatch"},
	} {
		for _, field := range fields {
			t.Run(owner+"/"+field, func(t *testing.T) {
				body, err := json.Marshal(map[string]any{"executions": []any{map[string]any{owner: map[string]string{field: "retired"}}}})
				require(t, err)
				var decoded reconstructiontypes.Runtime
				if err := decode(body, &decoded); err == nil {
					t.Fatalf("retired %s.%s accepted", owner, field)
				}
			})
		}
	}
}
