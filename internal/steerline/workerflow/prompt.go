package workerflow

import (
	"errors"
	"fmt"
	"strings"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/task"
)

func Prompt(task task.TaskRecord, contract task.TaskContract, workflowRunID string, policy executionapi.PublicationPolicy) (string, error) {
	if task.TaskID == "" || task.Title == "" || contract.ContractID == "" || contract.SpecRef.Path == "" || contract.PlanRef.Path == "" || !executionapi.StableExecutionID(workflowRunID) {
		return "", errors.New("worker instruction requires a fixed task, Spec, Plan and workflow run")
	}
	if err := policy.Validate(); err != nil {
		return "", err
	}
	var prompt strings.Builder
	fmt.Fprintf(&prompt, "SteerLine task %s: %s\nFixed SteerLine workflow_run_id: %s\n", task.TaskID, task.Title, workflowRunID)
	prompt.WriteString("\n")
	if policy.Publisher == executionapi.PublisherHostService {
		fmt.Fprintf(&prompt, "Follow the fixed task contract and the project's instructions. Read each reference from its listed Git commit and check its SHA-256. Implement and verify the task, then stage only the intended changes. SteerLine will commit, push, and create a draft PR targeting %s from the host after your explicit publication request. Keep working in this same session when the user sends feedback.\n\n", contract.Branch)
	} else {
		fmt.Fprintf(&prompt, "Follow the fixed task contract and the project's instructions. Read each reference from its listed Git commit and check its SHA-256. Implement the task, commit and push the code, create a PR targeting %s, and report its URL. Keep working in this same session when the user sends feedback.\n\n", contract.Branch)
	}
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
