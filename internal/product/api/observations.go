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

type JournalCursor struct {
	Path             string `json:"path,omitempty"`
	FileID           string `json:"file_id,omitempty"`
	Generation       uint64 `json:"generation,omitempty"`
	Offset           int64  `json:"offset,omitempty"`
	ScanOffset       int64  `json:"scan_offset,omitempty"`
	Pending          []byte `json:"pending,omitempty"`
	Dropping         bool   `json:"dropping,omitempty"`
	DroppingStart    int64  `json:"dropping_start,omitempty"`
	SkipUntilNewline bool   `json:"skip_until_newline,omitempty"`
	AnchorOffset     int64  `json:"anchor_offset,omitempty"`
	AnchorLength     int    `json:"anchor_length,omitempty"`
	AnchorHash       string `json:"anchor_hash,omitempty"`
	HeaderLength     int    `json:"header_length,omitempty"`
	HeaderHash       string `json:"header_hash,omitempty"`
	Initialized      bool   `json:"initialized,omitempty"`
}

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
	WorkflowRunID            string            `json:"workflow_run_id,omitempty"`
	ExpectedTaskVersion      uint64            `json:"expected_task_version,omitempty"`
	ExpectedGraphRevision    uint64            `json:"expected_graph_revision,omitempty"`
	ExpectedCandidateVersion uint64            `json:"expected_candidate_version,omitempty"`
	Reason                   string            `json:"reason,omitempty"`
	Environment              *HostActionTarget `json:"environment,omitempty"`
	TaskID                   string            `json:"task_id,omitempty"`
	Mode                     string            `json:"mode,omitempty"`
	ExecutionProfileID       string            `json:"execution_profile_id,omitempty"`
	ExecutionProfileDigest   string            `json:"execution_profile_digest,omitempty"`
}
