package steerline

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

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
	NativeClaim            *TaskRecord                       `json:"native_claim,omitempty"`
	ClaimError             string                            `json:"claim_error,omitempty"`
	PR                     *PRInfo                           `json:"pr,omitempty"`
	PRError                string                            `json:"pr_error,omitempty"`
	Blockers               []ControlBlocker                  `json:"blockers"`
	ConcurrentActionReason string                            `json:"concurrent_action_reason,omitempty"`
}

// InspectRecovery reads the current SQLite projection and performs read-only
// host, workspace, claim and PR checks. It never constructs a running Service.
func InspectRecovery(ctx context.Context, c Config, runID string) (RecoveryInspection, error) {
	if err := requireNativeTaskAuthority(c); err != nil {
		return RecoveryInspection{}, err
	}
	if strings.TrimSpace(runID) == "" {
		return RecoveryInspection{}, errors.New("recovery inspection requires a run ID")
	}
	if err := c.ResolveIdentity(ctx); err != nil {
		return RecoveryInspection{}, err
	}
	runtimeState, err := LoadDatabaseRuntime(c)
	if err != nil {
		return RecoveryInspection{}, err
	}
	if runtimeState == nil || runtimeState.Repository.Identity != c.identity {
		return RecoveryInspection{}, errors.New("current SQLite runtime projection is unavailable or belongs to another repository")
	}
	var run *Run
	for _, candidate := range runtimeState.Runs {
		if candidate != nil && candidate.RunID == runID {
			run = candidate
			break
		}
	}
	if run == nil {
		return RecoveryInspection{}, fmt.Errorf("run %s not found", runID)
	}
	inspection := RecoveryInspection{Version: 1, SampledAt: now(), RunID: run.RunID, RunRevision: run.ControlRevision, ServiceInstanceID: runtimeState.Service.InstanceID, ServiceStatus: runtimeState.Service.Status, DispatchMode: runtimeState.Control.DispatchMode, DispatchPaused: runtimeState.Control.DispatchPaused, SlotOccupied: runtimeState.SlotOccupied, TaskID: run.TaskID, ExecutionStatus: run.ExecutionStatus, DeliveryStatus: run.FinishTaskStatus, Backend: run.Backend, Protocol: run.Protocol, Target: run.Target, ProcessCheck: "not inspected", Blockers: append([]ControlBlocker(nil), runtimeState.Control.Blockers...)}
	if run.Decision != nil {
		inspection.DecisionID, inspection.DecisionStatus = run.Decision.ID, run.Decision.Status
	}
	service := recoveryService(c, *runtimeState)
	inspection.ProcessCheck = service.inspectRunProcess(run)
	workspace, workspaceErr := service.readRunWorkspace(ctx, run)
	if workspaceErr == nil {
		inspection.Workspace = &workspace
	} else {
		inspection.WorkspaceError = workspaceErr.Error()
	}
	service.inspectRecoveryClaim(ctx, run, &inspection)
	if run.PR.URL != nil {
		fixed, configErr := service.executionConfigForRun(run)
		if configErr != nil {
			inspection.PRError = configErr.Error()
		} else if info, queryErr := publicationClient(fixed).Query(ctx, *run.PR.URL); queryErr == nil {
			inspection.PR = &info
		} else {
			inspection.PRError = queryErr.Error()
		}
	}
	if runtimeState.Control.Operation != nil {
		inspection.ConcurrentActionReason = "another control operation is applying"
	}
	if runtimeState.SlotOccupied && runtimeState.CurrentTask != nil {
		tracked := false
		for _, candidate := range runtimeState.Runs {
			if candidate != nil && candidate.TaskID == *runtimeState.CurrentTask {
				tracked = true
				break
			}
		}
		if !tracked {
			inspection.ConcurrentActionReason = "an untracked execution preparation still reserves the slot"
		}
	}
	return inspection, nil
}

func recoveryService(c Config, runtimeState Runtime) Service {
	return Service{Config: c, State: runtimeState, domain: DomainSnapshot{Tasks: runtimeState.Tasks, WorkflowRuns: runtimeState.WorkflowRuns, Executions: runtimeState.Executions, Attempts: runtimeState.Attempts, Decisions: runtimeState.Decisions, Operations: runtimeState.Operations}, runs: runtimeState.Runs}
}

func (s *Service) inspectRecoveryClaim(ctx context.Context, run *Run, report *RecoveryInspection) {
	for i := range s.domain.Tasks {
		task := &s.domain.Tasks[i]
		if task.TaskID == run.DomainTaskID && task.ProjectIdentity == s.Config.identity {
			report.NativeClaim = task
			return
		}
	}
	report.ClaimError = "center Task claim is missing from the current SQLite projection"
}

func validateConfirmationBasis(basis string) (string, error) {
	basis = strings.TrimSpace(basis)
	if basis == "" || !utf8.ValidString(basis) {
		return "", errors.New("recovery confirmation requires a nonempty UTF-8 evidence basis")
	}
	if utf8.RuneCountInString(basis) > 2000 {
		return "", errors.New("recovery confirmation exceeds the 2000 character audit limit; the basis was not truncated")
	}
	return basis, nil
}

func (s *Service) recalculateCurrentReservation(settledRunID string) {
	if s.State.CurrentTask != nil {
		if current := s.find(*s.State.CurrentTask); current != nil && current.RunID == settledRunID {
			s.State.CurrentTask = nil
		}
	}
	for _, run := range s.runs {
		if run != nil && (run.ExecutionStatus == "unknown" || run.ExecutionStatus == "pending" || run.ExecutionStatus == "running" || run.FinishTaskStatus == "waiting_decision") {
			if s.State.CurrentTask == nil {
				s.State.CurrentTask = ptr(run.TaskID)
			}
		}
	}
	s.refreshSlotOccupancy()
}
