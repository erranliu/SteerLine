// Package api owns independent product inputs, receipts, targets and scalars.
package api

import (
	"encoding/json"
	"time"
)

const (
	ProcessAbsent  ProcessProbeState = "absent"
	ProcessMatches ProcessProbeState = "matches"
	ProcessReused  ProcessProbeState = "pid-reused"
	ProcessUnknown ProcessProbeState = "unknown"
)

const (
	ConnectionNoRecord      ServiceConnectionState = "no-record"
	ConnectionCorrupt       ServiceConnectionState = "corrupt"
	ConnectionOffline       ServiceConnectionState = "offline"
	ConnectionUnknown       ServiceConnectionState = "alive-unknown"
	ConnectionIdentityError ServiceConnectionState = "identity-error"
	ConnectionInitializing  ServiceConnectionState = "initializing"
	ConnectionStale         ServiceConnectionState = "stale"
	ConnectionIncompatible  ServiceConnectionState = "incompatible"
	ConnectionConfigChanged ServiceConnectionState = "config-changed"
	ConnectionConnected     ServiceConnectionState = "connected"
)

const (
	SnapshotNone           SnapshotState = "none"
	SnapshotValid          SnapshotState = "valid"
	SnapshotStale          SnapshotState = "stale"
	SnapshotReadFailed     SnapshotState = "read-failed"
	SnapshotUnknownVersion SnapshotState = "unknown-version"
)

type ProcessProbeState string

type ProcessProbe struct {
	State  ProcessProbeState `json:"state"`
	Reason string            `json:"reason,omitempty"`
}

type ServiceConnectionState string

type SnapshotState string

type ControlRequest struct {
	ProtocolVersion         int             `json:"protocol_version"`
	RequestID               string          `json:"request_id"`
	RepositoryIdentity      string          `json:"repository_identity"`
	InstanceID              string          `json:"instance_id"`
	Action                  string          `json:"action"`
	ExpectedControlRevision uint64          `json:"expected_control_revision,omitempty"`
	RunID                   string          `json:"run_id,omitempty"`
	DecisionID              string          `json:"decision_id,omitempty"`
	ExpectedRunRevision     uint64          `json:"expected_run_revision,omitempty"`
	Payload                 json.RawMessage `json:"payload,omitempty"`
	RequestedBy             string          `json:"requested_by"`
	CreatedAt               time.Time       `json:"created_at"`
	ExpiresAt               time.Time       `json:"expires_at"`
}

type ControlPayload struct {
	ExpectedTaskVersion    uint64            `json:"expected_task_version,omitempty"`
	ExpectedGraphRevision  uint64            `json:"expected_graph_revision,omitempty"`
	Environment            *HostActionTarget `json:"environment,omitempty"`
	TaskID                 string            `json:"task_id,omitempty"`
	Mode                   string            `json:"mode,omitempty"`
	ExecutionProfileID     string            `json:"execution_profile_id,omitempty"`
	ExecutionProfileDigest string            `json:"execution_profile_digest,omitempty"`
}
