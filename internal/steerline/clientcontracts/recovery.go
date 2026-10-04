package clientcontracts

import (
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

type RecoveryInspection struct {
	Version                int                                   `json:"version"`
	SampledAt              time.Time                             `json:"sampled_at"`
	RunID                  string                                `json:"run_id"`
	RunRevision            uint64                                `json:"run_revision"`
	ServiceInstanceID      string                                `json:"service_instance_id"`
	ServiceStatus          string                                `json:"service_status"`
	DispatchMode           string                                `json:"dispatch_mode"`
	DispatchPaused         bool                                  `json:"dispatch_paused"`
	SlotOccupied           bool                                  `json:"slot_occupied"`
	TaskID                 string                                `json:"task_id"`
	DecisionID             string                                `json:"decision_id,omitempty"`
	DecisionStatus         string                                `json:"decision_status,omitempty"`
	ExecutionStatus        string                                `json:"execution_status"`
	DeliveryStatus         string                                `json:"delivery_status"`
	Backend                string                                `json:"backend"`
	Protocol               string                                `json:"protocol"`
	Target                 executionbackend.ExecutionTarget      `json:"target"`
	ProcessCheck           string                                `json:"process_check"`
	Workspace              *executionbackend.WorkspaceInspection `json:"workspace,omitempty"`
	WorkspaceError         string                                `json:"workspace_error,omitempty"`
	NativeClaim            *taskmodel.TaskRecord                 `json:"native_claim,omitempty"`
	ClaimError             string                                `json:"claim_error,omitempty"`
	PR                     *executionmodel.PRInfo                `json:"pr,omitempty"`
	PRError                string                                `json:"pr_error,omitempty"`
	Blockers               []contracts.ControlBlocker            `json:"blockers"`
	ConcurrentActionReason string                                `json:"concurrent_action_reason,omitempty"`
}
