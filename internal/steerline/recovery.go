package steerline

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
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
	Claim                  *Task                `json:"claim,omitempty"`
	ClaimError             string               `json:"claim_error,omitempty"`
	PR                     *PRInfo              `json:"pr,omitempty"`
	PRError                string               `json:"pr_error,omitempty"`
	ResultPath             string               `json:"result_path,omitempty"`
	Blockers               []ControlBlocker     `json:"blockers"`
	ConcurrentActionReason string               `json:"concurrent_action_reason,omitempty"`
	RetryReason            string               `json:"retry_reason,omitempty"`
	MarkStoppedReason      string               `json:"mark_stopped_reason,omitempty"`
	AcceptResultReason     string               `json:"accept_result_reason,omitempty"`
}

type RecoveryCommandBody struct {
	ConfirmedProcessStopped bool   `json:"confirmed_process_stopped,omitempty"`
	ConfirmationBasis       string `json:"confirmation_basis,omitempty"`
	ResultBase64            string `json:"result_base64,omitempty"`
	ResultSHA256            string `json:"result_sha256,omitempty"`
	Resolution              string `json:"resolution,omitempty"`
}

// InspectRecovery reads the current SQLite projection and performs read-only
// host, workspace, claim and PR checks. It never constructs a running Service.
func InspectRecovery(ctx context.Context, c Config, runID string) (RecoveryInspection, error) {
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
	inspection := RecoveryInspection{Version: 1, SampledAt: now(), RunID: run.RunID, RunRevision: run.ControlRevision, ServiceInstanceID: runtimeState.Service.InstanceID, ServiceStatus: runtimeState.Service.Status, DispatchMode: runtimeState.Control.DispatchMode, DispatchPaused: runtimeState.Control.DispatchPaused, SlotOccupied: runtimeState.SlotOccupied, TaskID: run.TaskID, ExecutionStatus: run.ExecutionStatus, DeliveryStatus: run.FinishTaskStatus, Backend: run.Backend, Protocol: run.Protocol, Target: run.Target, ProcessCheck: "not inspected", ResultPath: deref(run.ResultPath), Blockers: append([]ControlBlocker(nil), runtimeState.Control.Blockers...)}
	if run.Decision != nil {
		inspection.DecisionID, inspection.DecisionStatus = run.Decision.ID, run.Decision.Status
	}
	service := recoveryService(c, *runtimeState)
	inspection.ProcessCheck = service.inspectRunProcess(run)
	var workspace WorkspaceInspection
	var workspaceErr error
	if run.Process == nil && run.Host == "local" && run.WorkspaceIntent != nil {
		workspace, workspaceErr = service.inspectRetryWorkspace(ctx, run)
	} else {
		workspace, workspaceErr = service.readRunWorkspace(ctx, run)
	}
	if workspaceErr == nil {
		inspection.Workspace = &workspace
	} else {
		inspection.WorkspaceError = workspaceErr.Error()
	}
	if run.BeadsContext == nil {
		inspection.ClaimError = "dispatch-time task provider identity is missing"
	} else if source, contextErr := service.beads.Context(ctx); contextErr != nil {
		inspection.ClaimError = contextErr.Error()
	} else if !source.Same(*run.BeadsContext) {
		inspection.ClaimError = "task provider identity differs from the dispatch-time identity"
	} else if task, showErr := service.beads.Show(ctx, run.TaskID); showErr != nil {
		inspection.ClaimError = showErr.Error()
	} else {
		inspection.Claim = &task
	}
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
	inspection.RetryReason = inspectRetryReason(&service, run, &inspection)
	inspection.MarkStoppedReason = inspectMarkStoppedReason(run, &inspection)
	inspection.AcceptResultReason = inspectAcceptResultReason(&service, run, &inspection)
	return inspection, nil
}

func recoveryService(c Config, runtimeState Runtime) Service {
	runner := Runner{Dir: c.Repository, Timeout: duration(c.CommandTimeout)}
	return Service{Config: c, State: runtimeState, beads: Beads{Runner: runner, Command: c.BD, Claimant: c.Claimant}, prs: &PRClient{Config: c.PR, Runner: runner}}
}

func hasOtherRecoveryReservation(runtimeState Runtime, runID string) bool {
	for _, other := range runtimeState.Runs {
		if other != nil && other.RunID != runID && (other.ExecutionStatus == "unknown" || other.FinishTaskStatus == "waiting_decision") {
			return true
		}
	}
	return false
}

func inspectRetryReason(service *Service, run *Run, report *RecoveryInspection) string {
	switch {
	case service.State.Service.Status != "running" || !service.State.Service.Ready:
		return "service is not ready"
	case service.State.Control.DispatchMode != "manual":
		return "retry requires manual dispatch mode"
	case service.State.Control.DispatchPaused:
		return "dispatch is paused"
	case service.worker != nil:
		return "another worker is active"
	case service.State.Control.Operation != nil || len(service.State.Control.Blockers) > 0:
		return "an unresolved control operation or blocker prevents retry"
	case report.ConcurrentActionReason != "":
		return report.ConcurrentActionReason
	case hasOtherRecoveryReservation(service.State, run.RunID):
		return "another unknown execution or waiting decision still reserves the slot"
	case run.WorkflowVersion == MinimalWorkflowVersion:
		return "minimal-v1 must reconcile its current step attempt"
	case run.Host != "local" || run.Backend != string(BackendLocal) || run.Protocol != string(ProtocolLocalProcess):
		return "only the original local_process execution can retry"
	case run.FinishTaskStatus != "blocked" || run.ExecutionStatus != "failed" && run.ExecutionStatus != "succeeded":
		return "retry requires a blocked failed or succeeded run"
	case run.Decision != nil && run.Decision.Status != "failed" && run.Decision.Status != "applied" && run.Decision.Status != "superseded":
		return "an unresolved decision must be resolved before retry"
	case run.PR.URL != nil:
		return "retry is unavailable after a PR is bound"
	case report.ProcessCheck == "matching process still exists":
		return "the original worker is still alive"
	case report.ProcessCheck != "process absent" && report.ProcessCheck != "execution identity unavailable":
		return "the original process identity cannot be verified: " + report.ProcessCheck
	case report.Workspace == nil:
		return nonEmpty(report.WorkspaceError, "verified workspace evidence is unavailable")
	case report.Workspace.Head != report.Workspace.Base:
		return "worktree HEAD has advanced from base_commit"
	case report.Workspace.Dirty:
		return "worktree is dirty"
	case report.ClaimError != "" || report.Claim == nil || report.Claim.Status != "in_progress" || report.Claim.Assignee != run.Claimant:
		return nonEmpty(report.ClaimError, "dispatch-time task claim no longer matches")
	default:
		return ""
	}
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

func inspectAcceptResultReason(service *Service, run *Run, report *RecoveryInspection) string {
	switch {
	case run.WorkflowVersion == MinimalWorkflowVersion:
		return "minimal-v1 results must pass their current step gates"
	case run.FinishTaskStatus == "completed":
		return "completed runs cannot be changed"
	case run.FinishTaskStatus == "waiting_decision":
		return "the current decision must be handled before accepting a result"
	case run.PR.URL != nil:
		return "run already has a bound PR; recovery cannot replace or duplicate that binding"
	case run.Decision != nil && run.Decision.Status != "applied" && run.Decision.Status != "superseded":
		return "an unresolved or unverified decision cannot be bypassed by accepting a result"
	case run.FinishTaskStatus != "blocked" || run.ExecutionStatus != "unknown" && run.ExecutionStatus != "failed" && run.ExecutionStatus != "succeeded":
		return "result acceptance requires a blocked or unknown run"
	case service.worker != nil:
		return "a worker is still active"
	case report.ProcessCheck == "matching process still exists":
		return "the original worker is still alive"
	case report.ConcurrentActionReason != "":
		return report.ConcurrentActionReason
	case run.WorkspaceIntent == nil || run.WorktreePath == nil || run.WorktreeBranch == nil || run.BaseCommit == nil:
		return "complete workspace preparation evidence is missing"
	case report.Workspace == nil:
		return nonEmpty(report.WorkspaceError, "workspace identity could not be verified")
	default:
		return "confirm that the original execution stopped, select result bytes and verify the PR"
	}
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
	if requestedBy == "" || strings.TrimSpace(basis) == "" {
		return errors.New("mark-stopped requires the operator and confirmation evidence")
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
	run.ManualStop = &ManualStopConfirmation{AttemptID: run.AttemptID, ConfirmedAt: now(), ConfirmedBy: requestedBy, Basis: clip(strings.TrimSpace(basis), 2000)}
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

func (s *Service) acceptResultBytes(ctx context.Context, runID, commandID, requestedBy, basis string, data []byte, expectedDigest string) error {
	if requestedBy == "" || strings.TrimSpace(basis) == "" || !controlIDPattern.MatchString(commandID) {
		return errors.New("accept-result requires operator, confirmation evidence and request identity")
	}
	if len(data) == 0 || len(data) > 2<<20 {
		return errors.New("recovery result must be between 1 byte and 2 MiB")
	}
	sum := sha256.Sum256(data)
	if len(expectedDigest) != 64 || hex.EncodeToString(sum[:]) != expectedDigest {
		return errors.New("recovery result SHA-256 does not match the frozen preview bytes")
	}
	if err := s.requireRecoveryIdle(runID); err != nil {
		return err
	}
	run := s.recoveryTarget(runID)
	if run == nil {
		return fmt.Errorf("run %s not found", runID)
	}
	if run.WorkflowVersion == MinimalWorkflowVersion {
		return errors.New("minimal-v1 results must pass their current step gates; single-workflow accept-result is unsupported")
	}
	if run.PR.URL != nil {
		return errors.New("run already has a bound PR; recovery cannot replace or duplicate that binding")
	}
	if run.FinishTaskStatus == "completed" || run.FinishTaskStatus == "waiting_decision" || run.FinishTaskStatus != "blocked" || run.ExecutionStatus != "unknown" && run.ExecutionStatus != "failed" && run.ExecutionStatus != "succeeded" {
		return errors.New("accept-result requires a non-completed blocked run outside the decision flow")
	}
	if run.Decision != nil && run.Decision.Status != "applied" && run.Decision.Status != "superseded" {
		return errors.New("an unresolved or unverified decision cannot be bypassed by accepting a result")
	}
	if run.WorkspaceIntent == nil || run.WorktreePath == nil || run.WorktreeBranch == nil || run.BaseCommit == nil {
		return errors.New("run lacks complete workspace preparation evidence")
	}
	processState := s.inspectRunProcess(run)
	if processState == "matching process still exists" || !stoppedEvidenceAcceptable(processState, basis) {
		return errors.New("old execution cannot be accepted as stopped: " + processState)
	}
	result, err := ParseResultBytes(data, run)
	if err != nil {
		return err
	}
	if result.Outcome != "pr_submitted" || result.PRURL == nil {
		return errors.New("manual acceptance requires a submitted PR result")
	}
	workspace, err := s.readRunWorkspace(ctx, run)
	if err != nil {
		return fmt.Errorf("workspace identity verification failed: %w", err)
	}
	if err = verifyHandoff(run, result); err != nil {
		return err
	}
	info, err := s.prs.Query(ctx, *result.PRURL)
	if err != nil {
		return fmt.Errorf("real PR verification failed: %w", err)
	}
	if info.Repository != s.Config.PR.Repository || info.URL != *result.PRURL || info.BaseBranch != s.Config.Branch || info.State != "open" && info.State != "merged" {
		return errors.New("PR repository, URL, base branch or open/merged state does not match the configured delivery target")
	}
	if info.HeadRevision == "" || info.HeadRevision != workspace.Head {
		return errors.New("verified PR head does not match the inspected worktree HEAD")
	}
	for _, other := range s.State.Runs {
		if other != nil && other.RunID != run.RunID && other.PR.URL != nil && *other.PR.URL == *result.PRURL {
			return errors.New("PR is already bound to another run")
		}
	}
	managedPath := filepath.Join(s.Config.RunDir, "runs", run.RunID, "recovery", commandID, "result.json")
	if err = os.MkdirAll(filepath.Dir(managedPath), 0700); err != nil {
		return err
	}
	if err = AtomicWrite(managedPath, data); err != nil {
		return err
	}
	if err = s.bind(run, result); err != nil {
		return err
	}
	run.ExecutionStatus = "succeeded"
	run.ResultPath = ptr(managedPath)
	run.ManualStop = &ManualStopConfirmation{AttemptID: run.AttemptID, ConfirmedAt: now(), ConfirmedBy: requestedBy, Basis: clip(strings.TrimSpace(basis), 2000)}
	run.EndedAt = ptr(now())
	run.PR.State = info.State
	run.PR.QueriedAt = ptr(now())
	run.PR.MergedAt, run.PR.MergeCommit = info.MergedAt, info.MergeCommit
	s.recalculateCurrentReservation(run.RunID)
	s.note(run, "steer", "recovery", "accepted", "verified result accepted", *result.PRURL, map[string]any{"requested_by": requestedBy, "result_sha256": expectedDigest, "request_id": commandID})
	return s.save()
}

func (s *Service) resolveControlBlocker(ctx context.Context, blockerID, requestedBy, basis string) (map[string]any, error) {
	if requestedBy == "" || strings.TrimSpace(basis) == "" {
		return nil, errors.New("resolve-command requires operator and evidence")
	}
	if err := s.requireRecoveryIdle(""); err != nil {
		return nil, err
	}
	index := -1
	var blocker ControlBlocker
	for i, candidate := range s.State.Control.Blockers {
		if candidate.RequestID == blockerID {
			index, blocker = i, candidate
			break
		}
	}
	if index < 0 {
		return nil, errors.New("requested control blocker is no longer present")
	}
	if s.domainStore != nil {
		command, commandErr := s.domainStore.Command(ctx, s.Config.identity, blockerID)
		if commandErr == nil {
			return s.resolveServiceCommandBlocker(ctx, blocker, command, requestedBy, basis)
		}
		if !errors.Is(commandErr, sql.ErrNoRows) {
			return nil, commandErr
		}
	}
	req, err := ReadControlRequest(s.Config, blockerID)
	if err != nil {
		return nil, fmt.Errorf("original immutable request cannot be read: %w", err)
	}
	receipt, err := ReadVerifiedControlReceipt(s.Config, blockerID)
	if err != nil || receipt.Status != "indeterminate" || receipt.Action != blocker.Action {
		return nil, errors.New("original indeterminate receipt is missing or conflicts with its blocker")
	}
	taskID := blocker.TaskID
	if taskID == "" {
		payload, decodeErr := decodeControlPayload(*req)
		if decodeErr == nil {
			taskID = payload.TaskID
		}
	}
	var resolution string
	if run := s.recoveryTarget(blocker.RunID); run != nil {
		if err = s.markStoppedCore(run.RunID, requestedBy, basis); err != nil {
			return nil, err
		}
		resolution = "execution marked failed/blocked after explicit stop confirmation"
	} else {
		if taskID == "" {
			return nil, errors.New("original request has no task identity")
		}
		source, contextErr := s.beads.Context(ctx)
		if contextErr != nil {
			return nil, fmt.Errorf("task claim verification failed: %w", contextErr)
		}
		owned, ownedErr := s.beads.Owned(ctx)
		if ownedErr != nil {
			return nil, fmt.Errorf("owned-task verification failed: %w", ownedErr)
		}
		ownedByService := false
		for _, candidate := range owned {
			if candidate.ID == taskID {
				ownedByService = true
				break
			}
		}
		task, showErr := s.beads.Show(ctx, taskID)
		if showErr != nil {
			return nil, fmt.Errorf("task claim verification failed: %w", showErr)
		}
		if task.Status == "open" && task.Assignee == "" && !ownedByService {
			resolution = "verified no current claim or run exists; original start was not replayed"
		} else if task.Status == "in_progress" && task.Assignee == s.Config.Claimant && ownedByService {
			if err = s.reconcile(ctx); err != nil {
				return nil, err
			}
			run := s.find(taskID)
			if run == nil {
				return nil, errors.New("owned task could not be reconstructed as an orphan recovery record")
			}
			if run.BeadsContext != nil && !source.Same(*run.BeadsContext) {
				return nil, errors.New("orphan task provider identity differs from the original run evidence")
			}
			if blocker.RunID != "" && run.RunID != blocker.RunID {
				return nil, errors.New("orphan recovery identity differs from the preallocated run ID")
			}
			if err = s.markStoppedCore(run.RunID, requestedBy, basis); err != nil {
				return nil, err
			}
			resolution = "orphan claim was retained as a failed/blocked run with missing evidence left absent"
		} else {
			return nil, errors.New("task ownership or status conflicts with the original command; blocker remains")
		}
	}
	blockers := append([]ControlBlocker(nil), s.State.Control.Blockers...)
	s.State.Control.Blockers = s.State.Control.Blockers[:0]
	for _, candidate := range blockers {
		if candidate.RequestID != blockerID {
			s.State.Control.Blockers = append(s.State.Control.Blockers, candidate)
		}
	}
	receiptResult := map[string]any{"original_request_id": blockerID, "resolved_by_request_id": "", "resolved_by": requestedBy, "resolution": resolution, "basis": clip(strings.TrimSpace(basis), 2000)}
	s.recalculateCurrentReservation("")
	if err = s.save(); err != nil {
		return nil, err
	}
	return receiptResult, nil
}

func (s *Service) resolveServiceCommandBlocker(ctx context.Context, blocker ControlBlocker, command CommandRecord, requestedBy, basis string) (map[string]any, error) {
	if command.Status != "indeterminate" {
		return nil, errors.New("service command blocker does not refer to an indeterminate command")
	}
	var envelope ServiceCommandPayload
	var body RecoveryCommandBody
	if err := decode(command.Payload, &envelope); err != nil {
		return nil, err
	}
	if err := decode(envelope.Body, &body); err != nil {
		return nil, err
	}
	run := s.recoveryTarget(command.TargetID)
	if run == nil || run.FinishTaskStatus == "completed" {
		return nil, errors.New("in-flight recovery command target is missing or already completed")
	}
	resolution := ""
	switch command.Kind {
	case "accept_result":
		data, err := base64.StdEncoding.DecodeString(body.ResultBase64)
		if err != nil || len(data) == 0 || len(data) > 2<<20 || operationDigest(data) != body.ResultSHA256 {
			return nil, errors.New("original frozen recovery result is missing or fails its recorded SHA-256")
		}
		path := filepath.Join(s.Config.RunDir, "runs", run.RunID, "recovery", command.CommandID, "result.json")
		if existing, readErr := os.ReadFile(path); readErr == nil && operationDigest(existing) != body.ResultSHA256 {
			return nil, errors.New("managed recovery result path contains different bytes")
		} else if readErr != nil && !os.IsNotExist(readErr) {
			return nil, readErr
		}
		if run.FinishTaskStatus == "waiting_pr_merge" && run.ResultPath != nil && canonical(*run.ResultPath) == canonical(path) {
			result, parseErr := ParseResultBytes(data, run)
			if parseErr != nil || result.PRURL == nil {
				return nil, errors.New("already accepted managed result no longer validates")
			}
			info, queryErr := s.prs.Query(ctx, *result.PRURL)
			if queryErr != nil || info.State != "open" && info.State != "merged" || info.BaseBranch != s.Config.Branch {
				return nil, errors.New("accepted result PR could not be freshly verified")
			}
			resolution = "reconciled durable managed result and current PR state without rereading the source path"
		} else {
			if err := s.acceptResultBytes(ctx, run.RunID, command.CommandID, command.RequestedBy, body.ConfirmationBasis, data, body.ResultSHA256); err != nil {
				return nil, err
			}
			resolution = "verified and accepted the original frozen result bytes without reading or replaying an external execution"
		}
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
