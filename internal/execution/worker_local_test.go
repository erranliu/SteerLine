package execution

import (
	"strings"
	"testing"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

const workerFlowMockSession = "00000000-0000-0000-0000-000000000008"

func TestWorkerWorkspaceInstructionUsesPreparedRemoteIdentity(t *testing.T) {
	workspace := executionmodel.WorkspaceEvidence{Worktree: "D:/remote/sl-101", Branch: "remote/sl-101", Base: strings.Repeat("b", 40), Head: strings.Repeat("c", 40)}
	prompt, err := workerWorkspaceInstruction(workspace)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"Fixed assigned worktree: D:/remote/sl-101", "Fixed assigned branch: remote/sl-101", "Fixed base commit: " + workspace.Base, "Existing evidence from earlier runs is not the authority"} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("prepared workspace context missing %q: %s", expected, prompt)
		}
	}
	if strings.Contains(prompt, workspace.Head) {
		t.Fatal("current head replaced the frozen base")
	}
	for _, missing := range []executionmodel.WorkspaceEvidence{{}, {Worktree: workspace.Worktree, Base: workspace.Base}, {Worktree: workspace.Worktree, Branch: workspace.Branch, Base: "HEAD"}} {
		if _, err := workerWorkspaceInstruction(missing); err == nil {
			t.Fatal("accepted an incomplete assigned workspace")
		}
	}
}

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
