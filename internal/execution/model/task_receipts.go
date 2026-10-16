package model

type TaskDeliveryReceipt struct {
	TaskID        string             `json:"task_id"`
	TaskVersion   uint64             `json:"task_version"`
	WorkflowRunID string             `json:"workflow_run_id"`
	GraphRevision uint64             `json:"graph_revision"`
	Status        string             `json:"status"`
	Delivery      TaskDeliveryRecord `json:"delivery"`
}
