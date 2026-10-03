// Package clientcontracts owns pure product client values and capability ports.
package clientcontracts

import (
	"encoding/json"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
)

type Runtime = contracts.Runtime
type ProcessProbeState string

const (
	ProcessAbsent  ProcessProbeState = "absent"
	ProcessMatches ProcessProbeState = "matches"
	ProcessReused  ProcessProbeState = "pid-reused"
	ProcessUnknown ProcessProbeState = "unknown"
)

type ProcessProbe struct {
	State  ProcessProbeState `json:"state"`
	Reason string            `json:"reason,omitempty"`
}

type ServiceConnectionState string

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

// ServiceConnection keeps the snapshot, heartbeat, process identity, config
// match and protocol result separate so callers can present actionable detail.
type ServiceConnection struct {
	Snapshot        ObservationSnapshot    `json:"snapshot"`
	Process         ProcessProbe           `json:"process"`
	State           ServiceConnectionState `json:"state"`
	IdentityMatched bool                   `json:"identity_matched"`
	ConfigMatched   bool                   `json:"config_matched"`
	Protocol        int                    `json:"control_protocol"`
	Capabilities    []string               `json:"capabilities"`
	CanControl      bool                   `json:"can_control"`
	CanStop         bool                   `json:"can_stop"`
	CanStart        bool                   `json:"can_start"`
	StartupID       string                 `json:"startup_id,omitempty"`
	Diagnostic      string                 `json:"diagnostic,omitempty"`
}

type SnapshotState string

const (
	SnapshotNone           SnapshotState = "none"
	SnapshotValid          SnapshotState = "valid"
	SnapshotStale          SnapshotState = "stale"
	SnapshotReadFailed     SnapshotState = "read-failed"
	SnapshotUnknownVersion SnapshotState = "unknown-version"
)

// ObservationSnapshot carries runtime data and the independent read/freshness
// state. Runtime.Service.Status remains the last service report; this type does
// not infer process liveness from a PID.
type ObservationSnapshot struct {
	Runtime        *Runtime
	State          SnapshotState
	SampledAt      time.Time
	HeartbeatAge   time.Duration
	FreshnessLimit time.Duration
	ClockSkew      bool
	Error          string
}

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
	LastSeq          uint64 `json:"last_seq,omitempty"`
	HaveSeq          bool   `json:"have_seq,omitempty"`
	FallbackSeq      uint64 `json:"fallback_seq,omitempty"`
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
	TaskID                 string `json:"task_id,omitempty"`
	Mode                   string `json:"mode,omitempty"`
	ExecutionProfileID     string `json:"execution_profile_id,omitempty"`
	ExecutionProfileDigest string `json:"execution_profile_digest,omitempty"`
}
