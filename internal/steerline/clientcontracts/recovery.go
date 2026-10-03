package clientcontracts

import (
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

type RecoveryInspection struct {
	Version                int                               `json:"version"`
	SampledAt              time.Time                         `json:"sampled_at"`
	RunID                  string                            `json:"run_id"`
	RunRevision            uint64                            `json:"run_revision"`
	ServiceInstanceID      string                            `json:"service_instance_id"`
	ServiceStatus          string                            `json:"service_status"`
	DispatchMode           string                            `json:"dispatch_mode"`
	DispatchPaused         bool                              `json:"dispatch_paused"`
	SlotOccupied           bool                              `json:"slot_occupied"`
	TaskID                 string                            `json:"task_id"`
	DecisionID             string                            `json:"decision_id,omitempty"`
	DecisionStatus         string                            `json:"decision_status,omitempty"`
	ExecutionStatus        string                            `json:"execution_status"`
	DeliveryStatus         string                            `json:"delivery_status"`
	Backend                string                            `json:"backend"`
	Protocol               string                            `json:"protocol"`
	Target                 executionapi.ExecutionTarget      `json:"target"`
	ProcessCheck           string                            `json:"process_check"`
	Workspace              *executionapi.WorkspaceInspection `json:"workspace,omitempty"`
	WorkspaceError         string                            `json:"workspace_error,omitempty"`
	NativeClaim            *contracts.TaskRecord             `json:"native_claim,omitempty"`
	ClaimError             string                            `json:"claim_error,omitempty"`
	PR                     *contracts.PRInfo                 `json:"pr,omitempty"`
	PRError                string                            `json:"pr_error,omitempty"`
	Blockers               []contracts.ControlBlocker        `json:"blockers"`
	ConcurrentActionReason string                            `json:"concurrent_action_reason,omitempty"`
}
