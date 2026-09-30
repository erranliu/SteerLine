package task

// TaskClaimReceipt records reservation, not worker start.
type TaskClaimReceipt struct {
	RequestID        string `json:"request_id"`
	TaskID           string `json:"task_id"`
	TaskVersion      uint64 `json:"task_version"`
	WorkflowRunID    string `json:"workflow_run_id"`
	ExecutionID      string `json:"execution_id"`
	AttemptID        string `json:"attempt_id"`
	BaseCommit       string `json:"base_commit"`
	Status           string `json:"status"`
	StartTaskApplied bool   `json:"start_task_applied"`
}

type TaskDeliveryReceipt struct {
	TaskID        string             `json:"task_id"`
	TaskVersion   uint64             `json:"task_version"`
	WorkflowRunID string             `json:"workflow_run_id"`
	GraphRevision uint64             `json:"graph_revision"`
	Status        string             `json:"status"`
	Delivery      TaskDeliveryRecord `json:"delivery"`
}
