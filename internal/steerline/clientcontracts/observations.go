// Package clientcontracts owns pure product client values and capability ports.
package clientcontracts

import (
	"fmt"
	"time"

	productapi "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
)

type Runtime = contracts.Runtime

// ServiceConnection keeps the snapshot, heartbeat, process identity, config
// match and protocol result separate so callers can present actionable detail.
type ServiceConnection struct {
	Snapshot        ObservationSnapshot               `json:"snapshot"`
	Process         productapi.ProcessProbe           `json:"process"`
	State           productapi.ServiceConnectionState `json:"state"`
	IdentityMatched bool                              `json:"identity_matched"`
	ConfigMatched   bool                              `json:"config_matched"`
	Protocol        int                               `json:"control_protocol"`
	Capabilities    []string                          `json:"capabilities"`
	CanControl      bool                              `json:"can_control"`
	CanStop         bool                              `json:"can_stop"`
	CanStart        bool                              `json:"can_start"`
	StartupID       string                            `json:"startup_id,omitempty"`
	Diagnostic      string                            `json:"diagnostic,omitempty"`
}

// ObservationSnapshot carries runtime data and the independent read/freshness
// state. Runtime.Service.Status remains the last service report; this type does
// not infer process liveness from a PID.
type ObservationSnapshot struct {
	Runtime        *Runtime
	State          productapi.SnapshotState
	SampledAt      time.Time
	HeartbeatAge   time.Duration
	FreshnessLimit time.Duration
	ClockSkew      bool
	Error          string
}

// EnvironmentTarget keeps a side action bound to the blocker the user saw.
func EnvironmentTarget(run *contracts.Run) (*productapi.HostActionTarget, error) {
	if run == nil || run.EnvironmentBlocker == nil {
		return nil, fmt.Errorf("run has no environment blocker")
	}
	b := run.EnvironmentBlocker
	return &productapi.HostActionTarget{RunID: run.RunID, ExecutionID: run.ExecutionID, AttemptID: run.AttemptID, TurnID: b.TurnID, BlockerID: b.ID, Version: b.Version}, nil
}
