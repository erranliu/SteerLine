package task

// DeliveryGate contains the selected worker facts read from center authority.
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
func DeliveryPR(record TaskRecord, contract TaskContract, gate DeliveryGate, workflowVersion string) (string, error) {
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

// DependencyClaimError maps an unmet predecessor to its stable command error.
func DependencyClaimError(blockers []TaskDependencyStatus) error {
	for _, blocker := range blockers {
		switch blocker.Blocker {
		case "discarded_predecessor":
			return newError("discarded_predecessor", "a discarded predecessor blocks this task")
		case "inconsistent_delivery":
			return newError("inconsistent_delivery", "a predecessor's state conflicts with its delivery record")
		}
	}
	return newError("task_not_ready", "one or more predecessors lack verified delivery")
}
