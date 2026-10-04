package steerline

import (
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/workerflow"
	"strings"
	"testing"
)

const workerFlowMockSession = "00000000-0000-0000-0000-000000000008"

func TestWorkerFlowPromptIncludesFixedWorkflowRunID(t *testing.T) {
	task := TaskRecord{TaskID: "102", Title: "Native branch A"}
	contract := TaskContract{ContractID: "fixed", Branch: "main", SpecRef: FileReference{Path: "docs/spec/task.spec.md"}, PlanRef: FileReference{Path: "docs/spec/task.plan.md"}}
	runID := strings.Repeat("a", 32)
	for _, publisher := range []string{"worker"} {
		prompt, err := workerflow.Prompt(task, contract, runID, executionapi.PublicationPolicy{Publisher: publisher})
		if err != nil || !strings.Contains(prompt, "Fixed SteerLine workflow_run_id: "+runID) || strings.Contains(prompt, "SteerLine will commit") != (publisher == "host_service") {
			t.Fatalf("worker prompt omitted its fixed center identity: %q / %v", prompt, err)
		}
	}
	if _, err := workerflow.Prompt(task, contract, "", executionapi.PublicationPolicy{Publisher: "worker"}); err == nil {
		t.Fatal("worker prompt accepted a missing workflow run ID")
	}
}
