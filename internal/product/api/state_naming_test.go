package api

import (
	"encoding/json"
	"testing"
)

func TestStateNamingPublicWorkflowFields(t *testing.T) {
	for _, value := range []any{
		CandidateSummary{WorkflowStage: "await_merge", WorkflowStatus: "waiting_external", TaskStatus: "in_progress", ExecutionStatus: "succeeded", ResourceStatus: "idle"},
		WorkflowSummary{WorkflowStage: "await_merge", WorkflowStatus: "waiting_external"},
		RecoveryInspection{WorkflowStage: "await_merge", WorkflowStatus: "waiting_external"},
	} {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		for _, old := range []string{"step_id", "status", "finish_task_status", "delivery_status"} {
			if _, exists := fields[old]; exists {
				t.Fatalf("obsolete workflow field %s in %s", old, raw)
			}
		}
		if string(fields["workflow_stage"]) != `"await_merge"` || string(fields["workflow_status"]) != `"waiting_external"` {
			t.Fatalf("workflow facts lost: %s", raw)
		}
	}
}
