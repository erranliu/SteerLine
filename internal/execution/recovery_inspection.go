package execution

import (
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

// RecoveryInspection contains internal read facts. The app projects these
// facts to the versioned product/API summary; no product package enters the
// execution service.
type RecoveryInspection struct {
	Version                                                                                               int
	SampledAt                                                                                             time.Time
	RunID                                                                                                 string
	RunRevision                                                                                           uint64
	ServiceInstanceID, ServiceStatus, DispatchMode                                                        string
	DispatchPaused, SlotOccupied                                                                          bool
	TaskID, DecisionID, DecisionStatus, ExecutionStatus, WorkflowStage, WorkflowStatus, Backend, Protocol string
	Target                                                                                                executionbackend.ExecutionTarget
	ProcessCheck                                                                                          string
	Workspace                                                                                             *executionbackend.WorkspaceInspection
	WorkspaceError                                                                                        string
	NativeClaim                                                                                           *taskmodel.TaskRecord
	ClaimError                                                                                            string
	PR                                                                                                    *executionmodel.PRInfo
	PRError                                                                                               string
	Blockers                                                                                              []reconstructiontypes.ControlBlocker
	ConcurrentActionReason                                                                                string
}
