package task

import (
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

type DeliveryGate struct {
	WorkflowVersion     string
	WorkflowTaskID      string
	SelectedExecutionID string
	ExecutionID         string
	ExecutionWorkflowID string
	WorkflowRunID       string
	PRURL               string
	SessionID           string
	WorkspaceBranch     string
	Phase               string
}

// DeliveryPR checks the selected worker identity before external merge proof is read.
func DeliveryPR(record taskmodel.TaskRecord, contract taskmodel.TaskContract, gate DeliveryGate, workflowVersion string) (string, error) {
	if gate.WorkflowVersion != workflowVersion {
		return "", newError("unsupported_schema", "selected workflow uses an unsupported protocol")
	}
	if record.ContractID != contract.ContractID || contract.Digest == "" {
		return "", newError("contract_mismatch", "selected task contract is unavailable")
	}
	if gate.WorkflowTaskID != record.TaskID || gate.SelectedExecutionID != gate.ExecutionID || gate.ExecutionWorkflowID != gate.WorkflowRunID ||
		gate.PRURL == "" || gate.SessionID == "" || gate.WorkspaceBranch == "" ||
		gate.Phase != "executing" && gate.Phase != "waiting_decision" && gate.Phase != "waiting_pr_merge" {
		return "", newError("task_not_ready", "selected worker flow has no bound session, execution branch or PR")
	}
	return gate.PRURL, nil
}
