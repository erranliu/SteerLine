package api

import "time"

type TaskBatchIssue struct {
	Code    string `json:"code"`
	Node    string `json:"node,omitempty"`
	Message string `json:"message"`
}

type TaskBatchValidation struct {
	Valid            bool             `json:"valid"`
	ProjectID        string           `json:"project_id"`
	TaskCount        int              `json:"task_count"`
	DependencyCount  int              `json:"dependency_count"`
	AuthorityVersion uint64           `json:"authority_version"`
	GraphRevision    uint64           `json:"graph_revision"`
	DatabaseRevision uint64           `json:"database_revision"`
	DatabaseID       string           `json:"database_id"`
	SampledAt        time.Time        `json:"sampled_at"`
	Cycle            []string         `json:"cycle,omitempty"`
	Issues           []TaskBatchIssue `json:"issues"`
}

type TaskBatchReceipt struct {
	RequestID     string            `json:"request_id"`
	BatchID       string            `json:"batch_id"`
	TaskIDs       map[string]string `json:"task_ids"`
	GraphRevision uint64            `json:"graph_revision"`
	PayloadDigest string            `json:"payload_digest"`
	Status        string            `json:"status"`
	DispatchNote  string            `json:"dispatch_note"`
}

type TaskDiscardReceipt struct {
	RequestID     string   `json:"request_id"`
	TaskID        string   `json:"task_id"`
	Status        string   `json:"status"`
	Code          string   `json:"code,omitempty"`
	Message       string   `json:"message,omitempty"`
	GraphRevision uint64   `json:"graph_revision"`
	StopPending   []string `json:"stop_pending,omitempty"`
}
