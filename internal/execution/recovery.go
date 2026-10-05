package execution

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/erranliu/SteerLine/internal/execution/reconstruction"
)

// InspectRecovery reads the current SQLite projection and performs read-only
// host, workspace, claim and PR checks. It never constructs a running Service.
type RuntimeReader func(context.Context, Config) (*reconstruction.Runtime, error)

func InspectRecovery(ctx context.Context, c Config, runID string, readRuntime RuntimeReader) (RecoveryInspection, error) {
	if err := c.RequireNativeTaskAuthority(); err != nil {
		return RecoveryInspection{}, err
	}
	if strings.TrimSpace(runID) == "" {
		return RecoveryInspection{}, errors.New("recovery inspection requires a run ID")
	}
	if err := c.ResolveIdentity(ctx); err != nil {
		return RecoveryInspection{}, err
	}
	if readRuntime == nil {
		return RecoveryInspection{}, errors.New("bound runtime read port is required")
	}
	runtimeState, err := readRuntime(ctx, c)
	if err != nil {
		return RecoveryInspection{}, err
	}
	if runtimeState == nil || runtimeState.Repository.Identity != c.identity {
		return RecoveryInspection{}, errors.New("current SQLite runtime projection is unavailable or belongs to another repository")
	}
	var run *reconstruction.RunView
	for _, candidate := range runtimeState.Runs {
		if candidate != nil && candidate.RunID == runID {
			run = candidate
			break
		}
	}
	if run == nil {
		return RecoveryInspection{}, fmt.Errorf("run %s not found", runID)
	}
	inspection := RecoveryInspection{Version: 1, SampledAt: now(), RunID: run.RunID, RunRevision: run.ControlRevision, ServiceInstanceID: runtimeState.Service.InstanceID, ServiceStatus: runtimeState.Service.Status, DispatchMode: runtimeState.Control.DispatchMode, DispatchPaused: runtimeState.Control.DispatchPaused, SlotOccupied: runtimeState.SlotOccupied, TaskID: run.TaskID, ExecutionStatus: run.ExecutionStatus, DeliveryStatus: run.FinishTaskStatus, Backend: run.Backend, Protocol: run.Protocol, Target: run.Target, ProcessCheck: "not inspected", Blockers: append([]reconstruction.ControlBlocker(nil), runtimeState.Control.Blockers...)}
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

func recoveryService(c Config, runtimeState reconstruction.Runtime) Service {
	return Service{Config: c, lifecycleResolver: c.ExecutionDirectory, State: runtimeState, domain: reconstruction.Snapshot{Tasks: runtimeState.Tasks, WorkflowRuns: runtimeState.WorkflowRuns, Executions: runtimeState.Executions, Attempts: runtimeState.Attempts, Decisions: runtimeState.Decisions, Operations: runtimeState.Operations, Deliveries: runtimeState.Deliveries}, runs: runtimeState.Runs}
}

func (s *Service) inspectRecoveryClaim(ctx context.Context, run *reconstruction.RunView, report *RecoveryInspection) {
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
