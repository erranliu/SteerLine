package model

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
	Status              string
	StepID              string
	CandidateSHA        string
}

// DeliveryPR checks the selected worker identity before external merge proof is read.
func DeliveryPR(taskID, recordContractID, contractID, contractDigest string, gate DeliveryGate, workflowVersion string) (string, error) {
	if gate.WorkflowVersion != workflowVersion {
		return "", boundaryError("unsupported_schema", "selected workflow uses an unsupported protocol")
	}
	if recordContractID != contractID || contractDigest == "" {
		return "", boundaryError("contract_mismatch", "selected task contract is unavailable")
	}
	if gate.WorkflowTaskID != taskID || gate.SelectedExecutionID != gate.ExecutionID || gate.ExecutionWorkflowID != gate.WorkflowRunID ||
		gate.PRURL == "" || gate.SessionID == "" || gate.WorkspaceBranch == "" ||
		gate.Status != "waiting_external" || gate.StepID != StepAwaitMerge || gate.CandidateSHA == "" {
		return "", boundaryError("task_not_ready", "selected worker flow has no bound session, execution branch or PR")
	}
	return gate.PRURL, nil
}

func boundaryError(code, message string) error { return &ResultError{Code: code, Message: message} }
