package execution

import (
	"github.com/erranliu/SteerLine/internal/execution/workflow"
	"strings"
	"testing"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

const workerFlowMockSession = "00000000-0000-0000-0000-000000000008"

func TestWorkerWorkspaceInstructionUsesPreparedRemoteIdentity(t *testing.T) {
	workspace := executionmodel.WorkspaceEvidence{Worktree: "D:/remote/sl-101", Branch: "remote/sl-101", Base: strings.Repeat("b", 40), Head: strings.Repeat("c", 40)}
	err := validateWorkerWorkspace(workspace)
	if err != nil {
		t.Fatal(err)
	}
	for _, missing := range []executionmodel.WorkspaceEvidence{{}, {Worktree: workspace.Worktree, Base: workspace.Base}, {Worktree: workspace.Worktree, Branch: workspace.Branch, Base: "HEAD"}} {
		if err := validateWorkerWorkspace(missing); err == nil {
			t.Fatal("accepted an incomplete assigned workspace")
		}
	}
}

func TestWorkerFlowPromptBindsFixedTaskContract(t *testing.T) {
	task := taskmodel.TaskRecord{TaskID: "102", Title: "Native branch A"}
	contract := taskmodel.TaskContract{ContractID: "fixed", Branch: "main", SpecRef: taskmodel.FileReference{Path: "docs/spec/task.spec.md", Commit: strings.Repeat("b", 40), SHA256: strings.Repeat("c", 64)}, PlanRef: taskmodel.FileReference{Path: "docs/spec/task.plan.md", Commit: strings.Repeat("d", 40), SHA256: strings.Repeat("e", 64)}}
	runID := strings.Repeat("a", 32)
	prompt, err := Prompt(task, contract, runID, executionbackend.PublicationPolicy{Publisher: "worker"})
	if err != nil || prompt != workflow.ImplementPrompt+"\nSpec: "+contract.SpecRef.Path+"\nPlan: "+contract.PlanRef.Path {
		t.Fatalf("worker prompt contains more than the action and document paths: %q / %v", prompt, err)
	}
	if _, err := Prompt(task, contract, "", executionbackend.PublicationPolicy{Publisher: "worker"}); err == nil {
		t.Fatal("worker prompt accepted a missing workflow run ID")
	}
}
