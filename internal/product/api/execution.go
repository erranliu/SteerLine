package api

import (
	"time"
)

type ControlReceipt struct {
	Version         int        `json:"version"`
	RequestID       string     `json:"request_id"`
	RequestDigest   string     `json:"request_digest"`
	InstanceID      string     `json:"instance_id"`
	Action          string     `json:"action"`
	Status          string     `json:"status"`
	Code            string     `json:"code"`
	Message         string     `json:"message"`
	RequestedBy     string     `json:"requested_by"`
	SubmittedAt     time.Time  `json:"submitted_at"`
	AcceptedAt      *time.Time `json:"accepted_at,omitempty"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	RunID           string     `json:"run_id,omitempty"`
	AttemptID       string     `json:"attempt_id,omitempty"`
	DecisionID      string     `json:"decision_id,omitempty"`
	ControlRevision uint64     `json:"control_revision,omitempty"`
	RunRevision     uint64     `json:"run_revision,omitempty"`
}
