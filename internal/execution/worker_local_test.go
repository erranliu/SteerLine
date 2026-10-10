package execution

import (
	"encoding/json"
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

func TestWorkerFlowPromptBindsFixedTaskContract(t *testing.T) {
	task := taskmodel.TaskRecord{TaskID: "102", Title: "Native branch A"}
	contract := taskmodel.TaskContract{ContractID: "fixed", Branch: "main", SpecRef: taskmodel.FileReference{Path: "docs/spec/task.spec.md", Commit: strings.Repeat("b", 40), SHA256: strings.Repeat("c", 64)}, PlanRef: taskmodel.FileReference{Path: "docs/spec/task.plan.md", Commit: strings.Repeat("d", 40), SHA256: strings.Repeat("e", 64)}}
	runID := strings.Repeat("a", 32)
	prompt, err := Prompt(task, contract, runID, executionbackend.PublicationPolicy{Publisher: "worker"})
	if err != nil || !strings.HasPrefix(prompt, "Current step: implement.execute.") || !strings.Contains(prompt, "Do not publish in this step.") {
		t.Fatalf("worker prompt omitted its current step boundary: %q / %v", prompt, err)
	}
	_, body, found := strings.Cut(prompt, "\nFixed facts:\n")
	var facts struct {
		TaskID string                  `json:"task_id"`
		Title  string                  `json:"title"`
		Spec   taskmodel.FileReference `json:"spec"`
		Plan   taskmodel.FileReference `json:"plan"`
	}
	if !found || json.Unmarshal([]byte(body), &facts) != nil || facts.TaskID != task.TaskID || facts.Title != task.Title || facts.Spec != contract.SpecRef || facts.Plan != contract.PlanRef {
		t.Fatal("worker prompt changed its fixed task or committed Spec/Plan references")
	}
	if _, err := Prompt(task, contract, "", executionbackend.PublicationPolicy{Publisher: "worker"}); err == nil {
		t.Fatal("worker prompt accepted a missing workflow run ID")
	}
}
