package ports

import (
	"encoding/json"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

const WorkerFlowRuntimeSchema = 21

type CheckpointRepository struct {
	Path     string `json:"path"`
	Remote   string `json:"remote"`
	Branch   string `json:"branch"`
	Identity string `json:"identity"`
}

type CheckpointServiceState struct {
	Mode                string                            `json:"mode"`
	InstanceID          string                            `json:"instance_id"`
	PID                 int                               `json:"pid"`
	Status              string                            `json:"status"`
	Claimant            string                            `json:"claimant"`
	Error               *string                           `json:"error"`
	ProcessIdentity     *executionbackend.ProcessIdentity `json:"process_identity"`
	StartupID           *string                           `json:"startup_id"`
	ConfigDigest        string                            `json:"config_digest"`
	Ready               bool                              `json:"ready"`
	ControlProtocol     int                               `json:"control_protocol"`
	Capabilities        []string                          `json:"capabilities"`
	HeartbeatIntervalMS int64                             `json:"heartbeat_interval_ms"`
}

type CheckpointControlOperation struct {
	RequestID     string                         `json:"request_id"`
	Action        string                         `json:"action"`
	TaskID        string                         `json:"task_id,omitempty"`
	Payload       json.RawMessage                `json:"payload,omitempty"`
	Phase         string                         `json:"phase"`
	RunID         string                         `json:"run_id,omitempty"`
	AttemptID     string                         `json:"attempt_id,omitempty"`
	DecisionID    string                         `json:"decision_id,omitempty"`
	RequestDigest string                         `json:"request_digest"`
	Result        *executionmodel.ControlReceipt `json:"result,omitempty"`
}

type CheckpointControlBlocker struct {
	RequestID  string `json:"request_id"`
	Action     string `json:"action"`
	TaskID     string `json:"task_id,omitempty"`
	RunID      string `json:"run_id,omitempty"`
	AttemptID  string `json:"attempt_id,omitempty"`
	DecisionID string `json:"decision_id,omitempty"`
	Reason     string `json:"reason"`
}

// ServiceCheckpoint persists only service-owned recovery state. Execution
// records are sampled through their separate read port in the same snapshot.
type ServiceCheckpoint struct {
	SchemaVersion int                         `json:"schema_version"`
	Revision      uint64                      `json:"revision"`
	UpdatedAt     time.Time                   `json:"updated_at"`
	HeartbeatAt   time.Time                   `json:"heartbeat_at"`
	Service       CheckpointServiceState      `json:"service"`
	Repository    CheckpointRepository        `json:"repository"`
	Operation     *CheckpointControlOperation `json:"operation,omitempty"`
	Blockers      []CheckpointControlBlocker  `json:"blockers,omitempty"`
	ReservedTask  *string                     `json:"reserved_task,omitempty"`
}
