package backend

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
	Version           int                         `json:"version"`
	ID                string                      `json:"id"`
	RunID             string                      `json:"run_id"`
	AttemptID         string                      `json:"attempt_id"`
	Backend           BackendKind                 `json:"backend"`
	Protocol          ExecutionProtocol           `json:"protocol"`
	Target            ExecutionTarget             `json:"target"`
	Action            string                      `json:"action"`
	IdempotencyKey    string                      `json:"idempotency_key,omitempty"`
	Request           []byte                      `json:"request,omitempty"`
	Receipt           []byte                      `json:"receipt,omitempty"`
	PayloadDigest     string                      `json:"payload_digest"`
	Status            OperationStatus             `json:"status"`
	StartedAt         time.Time                   `json:"started_at"`
	UpdatedAt         time.Time                   `json:"updated_at"`
	Diagnostic        string                      `json:"diagnostic,omitempty"`
	ResidualResources []string                    `json:"residual_resources,omitempty"`
	Ref               *ExecutionRef               `json:"ref,omitempty"`
	LifecycleIdentity *LifecycleOperationIdentity `json:"lifecycle_identity,omitempty"`
}

// LifecycleOperationIdentity fixes the input resource binding at intent time.
// Reconciliation may inspect later committed receipts, but never replaces this
// original identity with a current Attempt, target, or generation.
type LifecycleOperationIdentity struct {
	ProjectID           string       `json:"project_id"`
	ExecutionID         string       `json:"execution_id"`
	ConfigurationDigest string       `json:"configuration_digest"`
	ResourceGeneration  string       `json:"resource_generation"`
	InputBindingDigest  string       `json:"input_binding_digest"`
	Ref                 ExecutionRef `json:"ref"`
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
	// BlockedEnvironment removes ambient routing from commands with a fixed host.
	BlockedEnvironment []string
	// Timeout overrides the host's ordinary command budget for this operation.
	// The caller's context can still cancel it earlier.
	Timeout time.Duration
}
