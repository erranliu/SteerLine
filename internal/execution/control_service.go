package execution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
	"os"
	"strings"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
)

var errClaimConflict = errors.New("native task claim conflict")

var errTaskNotReady = errors.New("target task is no longer ready")

func checkpointControlReceipt(receipt productapi.ControlReceipt) executionmodel.ControlReceipt {
	return executionmodel.ControlReceipt{Version: receipt.Version, RequestID: receipt.RequestID, RequestDigest: receipt.RequestDigest, InstanceID: receipt.InstanceID, Action: receipt.Action,
		Status: receipt.Status, Code: receipt.Code, Message: receipt.Message, RequestedBy: receipt.RequestedBy, SubmittedAt: receipt.SubmittedAt, AcceptedAt: receipt.AcceptedAt,
		CompletedAt: receipt.CompletedAt, RunID: receipt.RunID, AttemptID: receipt.AttemptID, DecisionID: receipt.DecisionID, ControlRevision: receipt.ControlRevision, RunRevision: receipt.RunRevision}
}

func productControlReceipt(receipt executionmodel.ControlReceipt) productapi.ControlReceipt {
	return productapi.ControlReceipt{Version: receipt.Version, RequestID: receipt.RequestID, RequestDigest: receipt.RequestDigest, InstanceID: receipt.InstanceID, Action: receipt.Action,
		Status: receipt.Status, Code: receipt.Code, Message: receipt.Message, RequestedBy: receipt.RequestedBy, SubmittedAt: receipt.SubmittedAt, AcceptedAt: receipt.AcceptedAt,
		CompletedAt: receipt.CompletedAt, RunID: receipt.RunID, AttemptID: receipt.AttemptID, DecisionID: receipt.DecisionID, ControlRevision: receipt.ControlRevision, RunRevision: receipt.RunRevision}
}

func (s *Service) baseReceipt(req productapi.ControlRequest, digest string, status, code, message string) productapi.ControlReceipt {
	return productapi.ControlReceipt{Version: 1, RequestID: req.RequestID, RequestDigest: digest, InstanceID: req.InstanceID, Action: req.Action, Status: status, Code: code, Message: message, RequestedBy: req.RequestedBy, SubmittedAt: req.CreatedAt}
}

func (s *Service) rejectControl(req productapi.ControlRequest, digest, code string, err error) error {
	message := "request rejected"
	if err != nil {
		message = err.Error()
	}
	receipt := s.baseReceipt(req, digest, "rejected", code, message)
	nowAt := now()
	receipt.CompletedAt = &nowAt
	receipt.ControlRevision = s.State.Control.Revision
	if e := s.writeReceipt(receipt); e != nil {
		return fmt.Errorf("publish rejected control receipt: %w", e)
	}
	if s.controlSeen == nil {
		s.controlSeen = map[string]string{}
	}
	s.controlSeen[req.RequestID] = digest
	return nil
}

func (s *Service) finishControl(op *reconstructiontypes.ControlOperation, receipt productapi.ControlReceipt) error {
	nowAt := now()
	receipt.CompletedAt = &nowAt
	receipt.ControlRevision = s.State.Control.Revision + 1
	if op.RunID != "" {
		receipt.RunID = op.RunID
		receipt.AttemptID = op.AttemptID
		if run := s.find(op.RunID); run != nil {
			receipt.AttemptID, receipt.RunRevision = run.AttemptID, run.ControlRevision
		}
	}
	op.Phase = "completed"
	copy := checkpointControlReceipt(receipt)
	op.Result = &copy
	s.State.Control.Operation = op
	if err := s.publishCheckpoint(); err != nil {
		return err
	}
	s.injectControlFault("terminal_checkpoint")
	if err := s.writeReceipt(receipt); err != nil {
		return fmt.Errorf("terminal control receipt could not be published; checkpoint retained: %w", err)
	}
	s.injectControlFault("after_terminal_receipt")
	s.State.Control.Operation = nil
	if err := s.publishCheckpoint(); err != nil {
		return err
	}
	if s.controlSeen == nil {
		s.controlSeen = map[string]string{}
	}
	s.controlSeen[op.RequestID] = op.RequestDigest
	return nil
}

func (s *Service) nextControlRequest() (*productapi.ControlRequest, string, error) {
	ids, err := s.host.ControlRequestIDs()
	if err != nil {
		return nil, "", err
	}
	if s.controlSeen == nil {
		s.controlSeen = map[string]string{}
	}
	for _, id := range ids {
		if _, seen := s.controlSeen[id]; seen {
			continue
		}
		if existing, e := s.host.ReadControlReceipt(id); e == nil {

			s.controlSeen[id] = existing.RequestDigest
			continue
		} else if !os.IsNotExist(e) {
			return nil, "", e
		}
		requestID := id
		req, err := s.host.ReadControlRequest(requestID)
		if os.IsNotExist(err) {
			continue
		}
		digest := s.host.ControlRequestDigest(requestID)
		if err != nil {
			if s.controlSeen[id] == digest {
				continue
			}
			bad := productapi.ControlRequest{RequestID: id, InstanceID: s.State.Service.InstanceID, Action: "invalid", RequestedBy: "unknown", CreatedAt: now(), ExpiresAt: now()}
			if e := s.rejectControl(bad, digest, "invalid_request", err); e != nil {
				return nil, "", e
			}
			continue
		}
		if req.RequestID != id {
			req.RequestID = id
			if e := s.rejectControl(req, digest, "invalid_request", errors.New("request ID does not match its directory")); e != nil {
				return nil, "", e
			}
			continue
		}
		return &req, digest, nil
	}
	return nil, "", nil
}

func (s *Service) processNextControl(ctx context.Context, cancel context.CancelFunc) error {
	if ctx.Err() != nil {
		return nil
	}
	req, digest, err := s.nextControlRequest()
	if err != nil || req == nil {
		return err
	}
	if replay, err := s.replayCandidateControl(ctx, *req, digest); replay || err != nil {
		return err
	}
	checkedAt := now()
	code, validationErr := ValidateControlRequest(*req, s.State.Service.InstanceID, s.State.Repository.Identity, s.State.Control.Revision, checkedAt)
	if validationErr != nil {
		return s.rejectControl(*req, digest, code, validationErr)
	}
	payload, err := DecodeControlPayload(*req)
	if err != nil {
		return s.rejectControl(*req, digest, "invalid_request", err)
	}
	if req.Action == "stop_service" {
		receipt := s.baseReceipt(*req, digest, "applying", "applying", "service is stopping after the active operation settles")
		accepted := checkedAt
		receipt.AcceptedAt = &accepted
		receipt.ControlRevision = s.State.Control.Revision
		if err = s.writeReceipt(receipt); err != nil {
			return fmt.Errorf("publish stop acceptance: %w", err)
		}
		s.stopControl = req
		s.stopAcceptedAt = &accepted
		s.controlSeen[req.RequestID] = digest
		cancel()
		return nil
	}
	if (s.State.Service.Status != "running" && s.State.Service.Status != "error") || !s.State.Service.Ready {
		return s.rejectControl(*req, digest, "stale_instance", errors.New("service is stopping or not ready"))
	}
	if req.Action == "start_candidate" || req.Action == "abandon_candidate" {
		return s.processCandidateControl(ctx, *req, payload, digest)
	}
	if s.State.Control.Operation != nil {
		return s.rejectControl(*req, digest, "slot_busy", errors.New("another control operation has not settled"))
	}
	switch req.Action {
	case "open_terminal", "recheck_environment":
		if payload.Environment == nil {
			return s.rejectControl(*req, digest, "invalid_request", errors.New("environment target required"))
		}
		target := payload.Environment
		run := s.find(target.RunID)
		if run == nil || run.RunID != target.RunID || run.ExecutionID != target.ExecutionID || run.AttemptID != target.AttemptID {
			return s.rejectControl(*req, digest, "stale_target", executionmodel.ErrTurnStale)
		}
	case "start_task":
		profileID := payload.ExecutionProfileID
		if profileID == "" {
			if s.runtime.Profiles.HasMultipleProfiles() {
				return s.rejectControl(*req, digest, "invalid_request", errors.New("start_task requires an execution profile selection"))
			}
			profileID = s.runtime.Profiles.DefaultProfileID()
		}
		if _, _, profileErr := s.runtime.Profiles.ResolveExecutionProfile(profileID, payload.ExecutionProfileDigest); profileErr != nil {
			return s.rejectControl(*req, digest, "stale_profile", profileErr)
		}
		if s.State.Control.DispatchMode != "manual" {
			return s.rejectControl(*req, digest, "wrong_mode", errors.New("start_task requires manual dispatch mode"))
		}
		if s.State.Control.DispatchPaused {
			return s.rejectControl(*req, digest, "dispatch_paused", errors.New("dispatch is paused"))
		}
		if err = s.validateReadyTarget(ctx, payload.TaskID); err != nil {
			code := "task_not_ready"
			if errors.Is(err, errAlreadyManaged) {
				code = "already_managed"
			}
			return s.rejectControl(*req, digest, code, err)
		}
	case "set_dispatch_mode":
		if payload.Mode == "auto" && s.runtime.Profiles.DefaultProfileID() == "" {
			return s.rejectControl(*req, digest, "invalid_request", errors.New("automatic dispatch requires default_execution_profile"))
		}
	case "pause_dispatch":
		if s.State.Control.DispatchPaused {
			return s.rejectControl(*req, digest, "invalid_request", errors.New("dispatch is already paused"))
		}
	case "resume_dispatch":
		if s.State.Control.DispatchMode == "auto" && s.runtime.Profiles.DefaultProfileID() == "" {
			return s.rejectControl(*req, digest, "invalid_request", errors.New("automatic dispatch requires default_execution_profile"))
		}
		if !s.State.Control.DispatchPaused {
			return s.rejectControl(*req, digest, "invalid_request", errors.New("dispatch is not paused"))
		}
	}

	checkedAt = now()
	code, validationErr = ValidateControlRequest(*req, s.State.Service.InstanceID, s.State.Repository.Identity, s.State.Control.Revision, checkedAt)
	if validationErr != nil {
		return s.rejectControl(*req, digest, code, validationErr)
	}
	if ctx.Err() != nil {
		return nil
	}
	var runID string
	if req.Action == "start_task" {
		runID = newID()
	} else if payload.Environment != nil {
		runID = payload.Environment.RunID
	}
	attemptID := runID
	if payload.Environment != nil {
		attemptID = payload.Environment.AttemptID
	}
	op := &reconstructiontypes.ControlOperation{RequestID: req.RequestID, Action: req.Action, TaskID: payload.TaskID, Payload: append(json.RawMessage(nil), req.Payload...), Phase: "applying", RunID: runID, AttemptID: attemptID, RequestDigest: digest}
	s.State.Control.Operation = op
	if err = s.publishCheckpoint(); err != nil {
		return err
	}
	s.injectControlFault("after_checkpoint")
	applying := s.baseReceipt(*req, digest, "applying", "applying", "service accepted the request")
	accepted := checkedAt
	applying.AcceptedAt = &accepted
	applying.RunID = runID
	applying.AttemptID = attemptID
	applying.ControlRevision = s.State.Control.Revision
	if err = s.writeReceipt(applying); err != nil {
		return fmt.Errorf("control applying receipt persistence failed; no action dispatched: %w", err)
	}
	s.injectControlFault("after_applying_receipt")
	s.controlSeen[req.RequestID] = digest
	result := s.baseReceipt(*req, digest, "applied", "applied", "control action applied")
	result.AcceptedAt = &accepted
	if err = s.applyControlAction(ctx, *req, payload, op, &result); errors.Is(err, errLifecycleEffectPending) {
		return nil
	} else if err != nil {
		if errors.Is(err, errClaimConflict) {
			result.Status, result.Code, result.Message = "rejected", "claim_conflict", err.Error()
		} else if errors.Is(err, errAlreadyManaged) {
			result.Status, result.Code, result.Message = "rejected", "already_managed", err.Error()
		} else if errors.Is(err, errTaskNotReady) {
			result.Status, result.Code, result.Message = "rejected", "task_not_ready", err.Error()
		} else {
			result.Status, result.Code, result.Message = "failed", controlFailureCode(err), err.Error()
		}
		if runID != "" {
			r := s.find(payload.TaskID)
			if r == nil || r.RunID != runID {
				if errors.Is(err, context.Canceled) {
					result.Status, result.Code, result.Message = "failed", "failed", "cancelled before a run was reserved"
				}
			} else if r.ExecutionStatus == "unknown" || r.TaskStatus == "in_progress" && r.ExecutionStatus == "pending" {
				reason := "start_task side effects require manual reconciliation; request will not be replayed"
				s.State.Control.Blockers = append(s.State.Control.Blockers, reconstructiontypes.ControlBlocker{RequestID: req.RequestID, Action: req.Action, TaskID: payload.TaskID, RunID: runID, AttemptID: runID, Reason: reason})
				s.State.SlotOccupied = true
				result.Status, result.Code, result.Message = "indeterminate", "indeterminate", reason
			}
		}
		if runID != "" {
			result.RunID = runID
			result.AttemptID = runID
		}
	}
	if runID != "" {
		result.RunID = runID
		result.AttemptID = runID
		if r := s.find(payload.TaskID); r != nil && r.RunID == runID {
			result.RunRevision = r.ControlRevision
		}
	}
	return s.finishControl(op, result)
}

// A completed checkpoint is sufficient to retry receipt publication without
// repeating its action. Keep supervision and stop handling alive while the
// receipt destination is unavailable, but do not accept further mutations.
func (s *Service) pollControl(ctx context.Context, cancel context.CancelFunc) error {
	if err := s.pollCandidateControlReceipts(ctx); err != nil {
		return err
	}
	if op := s.State.Control.Operation; op != nil && op.Phase == "applying" {
		req, err := s.host.ReadControlRequest(op.RequestID)
		if err == nil && req.InstanceID == s.State.Service.InstanceID {
			stored := req
			stored.Payload = op.Payload
			if s.host.ControlRequestDigest(op.RequestID) != op.RequestDigest || RequestDigest(stored) != RequestDigest(req) {
				return errors.New("immutable admitted control request changed")
			}
			if s.lifecycleEffectPending(op.RunID) {
				return s.retainControlPublicationFailure(s.processNextControl(ctx, cancel))
			}
			payload, err := DecodeControlPayload(req)
			if err != nil {
				return err
			}
			result := s.baseReceipt(req, op.RequestDigest, "applied", "applied", "control action applied")
			if original, readErr := s.host.ReadControlReceipt(op.RequestID); readErr == nil {
				result.AcceptedAt = original.AcceptedAt
			}
			if err = s.applyControlAction(ctx, req, payload, op, &result); errors.Is(err, errLifecycleEffectPending) {
				return nil
			} else if err != nil {
				result.Status, result.Code, result.Message = "failed", controlFailureCode(err), err.Error()
			}
			return s.finishControl(op, result)
		}
	}

	if ctx.Err() != nil {
		return nil
	}
	if op := s.State.Control.Operation; op != nil && op.Phase == "completed" {
		if err := s.recoverControlOperation(); err != nil {
			return s.retainControlPublicationFailure(err)
		}
	}
	return s.retainControlPublicationFailure(s.processNextControl(ctx, cancel))
}

func (s *Service) retainControlPublicationFailure(err error) error {
	if err == nil {
		return nil
	}
	if s.store.Err() != nil || s.State.Control.Operation == nil || s.State.Control.Operation.Phase != "completed" {
		return err
	}
	s.setError(err)
	return s.publishCheckpoint()
}

func controlFailureCode(err error) string {
	if errors.Is(err, context.Canceled) {
		return "failed"
	}
	if strings.Contains(err.Error(), "critical runtime persistence") || strings.Contains(err.Error(), "receipt") {
		return "persistence_failed"
	}
	return "failed"
}

var errAlreadyManaged = errors.New("task is already managed by a local run")

func (s *Service) validateReadyTarget(ctx context.Context, taskID string) error {
	if s.find(taskID) != nil {
		return errAlreadyManaged
	}
	ready, err := s.taskWork.Ready(ctx, s.Config.identity)
	if err != nil {
		return fmt.Errorf("native task readiness could not be verified: %w", err)
	}
	for _, task := range ready {
		if task.TaskID == taskID && s.taskWork.CanExecuteType(task.Type) {
			return nil
		}
	}
	return fmt.Errorf("%w: %s is not in the current ready native task set", errTaskNotReady, taskID)
}

func (s *Service) applyControlAction(ctx context.Context, req productapi.ControlRequest, payload productapi.ControlPayload, op *reconstructiontypes.ControlOperation, result *productapi.ControlReceipt) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	switch req.Action {
	case "open_terminal", "recheck_environment":
		run := s.find(op.RunID)
		if payload.Environment == nil || run == nil {
			return executionmodel.ErrTurnStale
		}
		return s.applyEnvironmentAction(ctx, run, req.Action, *payload.Environment, req.RequestID)

	case "set_dispatch_mode", "pause_dispatch", "resume_dispatch":
		authority, err := s.domainStore.NativeAuthority(ctx, s.Config.identity)
		if err != nil {
			return err
		}
		update := NativeDispatchUpdate{RequestID: req.RequestID, ExpectedAuthorityVersion: authority.AuthorityVersion, RequestedBy: req.RequestedBy}
		switch req.Action {
		case "set_dispatch_mode":
			update.Mode = &payload.Mode
		case "pause_dispatch":
			paused := true
			update.Paused = &paused
		case "resume_dispatch":
			paused := false
			update.Paused = &paused
		}
		updated, err := (s.commandService(s.Config.identity)).SetNativeDispatchState(ctx, update)
		if err != nil {
			return err
		}
		s.State.Control.DispatchMode, s.State.Control.DispatchPaused, s.State.Control.Revision = updated.DispatchMode, updated.DispatchPaused, updated.AuthorityVersion
	case "start_task":
		profileID := payload.ExecutionProfileID
		if profileID == "" {
			profileID = s.runtime.Profiles.DefaultProfileID()
		}
		if run := s.find(op.RunID); run != nil {
			if s.lifecycleEffectPending(run.RunID) {
				return errLifecycleEffectPending
			}
			if run.RunID != op.RunID {
				return errAlreadyManaged
			}
			if s.candidateBusinessClosed(run) {
				result.Message = "original start reservation retained; candidate no longer participates"
				return nil
			}
			workflow, execution := s.workflowRecords(run)
			if workflow == nil || workflow.ExecutionProfile != nil && (workflow.ExecutionProfile.ID != profileID) {
				return nativeErr("contract_mismatch", "reserved start request differs from the fixed execution profile")
			}
			if _, err := s.executionConfigForRun(run); err != nil {
				return err
			}
			if s.workers[run.RunID] != nil {
				result.Message = "native task worker identity is persisted and running"
				return nil
			}
			task := nativeTaskRecord(s.domain.Tasks, payload.TaskID)
			attempt := s.workflowAttempt(run, run.AttemptID)
			if task == nil || workflow == nil || execution == nil || attempt == nil {
				return nativeErr("inconsistent_delivery", "reserved native start request cannot be recovered from center records")
			}
			if execution.TurnNumber > 0 && execution.SessionID != "" && execution.Process != nil {
				started := s.operationByKey("worker-turn:" + execution.ExecutionID + ":" + execution.CurrentCommandID)
				if started != nil && started.Status == "confirmed" {
					result.Message = "original worker start is durably confirmed"
					return nil
				}
			}
			if attempt.Status != "planned" {
				return nativeErr("outcome_unknown", "reserved native start request has no replayable planned attempt; fixed worker identity is retained")
			}
			if workflow.WorkflowVersion != executionmodel.Version || attempt.StepID != "worker" {
				return nativeErr("unsupported_schema", "reserved start request has an unsupported workflow")
			}
			err := s.resumeWorkerFlow(ctx)
			if err != nil {
				return err
			}
			if s.lifecycleEffectPending(run.RunID) {
				return errLifecycleEffectPending
			}
			result.Message = "native task worker reservation was resumed"
			return nil
		}
		if err := s.selectDispatchProfile(profileID, payload.ExecutionProfileDigest); err != nil {
			return err
		}
		if err := s.validateReadyTarget(ctx, payload.TaskID); err != nil {
			return err
		}
		s.targetTask, s.lastDispatched, s.explicitDispatch, s.reservedRunID = payload.TaskID, "", true, op.RunID
		defer func() { s.targetTask = ""; s.explicitDispatch = false; s.reservedRunID = "" }()
		if err := s.dispatch(ctx); err != nil {
			return err
		}
		if run := s.find(payload.TaskID); run != nil && s.lifecycleEffectPending(run.RunID) {
			return errLifecycleEffectPending
		}
		if s.lastDispatched != payload.TaskID {
			if run := s.find(payload.TaskID); run != nil && run.RunID == op.RunID {
				return fmt.Errorf("target execution failed after reservation: %s", deref(run.Error))
			}
			observedRun := ""
			if run := s.find(payload.TaskID); run != nil {
				observedRun = run.RunID
			}
			return fmt.Errorf("%w: requested native task %s was not dispatched (last_dispatched=%q observed_run=%q reserved_run=%q slot_occupied=%t)", errClaimConflict, payload.TaskID, s.lastDispatched, observedRun, op.RunID, s.State.SlotOccupied)
		}
		result.Message = "native task claimed, prepared and worker started"
	default:
		return errors.New("unsupported control action")
	}
	return nil
}

func (s *Service) recoverControlOperation() error {
	if err := s.store.Err(); err != nil {
		return err
	}
	op := s.State.Control.Operation
	if op == nil {
		return nil
	}
	if op.Phase == "completed" && op.Result != nil {
		if err := s.writeReceipt(productControlReceipt(*op.Result)); err != nil {
			return err
		}
		s.State.Control.Operation = nil
		return s.publishCheckpoint()
	}
	requestID := op.RequestID
	req, err := s.host.ReadControlRequest(requestID)
	if err != nil || req.RequestID != op.RequestID || req.Action != op.Action || s.host.ControlRequestDigest(requestID) != op.RequestDigest {

		req = productapi.ControlRequest{RequestID: op.RequestID, InstanceID: "unknown", Action: op.Action, RequestedBy: "unknown"}
	}
	hasRunEvidence := false
	for _, run := range s.runs {
		if run != nil && run.RunID == op.RunID {
			hasRunEvidence = true
		}
	}
	code, message, status := "indeterminate", "service restarted during an applying operation; external side effects were not replayed", "indeterminate"
	if op.Action == "set_dispatch_mode" || op.Action == "pause_dispatch" || op.Action == "resume_dispatch" {
		code, message, status = "failed", "service restarted before the control state transition was durably saved", "failed"
	} else if _, receiptErr := s.host.ReadControlReceipt(op.RequestID); os.IsNotExist(receiptErr) && !hasRunEvidence {

		code, message, status = "persistence_failed", "applying receipt was not published; action was not dispatched", "failed"
	}
	if op.Action == "start_task" {
		taskID := op.TaskID
		if payload, e := DecodeControlPayload(req); e == nil {
			taskID = payload.TaskID
		}
		if status == "indeterminate" {
			s.State.Control.Blockers = append(s.State.Control.Blockers, reconstructiontypes.ControlBlocker{RequestID: op.RequestID, Action: op.Action, TaskID: taskID, RunID: op.RunID, AttemptID: op.AttemptID, Reason: message})
			s.State.SlotOccupied = true
		}
	}
	nowAt := now()
	receipt := s.baseReceipt(req, op.RequestDigest, status, code, message)
	receipt.CompletedAt = &nowAt
	if old, e := s.host.ReadControlReceipt(op.RequestID); e == nil {
		receipt.AcceptedAt = old.AcceptedAt
	}
	receipt.RunID = op.RunID
	receipt.AttemptID = op.AttemptID
	receipt.ControlRevision = s.State.Control.Revision + 1
	checkpointReceipt := checkpointControlReceipt(receipt)
	op.Phase, op.Result = "completed", &checkpointReceipt
	if err = s.publishCheckpoint(); err != nil {
		return err
	}
	if err = s.writeReceipt(receipt); err != nil {
		return err
	}
	s.State.Control.Operation = nil
	return s.publishCheckpoint()
}

func (s *Service) captureStopControl() {
	if s.stopControl == nil {
		req, _, checkedAt := s.host.FindStopRequest(s.State.Service.InstanceID, nil)
		s.stopControl = req
		if req != nil {
			s.stopAcceptedAt = &checkedAt
		}
	}
}

func (s *Service) finalizeStopControl() error {
	s.captureStopControl()
	if s.stopControl == nil {
		return nil
	}
	req := *s.stopControl
	digest := s.host.ControlRequestDigest(req.RequestID)
	s.State.Service.Status = "stopped"
	s.State.Service.Ready = false
	if err := s.publishCheckpoint(); err != nil {
		return err
	}
	receipt := s.baseReceipt(req, digest, "applied", "applied", "service stopped after the active operation settled")
	completed := now()
	receipt.AcceptedAt = s.stopAcceptedAt
	receipt.CompletedAt = &completed
	receipt.ControlRevision = s.State.Control.Revision
	return s.writeReceipt(receipt)
}

func (s *Service) ensureStopApplyingReceipt() error {
	s.captureStopControl()
	if s.stopControl == nil {
		return nil
	}
	if old, err := s.host.ReadControlReceipt(s.stopControl.RequestID); err == nil && old.Status == "applying" {
		if s.stopAcceptedAt == nil {
			s.stopAcceptedAt = old.AcceptedAt
		}
		return nil
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	digest := s.host.ControlRequestDigest(s.stopControl.RequestID)
	receipt := s.baseReceipt(*s.stopControl, digest, "applying", "applying", "service is stopping after the active operation settles")
	receipt.AcceptedAt = s.stopAcceptedAt
	receipt.ControlRevision = s.State.Control.Revision
	return s.writeReceipt(receipt)
}
