package execution

import (
	"encoding/json"
	"strings"
	"testing"

	contracts "github.com/erranliu/SteerLine/internal/steerline/contracts"
)

func TestCurrentRuntimeRetainsSupervisedCoordinatorTerminal(t *testing.T) {
	raw := `{"schema_version":5,"repository":{"identity":"project"},"executions":[{"execution_id":"execution","workflow_run_id":"workflow","target":{"dispatch":"dispatch","coordinator_terminal":"term-original"},"status":"blocked"}]}`
	var runtime contracts.Runtime
	if err := decode([]byte(raw), &runtime); err != nil {
		t.Fatalf("current persisted supervised target was rejected: %v", err)
	}
	if len(runtime.Executions) != 1 || runtime.Executions[0].Target.CoordinatorTerminal != "term-original" {
		t.Fatalf("coordinator terminal identity was lost: %+v", runtime.Executions)
	}
	encoded, err := json.Marshal(runtime)
	if err != nil || !strings.Contains(string(encoded), `"coordinator_terminal":"term-original"`) {
		t.Fatalf("runtime export omitted coordinator terminal identity: %v: %s", err, encoded)
	}
}
