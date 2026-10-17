// Package hostcontract owns service metadata and host observation contracts.
// It contains no resource implementation, storage or product projection.
package hostcontract

import (
	"encoding/json"
	"time"

	"github.com/erranliu/SteerLine/internal/runnercontract"
)

const ServiceCheckpointSchema = 21

type Repository struct {
	Path     string `json:"path"`
	Remote   string `json:"remote"`
	Branch   string `json:"branch"`
	Identity string `json:"identity"`
}

type ServiceState struct {
	RunnerBuild         *runnercontract.BuildPin `json:"runner_build,omitempty"`
	Mode                string                   `json:"mode"`
	InstanceID          string                   `json:"instance_id"`
	PID                 int                      `json:"pid"`
	Status              string                   `json:"status"`
	Claimant            string                   `json:"claimant"`
	Error               *string                  `json:"error"`
	ProcessIdentity     *ProcessIdentity         `json:"process_identity"`
	StartupID           *string                  `json:"startup_id"`
	ConfigDigest        string                   `json:"config_digest"`
	Ready               bool                     `json:"ready"`
	ControlProtocol     int                      `json:"control_protocol"`
	Capabilities        []string                 `json:"capabilities"`
	HeartbeatIntervalMS int64                    `json:"heartbeat_interval_ms"`
}

type ControlState struct {
	DispatchMode   string            `json:"dispatch_mode"`
	DispatchPaused bool              `json:"dispatch_paused"`
	Revision       uint64            `json:"revision"`
	Operation      *ControlOperation `json:"operation"`
	Blockers       []ControlBlocker  `json:"blockers"`
}

type ControlOperation struct {
	RequestID     string          `json:"request_id"`
	Action        string          `json:"action"`
	TaskID        string          `json:"task_id,omitempty"`
	Payload       json.RawMessage `json:"payload,omitempty"`
	Phase         string          `json:"phase"`
	RunID         string          `json:"run_id,omitempty"`
	AttemptID     string          `json:"attempt_id,omitempty"`
	DecisionID    string          `json:"decision_id,omitempty"`
	RequestDigest string          `json:"request_digest"`
	Result        *ControlReceipt `json:"result,omitempty"`
}

type ControlBlocker struct {
	RequestID  string `json:"request_id"`
	Action     string `json:"action"`
	TaskID     string `json:"task_id,omitempty"`
	RunID      string `json:"run_id,omitempty"`
	AttemptID  string `json:"attempt_id,omitempty"`
	DecisionID string `json:"decision_id,omitempty"`
	Reason     string `json:"reason"`
}

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

// ServiceCheckpoint contains only service-owned metadata; task and candidate
// facts must be sampled with it in the storage read transaction.
type ServiceCheckpoint struct {
	SchemaVersion int               `json:"schema_version"`
	Revision      uint64            `json:"revision"`
	UpdatedAt     time.Time         `json:"updated_at"`
	HeartbeatAt   time.Time         `json:"heartbeat_at"`
	Service       ServiceState      `json:"service"`
	Repository    Repository        `json:"repository"`
	Operation     *ControlOperation `json:"operation,omitempty"`
	Blockers      []ControlBlocker  `json:"blockers,omitempty"`
	ReservedTask  *string           `json:"reserved_task,omitempty"`
}
