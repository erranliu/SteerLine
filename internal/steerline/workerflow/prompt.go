package workerflow

import (
	"errors"
	"fmt"
	"strings"

	"github.com/erranliu/SteerLine/internal/steerline/task"
)

func stableID(value string) bool {
	if len(value) != 32 {
		return false
	}
	for _, c := range value {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func Prompt(task task.TaskRecord, contract task.TaskContract, workflowRunID string, localPublish bool) (string, error) {
	if task.TaskID == "" || task.Title == "" || contract.ContractID == "" || contract.SpecRef.Path == "" || contract.PlanRef.Path == "" || !stableID(workflowRunID) {
		return "", errors.New("worker instruction requires a fixed task, Spec, Plan and workflow run")
	}
	var prompt strings.Builder
	fmt.Fprintf(&prompt, "SteerLine task %s: %s\nFixed SteerLine workflow_run_id: %s\n", task.TaskID, task.Title, workflowRunID)
	if !localPublish {
		prompt.WriteString("Orca Run, Task and Dispatch IDs are separate from this ID.\n")
	}
	prompt.WriteString("\n")
	if localPublish {
		fmt.Fprintf(&prompt, "Follow the fixed task contract and the project's instructions. Read each reference from its listed Git commit and check its SHA-256. Implement and verify the task, then stage only the intended changes. SteerLine will commit, push, and create a draft PR targeting %s from the host after your explicit publication request. Keep working in this same session when the user sends feedback.\n\n", contract.Branch)
	} else {
		fmt.Fprintf(&prompt, "Follow the fixed task contract and the project's instructions. Read each reference from its listed Git commit and check its SHA-256. Implement the task, commit the code, create a PR targeting %s, and report its URL. Keep working in this same session when the user sends feedback.\n\n", contract.Branch)
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
