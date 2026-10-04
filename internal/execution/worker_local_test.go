package execution

import (
	"strings"
	"testing"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

const workerFlowMockSession = "00000000-0000-0000-0000-000000000008"

func TestWorkerFlowPromptIncludesFixedWorkflowRunID(t *testing.T) {
	task := taskmodel.TaskRecord{TaskID: "102", Title: "Native branch A"}
	contract := taskmodel.TaskContract{ContractID: "fixed", Branch: "main", SpecRef: taskmodel.FileReference{Path: "docs/spec/task.spec.md"}, PlanRef: taskmodel.FileReference{Path: "docs/spec/task.plan.md"}}
	runID := strings.Repeat("a", 32)
	for _, publisher := range []string{"worker"} {
		prompt, err := Prompt(task, contract, runID, executionbackend.PublicationPolicy{Publisher: publisher})
		if err != nil || !strings.Contains(prompt, "Fixed SteerLine workflow_run_id: "+runID) || strings.Contains(prompt, "SteerLine will commit") != (publisher == "host_service") {
			t.Fatalf("worker prompt omitted its fixed center identity: %q / %v", prompt, err)
		}
	}
	if _, err := Prompt(task, contract, "", executionbackend.PublicationPolicy{Publisher: "worker"}); err == nil {
		t.Fatal("worker prompt accepted a missing workflow run ID")
	}
}
