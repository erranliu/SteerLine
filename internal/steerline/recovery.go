package steerline

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

type RecoveryInspection struct {
	Version                int                  `json:"version"`
	SampledAt              time.Time            `json:"sampled_at"`
	RunID                  string               `json:"run_id"`
	RunRevision            uint64               `json:"run_revision"`
	ServiceInstanceID      string               `json:"service_instance_id"`
	ServiceStatus          string               `json:"service_status"`
	DispatchMode           string               `json:"dispatch_mode"`
	DispatchPaused         bool                 `json:"dispatch_paused"`
	SlotOccupied           bool                 `json:"slot_occupied"`
	TaskID                 string               `json:"task_id"`
	DecisionID             string               `json:"decision_id,omitempty"`
	DecisionStatus         string               `json:"decision_status,omitempty"`
	ExecutionStatus        string               `json:"execution_status"`
	DeliveryStatus         string               `json:"delivery_status"`
	Backend                string               `json:"backend"`
	Protocol               string               `json:"protocol"`
	Target                 ExecutionTarget      `json:"target"`
	ProcessCheck           string               `json:"process_check"`
	Workspace              *WorkspaceInspection `json:"workspace,omitempty"`
	WorkspaceError         string               `json:"workspace_error,omitempty"`
	NativeClaim            *TaskRecord          `json:"native_claim,omitempty"`
	ClaimError             string               `json:"claim_error,omitempty"`
	PR                     *PRInfo              `json:"pr,omitempty"`
	PRError                string               `json:"pr_error,omitempty"`
	Blockers               []ControlBlocker     `json:"blockers"`
	ConcurrentActionReason string               `json:"concurrent_action_reason,omitempty"`
	RetryReason            string               `json:"retry_reason,omitempty"`
	MarkStoppedReason      string               `json:"mark_stopped_reason,omitempty"`
}

type RecoveryCommandBody struct {
	ConfirmedProcessStopped bool   `json:"confirmed_process_stopped,omitempty"`
	ConfirmationBasis       string `json:"confirmation_basis,omitempty"`
	Resolution              string `json:"resolution,omitempty"`
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
		if info, queryErr := service.prs.Query(ctx, *run.PR.URL); queryErr == nil {
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
	for _, other := range runtimeState.Runs {
		if other != nil && other.RunID != run.RunID && (other.ExecutionStatus == "pending" || other.ExecutionStatus == "running" || other.FinishTaskStatus == "waiting_decision") {
			inspection.ConcurrentActionReason = "another run has an active worker or preparation action"
			break
		}
	}
	inspection.RetryReason = inspectRetryReason(ctx, &service, run, &inspection)
	inspection.MarkStoppedReason = inspectMarkStoppedReason(run, &inspection)
	return inspection, nil
}

func recoveryService(c Config, runtimeState Runtime) Service {
	runner := Runner{Dir: c.Repository, Timeout: duration(c.CommandTimeout)}
	return Service{Config: c, State: runtimeState, prs: &PRClient{Config: c.PR, Runner: runner}}
}

func (s *Service) inspectRecoveryClaim(ctx context.Context, run *Run, report *RecoveryInspection) {
	for i := range s.State.Tasks {
		task := &s.State.Tasks[i]
		if task.TaskID == run.DomainTaskID && task.ProjectIdentity == s.Config.identity {
			report.NativeClaim = task
			return
		}
	}
	report.ClaimError = "center Task claim is missing from the current SQLite projection"
}

func hasOtherRecoveryReservation(runtimeState Runtime, runID string) bool {
	for _, other := range runtimeState.Runs {
		if other != nil && other.RunID != runID && (other.ExecutionStatus == "unknown" || other.FinishTaskStatus == "waiting_decision") {
			return true
		}
	}
	return false
}

func inspectRetryReason(ctx context.Context, service *Service, run *Run, report *RecoveryInspection) string {
	return inspectNativeRetryReason(ctx, service, run, report)
}

func inspectNativeRetryReason(ctx context.Context, service *Service, run *Run, report *RecoveryInspection) string {
	switch {
	case service.State.Service.Status != "running" || !service.State.Service.Ready:
		return "service is not ready"
	case service.State.Control.DispatchMode != "manual" || service.State.Control.DispatchPaused:
		return "native retry requires unpaused manual dispatch"
	case service.State.Control.Operation != nil || len(service.State.Control.Blockers) > 0 || report.ConcurrentActionReason != "":
		return "another control action or blocker prevents retry"
	case service.State.SlotOccupied || service.worker != nil:
		return "execution slot occupied"
	case run.WorkflowVersion != MinimalWorkflowVersion || run.ExecutionStatus != "failed" || run.FinishTaskStatus != "blocked" || run.PR.URL != nil:
		return "retry requires a settled failed native workflow without a bound PR"
	case report.NativeClaim == nil || report.NativeClaim.Status != "in_progress" || report.NativeClaim.Claimant != run.Claimant:
		return nonEmpty(report.ClaimError, "current native task claim does not match the run")
	}
	workflow, execution := service.minimalRecords(run)
	failed := service.minimalAttempt(run, run.AttemptID)
	if workflow == nil || execution == nil || failed == nil || execution.Status != "failed" || workflow.Phase != "blocked" || failed.StepID != execution.CurrentStep || failed.Status != "failed" && failed.Status != "rejected" {
		return "current native workflow, execution or failed attempt changed"
	}
	workspace, err := service.inspectMinimalWorkspace(ctx, run)
	if err != nil {
		return "native retry workspace could not be verified: " + err.Error()
	}
	if workspace.Dirty || workspace.Head != failed.InputRevision {
		return "native retry requires a clean workspace at the failed attempt input revision"
	}
	return ""
}

func inspectMarkStoppedReason(run *Run, report *RecoveryInspection) string {
	if run.WorkflowVersion == MinimalWorkflowVersion {
		return "minimal-v1 requires current step-attempt and Operation reconciliation"
	}
	if run.FinishTaskStatus == "completed" {
		return "completed runs cannot be changed"
	}
	if run.ExecutionStatus != "unknown" {
		return "mark-stopped applies only to an unknown execution"
	}
	if report.ProcessCheck == "matching process still exists" {
		return "the original worker is still alive"
	}
	if report.ConcurrentActionReason != "" {
		return report.ConcurrentActionReason
	}
	if strings.Contains(report.ProcessCheck, "conflict") || strings.Contains(report.ProcessCheck, "invalid") || strings.Contains(report.ProcessCheck, "target changed") || strings.Contains(report.ProcessCheck, "differs from the fixed binding") {
		return "execution identity conflicts with saved evidence: " + report.ProcessCheck
	}
	return "confirm that this exact original execution has stopped and record the evidence"
}

func (s *Service) recoveryTarget(runID string) *Run {
	for _, run := range s.State.Runs {
		if run != nil && run.RunID == runID {
			return run
		}
	}
	return nil
}

func (s *Service) requireRecoveryIdle(targetRunID string) error {
	if s.worker != nil {
		return errors.New("a worker is active; recovery changes are refused")
	}
	if s.State.Control.Operation != nil {
		return errors.New("a control operation is still applying")
	}
	if s.State.SlotOccupied && s.State.CurrentTask != nil && s.find(*s.State.CurrentTask) == nil {
		return errors.New("an untracked execution preparation still reserves the slot")
	}
	for _, run := range s.State.Runs {
		if run == nil || run.RunID == targetRunID {
			continue
		}
		if run.ExecutionStatus == "pending" || run.ExecutionStatus == "running" {
			return fmt.Errorf("run %s has a worker or preparation action", run.RunID)
		}
		if run.FinishTaskStatus == "waiting_decision" {
			return fmt.Errorf("run %s still holds a pending decision reservation", run.RunID)
		}
	}
	return nil
}

func (s *Service) markStoppedCore(runID, requestedBy, basis string) error {
	if requestedBy == "" {
		return errors.New("mark-stopped requires the operator")
	}
	var err error
	basis, err = validateConfirmationBasis(basis)
	if err != nil {
		return err
	}
	if err := s.requireRecoveryIdle(runID); err != nil {
		return err
	}
	run := s.recoveryTarget(runID)
	if run == nil {
		return fmt.Errorf("run %s not found", runID)
	}
	if run.WorkflowVersion == MinimalWorkflowVersion {
		return errors.New("minimal-v1 requires reconciliation of the recorded attempt and Operation; mark-stopped cannot settle step side effects")
	}
	if run.FinishTaskStatus == "completed" || run.ExecutionStatus != "unknown" {
		return errors.New("mark-stopped applies only to a non-completed unknown execution")
	}
	processState := s.inspectRunProcess(run)
	if processState == "matching process still exists" {
		return errors.New("original process or terminal is still alive")
	}
	if !stoppedEvidenceAcceptable(processState, basis) {
		return errors.New("execution identity or required external facts conflict; blocker cannot be cleared: " + processState)
	}
	run.ExecutionStatus = "failed"
	run.EndedAt = ptr(now())
	run.ManualStop = &ManualStopConfirmation{AttemptID: run.AttemptID, ConfirmedAt: now(), ConfirmedBy: requestedBy, Basis: basis}
	s.blocked(run, "Operator confirmed the original execution stopped; task and workspace remain for manual repair")
	if run.Decision != nil && run.Decision.Status != "failed" && run.Decision.Status != "applied" && run.Decision.Status != "superseded" {
		run.Decision.Status = "failed"
		run.Decision.Version++
		if run.WorkflowVersion == MinimalWorkflowVersion {
			if err := s.syncMinimalDecision(run); err != nil {
				return err
			}
		}
		s.note(run, "steer", "decision", "failed", "decision application stopped without proof", basis, decisionAnswerMeta(run))
	}
	s.recalculateCurrentReservation(runID)
	s.note(run, "steer", "recovery", "confirmed", "execution marked stopped", basis, map[string]any{"requested_by": requestedBy, "attempt_id": run.AttemptID})
	return s.save()
}

func stoppedEvidenceAcceptable(state, basis string) bool {
	switch {
	case state == "process absent" || state == "orca terminal exited":
		return strings.TrimSpace(basis) != ""
	case state == "execution identity unavailable" || strings.HasPrefix(state, "execution identity unverifiable:"):
		return strings.TrimSpace(basis) != ""
	default:
		return false
	}
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
	for _, run := range s.State.Runs {
		if run != nil && (run.ExecutionStatus == "unknown" || run.ExecutionStatus == "pending" || run.ExecutionStatus == "running" || run.FinishTaskStatus == "waiting_decision") {
			if s.State.CurrentTask == nil {
				s.State.CurrentTask = ptr(run.TaskID)
			}
		}
	}
	s.refreshSlotOccupancy()
}

func (s *Service) resolveControlBlocker(ctx context.Context, blockerID, requestedBy, basis string) (map[string]any, error) {
	if requestedBy == "" {
		return nil, errors.New("resolve-command requires operator")
	}
	var err error
	basis, err = validateConfirmationBasis(basis)
	if err != nil {
		return nil, err
	}
	if err := s.requireRecoveryIdle(""); err != nil {
		return nil, err
	}
	var blocker *ControlBlocker
	for i := range s.State.Control.Blockers {
		if s.State.Control.Blockers[i].RequestID == blockerID {
			blocker = &s.State.Control.Blockers[i]
			break
		}
	}
	if blocker == nil {
		return nil, errors.New("requested control blocker is no longer present")
	}
	command, err := s.domainStore.Command(ctx, s.Config.identity, blockerID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.New("native control blocker lacks its center command receipt; preserve its execution identity for manual reconciliation")
	}
	if err != nil {
		return nil, err
	}
	return s.resolveServiceCommandBlocker(ctx, *blocker, command, requestedBy, basis)
}

func (s *Service) resolveServiceCommandBlocker(ctx context.Context, blocker ControlBlocker, command CommandRecord, requestedBy, basis string) (map[string]any, error) {
	if command.Status != "indeterminate" || command.CommandID != blocker.RequestID || command.Kind != blocker.Action || command.TargetID != blocker.RunID || command.ProjectIdentity != s.Config.identity || !stableExecutionID(blocker.RunID) || !stableExecutionID(blocker.AttemptID) {
		return nil, errors.New("service command blocker does not match an indeterminate request and fixed run/attempt identity")
	}
	var envelope ServiceCommandPayload
	if err := decode(command.Payload, &envelope); err != nil {
		return nil, err
	}
	run := s.recoveryTarget(command.TargetID)
	if run == nil || run.FinishTaskStatus == "completed" || run.TaskID != blocker.TaskID || run.AttemptID != blocker.AttemptID || envelope.ExpectedRevision != command.ExpectedVersion || !stableExecutionID(envelope.RunAttemptID) {
		return nil, errors.New("in-flight recovery command target or saved attempt identity is missing, completed or conflicting")
	}
	resolution := ""
	switch command.Kind {
	case "mark_stopped", "retry_run":
		if !manuallyStopped(run) {
			if err := s.markStoppedCore(run.RunID, requestedBy, basis); err != nil {
				return nil, err
			}
		}
		resolution = "recorded a confirmed stop; original recovery command was not replayed"
	default:
		return nil, fmt.Errorf("service command %q does not have a resolver", command.Kind)
	}
	for i := 0; i < len(s.State.Control.Blockers); i++ {
		if s.State.Control.Blockers[i].RequestID == blocker.RequestID {
			s.State.Control.Blockers = append(s.State.Control.Blockers[:i], s.State.Control.Blockers[i+1:]...)
			break
		}
	}
	s.recalculateCurrentReservation("")
	if err := s.save(); err != nil {
		return nil, err
	}
	return map[string]any{"original_request_id": blocker.RequestID, "resolved_by": requestedBy, "resolution": resolution}, nil
}
