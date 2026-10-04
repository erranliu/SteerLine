package model

type PredecessorAddInput struct {
	TaskID                  string   `json:"task_id"`
	PredecessorIDs          []string `json:"predecessor_ids"`
	RequestID               string   `json:"request_id"`
	ExpectedTaskVersion     uint64   `json:"expected_task_version"`
	ExpectedGraphRevision   uint64   `json:"expected_graph_revision"`
	WorkflowRunID           string   `json:"workflow_run_id,omitempty"`
	ConfirmedProcessStopped bool     `json:"confirmed_process_stopped,omitempty"`
	Reason                  string   `json:"reason"`
	RequestedBy             string   `json:"requested_by"`
}

type PredecessorAddReceipt struct {
	RequestID      string   `json:"request_id"`
	TaskID         string   `json:"task_id"`
	PredecessorIDs []string `json:"predecessor_ids"`
	TaskVersion    uint64   `json:"task_version"`
	GraphRevision  uint64   `json:"graph_revision"`
	AbandonedRunID string   `json:"abandoned_run_id,omitempty"`
	Status         string   `json:"status"`
}
