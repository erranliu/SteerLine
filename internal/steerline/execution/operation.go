package execution

import (
	"time"
)

type OperationStatus string

const (
	OperationPending   OperationStatus = "pending"
	OperationInFlight  OperationStatus = "in_flight"
	OperationAccepted  OperationStatus = "accepted"
	OperationConfirmed OperationStatus = "confirmed"
	OperationRejected  OperationStatus = "rejected"
	OperationUnknown   OperationStatus = "outcome_unknown"
)

type Operation struct {
	Version           int               `json:"version"`
	ID                string            `json:"id"`
	RunID             string            `json:"run_id"`
	AttemptID         string            `json:"attempt_id"`
	Backend           BackendKind       `json:"backend"`
	Protocol          ExecutionProtocol `json:"protocol"`
	Target            ExecutionTarget   `json:"target"`
	Action            string            `json:"action"`
	IdempotencyKey    string            `json:"idempotency_key,omitempty"`
	Request           []byte            `json:"request,omitempty"`
	Receipt           []byte            `json:"receipt,omitempty"`
	PayloadDigest     string            `json:"payload_digest"`
	Status            OperationStatus   `json:"status"`
	StartedAt         time.Time         `json:"started_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
	Diagnostic        string            `json:"diagnostic,omitempty"`
	ResidualResources []string          `json:"residual_resources,omitempty"`
	Ref               *ExecutionRef     `json:"ref,omitempty"`
}

type StopReceipt struct {
	Disposition string `json:"disposition"`
	Diagnostic  string `json:"diagnostic,omitempty"`
}

// LaunchCommand is a fixed executable and independent argv. The execution
// implementation owns shell quoting; EnvironmentRefs contain no secret values.
type LaunchCommand struct {
	Executable       string
	Args             []string
	WorkingDirectory string
	EnvironmentRefs  []string
}
