package client

import (
	"testing"

	api "github.com/erranliu/SteerLine/internal/product/api"
)

func TestStateNamingDecodeRequiresCurrentFields(t *testing.T) {
	var candidate api.CandidateSummary
	if err := decodeJSON([]byte(`{"workflow_stage":"await_merge","workflow_status":"waiting_external","task_status":"in_progress"}`), &candidate); err != nil || candidate.WorkflowStage != "await_merge" || candidate.WorkflowStatus != "waiting_external" {
		t.Fatalf("current workflow snapshot failed: %+v / %v", candidate, err)
	}
	for _, raw := range []string{`{"step_id":"await_merge"}`, `{"status":"waiting_external"}`, `{"finish_task_status":"waiting_pr_merge"}`} {
		if err := decodeJSON([]byte(raw), &candidate); err == nil {
			t.Fatalf("retired snapshot accepted: %s", raw)
		}
	}
}
