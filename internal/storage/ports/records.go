package ports

import (
	"time"
)

type DomainEvent struct {
	ProjectIdentity string    `json:"project_identity"`
	EventID         int64     `json:"event_id,omitempty"`
	TaskID          string    `json:"task_id,omitempty"`
	WorkflowRunID   string    `json:"workflow_run_id,omitempty"`
	ExecutionID     string    `json:"execution_id,omitempty"`
	AttemptID       string    `json:"attempt_id,omitempty"`
	ExternalSource  string    `json:"external_source,omitempty"`
	ExternalID      string    `json:"external_id,omitempty"`
	Type            string    `json:"type"`
	ReceivedAt      time.Time `json:"received_at"`
	Payload         []byte    `json:"payload,omitempty"`
	PayloadBytes    int       `json:"payload_bytes,omitempty"`
	PayloadDigest   string    `json:"payload_digest,omitempty"`
	PayloadOmitted  bool      `json:"payload_omitted,omitempty"`
}

type CommandRecord struct {
	ProjectIdentity string    `json:"project_identity"`
	CommandID       string    `json:"command_id"`
	Kind            string    `json:"kind"`
	TargetID        string    `json:"target_id"`
	ExpectedVersion uint64    `json:"expected_version"`
	IdempotencyKey  string    `json:"idempotency_key"`
	RequestedBy     string    `json:"requested_by"`
	Payload         []byte    `json:"payload"`
	Status          string    `json:"status"`
	Result          []byte    `json:"result,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}
