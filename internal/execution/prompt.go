package execution

import (
	"errors"
	"fmt"
	"strings"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func Prompt(task taskmodel.TaskRecord, contract taskmodel.TaskContract, workflowRunID string, policy executionbackend.PublicationPolicy) (string, error) {
	if task.TaskID == "" || task.Title == "" || contract.ContractID == "" || contract.SpecRef.Path == "" || contract.PlanRef.Path == "" || !executionbackend.StableExecutionID(workflowRunID) {
		return "", errors.New("worker instruction requires a fixed task, Spec, Plan and workflow run")
	}
	if err := policy.Validate(); err != nil {
		return "", err
	}
	var prompt strings.Builder
	fmt.Fprintf(&prompt, "SteerLine task %s: %s\nFixed SteerLine workflow_run_id: %s\n", task.TaskID, task.Title, workflowRunID)
	prompt.WriteString("\n")
	fmt.Fprintf(&prompt, "Follow the fixed task contract and the project's instructions. Read each reference from its listed Git commit and check its SHA-256. Implement the task, commit and push the code, create a PR targeting %s, and report its URL. Keep working in this same session when the user sends feedback.\n\n", contract.Branch)
	fmt.Fprintf(&prompt, "Spec: %s at %s (SHA-256 %s)\n", contract.SpecRef.Path, contract.SpecRef.Commit, contract.SpecRef.SHA256)
	fmt.Fprintf(&prompt, "Plan: %s at %s (SHA-256 %s)\n", contract.PlanRef.Path, contract.PlanRef.Commit, contract.PlanRef.SHA256)
	for _, ref := range contract.CommonRefs {
		fmt.Fprintf(&prompt, "Common instruction: %s at %s (SHA-256 %s)\n", ref.Path, ref.Commit, ref.SHA256)
	}
	for _, ref := range contract.AcceptanceRefs {
		fmt.Fprintf(&prompt, "Acceptance reference: %s at %s (SHA-256 %s)\n", ref.Path, ref.Commit, ref.SHA256)
	}
	return prompt.String(), nil
}
