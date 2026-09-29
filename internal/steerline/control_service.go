package steerline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var errClaimConflict = errors.New("native task claim conflict")
var errTaskNotReady = errors.New("target task is no longer ready")

type stopAcceptance struct {
	Request ControlRequest
	At      time.Time
}

func (s *Service) writeReceipt(r ControlReceipt) error {
	return writeControlReceipt(s.Config.RunDir, r)
}

func writeControlReceipt(runDir string, r ControlReceipt) error {
	if !controlIDPattern.MatchString(r.RequestID) {
		return errors.New("invalid receipt request ID")
	}
	path := controlReceiptPath(runDir, r.RequestID)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return AtomicWrite(path, append(b, '\n'))
}

func (s *Service) baseReceipt(req ControlRequest, digest string, status, code, message string) ControlReceipt {
	return ControlReceipt{Version: 1, RequestID: req.RequestID, RequestDigest: digest, InstanceID: req.InstanceID, Action: req.Action, Status: status, Code: code, Message: message, RequestedBy: req.RequestedBy, SubmittedAt: req.CreatedAt}
}

func (s *Service) rejectControl(req ControlRequest, digest, code string, err error) error {
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

func (s *Service) finishControl(op *ControlOperation, receipt ControlReceipt) error {
	nowAt := now()
	receipt.CompletedAt = &nowAt
	receipt.ControlRevision = s.State.Control.Revision + 1
	if op.RunID != "" {
		receipt.RunID = op.RunID
		receipt.AttemptID = op.AttemptID
	}
	op.Phase = "completed"
	copy := receipt
	op.Result = &copy
	s.State.Control.Operation = op
	if err := s.save(); err != nil {
		return err
	}
	s.injectControlFault("terminal_checkpoint")
	if err := s.writeReceipt(receipt); err != nil {
		return fmt.Errorf("terminal control receipt could not be published; checkpoint retained: %w", err)
	}
	s.injectControlFault("after_terminal_receipt")
	s.State.Control.Operation = nil
	if err := s.save(); err != nil {
		return err
	}
	if s.controlSeen == nil {
		s.controlSeen = map[string]string{}
	}
	s.controlSeen[op.RequestID] = op.RequestDigest
	return nil
}

func rawRequestDigest(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return ""
	}
	h := sha256.New()
	limit := int64(maxControlRequestBytes)
	if info.Size() > limit {
		limit++
	} else {
		limit = info.Size()
	}
	if _, err = io.CopyN(h, f, limit); err != nil && err != io.EOF {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (s *Service) controlDirectories() ([]string, error) {
	return controlRequestIDs(s.Config.RunDir)
}

func (s *Service) nextControlRequest() (*ControlRequest, string, error) {
	ids, err := s.controlDirectories()
	if err != nil {
		return nil, "", err
	}
	if s.controlSeen == nil {
		s.controlSeen = map[string]string{}
	}
	for _, id := range ids {
		if _, seen := s.controlSeen[id]; seen {
			continue // Published requests are immutable; do not reparse history.
		}
		if existing, e := ReadControlReceipt(s.Config, id); e == nil {
			// Keep the original audit result even if an archived request was
			// subsequently damaged. A pending checkpoint is recovered separately.
			s.controlSeen[id] = existing.RequestDigest
			continue
		} else if !os.IsNotExist(e) {
			return nil, "", e
		}
		path := controlRequestPath(s.Config.RunDir, id)
		req, err := readControlRequest(path)
		if os.IsNotExist(err) {
			continue
		} // partial directory is not a request
		digest := rawRequestDigest(path)
		if err != nil {
			if s.controlSeen[id] == digest {
				continue
			}
			bad := ControlRequest{RequestID: id, InstanceID: s.State.Service.InstanceID, Action: "invalid", RequestedBy: "unknown", CreatedAt: now(), ExpiresAt: now()}
			if e := s.rejectControl(bad, digest, "invalid_request", err); e != nil {
				return nil, "", e
			}
			continue
		}
		if req.RequestID != id {
			req.RequestID = id // The validated directory owns the receipt path.
			if e := s.rejectControl(req, digest, "invalid_request", errors.New("request ID does not match its directory")); e != nil {
				return nil, "", e
			}
			continue
		}
		if req.Action == "start_task" {
			continue // The independent claim admission owns this request.
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
	checkedAt := now()
	code, validationErr := validateControlRequest(*req, s.State.Service.InstanceID, s.State.Repository.Identity, s.State.Control.Revision, checkedAt)
	if validationErr != nil {
		return s.rejectControl(*req, digest, code, validationErr)
	}
	payload, err := decodeControlPayload(*req)
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
	if s.State.Control.Operation != nil {
		return s.rejectControl(*req, digest, "slot_busy", errors.New("another control operation has not settled"))
	}
	switch req.Action {
	case "start_task":
		base := s.projectExecutionConfig()
		profileID := payload.ExecutionProfileID
		if profileID == "" {
			if len(base.ExecutionProfiles) != 0 {
				return s.rejectControl(*req, digest, "invalid_request", errors.New("start_task requires an execution profile selection"))
			}
			profileID = ProjectExecutionProfileID
		}
		if _, _, profileErr := base.ResolveExecutionProfile(profileID, payload.ExecutionProfileDigest); profileErr != nil {
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
		if payload.Mode == "auto" && s.projectExecutionConfig().DefaultProfileID() == "" {
			return s.rejectControl(*req, digest, "invalid_request", errors.New("automatic dispatch requires default_execution_profile"))
		}
	case "pause_dispatch":
		if s.State.Control.DispatchPaused {
			return s.rejectControl(*req, digest, "invalid_request", errors.New("dispatch is already paused"))
		}
	case "resume_dispatch":
		if s.State.Control.DispatchMode == "auto" && s.projectExecutionConfig().DefaultProfileID() == "" {
			return s.rejectControl(*req, digest, "invalid_request", errors.New("automatic dispatch requires default_execution_profile"))
		}
		if !s.State.Control.DispatchPaused {
			return s.rejectControl(*req, digest, "invalid_request", errors.New("dispatch is not paused"))
		}
	}
	// Eligibility reads can take longer than the request's remaining lifetime.
	// Recheck at the admission boundary, before persisting any action intent.
	checkedAt = now()
	code, validationErr = validateControlRequest(*req, s.State.Service.InstanceID, s.State.Repository.Identity, s.State.Control.Revision, checkedAt)
	if validationErr != nil {
		return s.rejectControl(*req, digest, code, validationErr)
	}
	if ctx.Err() != nil {
		return nil
	}
	var runID string
	if req.Action == "start_task" {
		runID = newID()
	}
	op := &ControlOperation{RequestID: req.RequestID, Action: req.Action, TaskID: payload.TaskID, Payload: append(json.RawMessage(nil), req.Payload...), Phase: "applying", RunID: runID, AttemptID: runID, RequestDigest: digest}
	s.State.Control.Operation = op
	if err = s.save(); err != nil {
		return err
	}
	s.injectControlFault("after_checkpoint")
	applying := s.baseReceipt(*req, digest, "applying", "applying", "service accepted the request")
	accepted := checkedAt
	applying.AcceptedAt = &accepted
	applying.RunID = runID
	applying.AttemptID = runID
	applying.ControlRevision = s.State.Control.Revision
	if err = s.writeReceipt(applying); err != nil {
		return fmt.Errorf("control applying receipt persistence failed; no action dispatched: %w", err)
	}
	s.injectControlFault("after_applying_receipt")
	s.controlSeen[req.RequestID] = digest
	result := s.baseReceipt(*req, digest, "applied", "applied", "control action applied")
	result.AcceptedAt = &accepted
	if err = s.applyControlAction(ctx, *req, payload, op, &result); err != nil {
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
				s.State.Control.Blockers = append(s.State.Control.Blockers, ControlBlocker{RequestID: req.RequestID, Action: req.Action, TaskID: payload.TaskID, RunID: runID, AttemptID: runID, Reason: reason})
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
	return s.save()
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
	ready, err := (QueryService{Store: s.domainStore, Project: s.Config.identity}).ReadyNativeTasks(ctx)
	if err != nil {
		return fmt.Errorf("native task readiness could not be verified: %w", err)
	}
	for _, task := range ready {
		if task.TaskID == taskID && contains(s.Config.ExecutableTypes, task.Type) {
			return nil
		}
	}
	return fmt.Errorf("%w: %s is not in the current ready native task set", errTaskNotReady, taskID)
}

func (s *Service) applyControlAction(ctx context.Context, req ControlRequest, payload ControlPayload, op *ControlOperation, result *ControlReceipt) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	switch req.Action {
	case "set_dispatch_mode", "pause_dispatch", "resume_dispatch":
		authority, err := s.domainStore.nativeAuthority(ctx, s.Config.identity)
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
		updated, err := (CommandService{Store: s.domainStore, Project: s.Config.identity}).SetNativeDispatchState(ctx, update)
		if err != nil {
			return err
		}
		s.State.Control.DispatchMode, s.State.Control.DispatchPaused, s.State.Control.Revision = updated.DispatchMode, updated.DispatchPaused, updated.AuthorityVersion
	case "start_task":
		profileID := payload.ExecutionProfileID
		if profileID == "" {
			profileID = ProjectExecutionProfileID
		}
		if run := s.find(payload.TaskID); run != nil {
			if run.RunID != op.RunID {
				return errAlreadyManaged
			}
			workflow, execution := s.minimalRecords(run)
			if workflow == nil || workflow.ExecutionProfile != nil && (workflow.ExecutionProfile.ID != profileID || workflow.ExecutionProfile.Digest != payload.ExecutionProfileDigest) {
				return nativeErr("contract_mismatch", "reserved start request differs from the fixed execution profile")
			}
			if err := s.useFixedRunProfile(run); err != nil {
				return err
			}
			if s.workers[run.RunID] != nil || s.worker != nil && s.active == run {
				result.Message = "native task worker identity is persisted and running"
				return nil
			}
			task := nativeTaskRecord(s.State.Tasks, payload.TaskID)
			attempt := s.minimalAttempt(run, run.AttemptID)
			if task == nil || workflow == nil || execution == nil || attempt == nil {
				return nativeErr("inconsistent_delivery", "reserved native start request cannot be recovered from center records")
			}
			if attempt.Status != "planned" {
				return nativeErr("outcome_unknown", "reserved native start request has no replayable planned attempt; fixed worker identity is retained")
			}
			var err error
			if attempt.StepID == "prepare" {
				err = s.prepareMinimalExecution(ctx, *task, run)
			} else {
				err = s.startMinimalStep(ctx, *task, run, attempt)
			}
			if err != nil {
				return err
			}
			if s.worker == nil || s.active != run || run.Process == nil {
				return nativeErr("outcome_unknown", "native start did not retain a verifiable worker identity")
			}
			result.Message = "native task worker identity is persisted and running"
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
		if err := s.writeReceipt(*op.Result); err != nil {
			return err
		}
		s.State.Control.Operation = nil
		return s.save()
	}
	path := controlRequestPath(s.Config.RunDir, op.RequestID)
	req, err := readControlRequest(path)
	if err != nil || req.RequestID != op.RequestID || req.Action != op.Action || rawRequestDigest(path) != op.RequestDigest {
		// The checkpoint owns the operation identity even if its request archive
		// is missing or damaged. Never take a different receipt path from it.
		req = ControlRequest{RequestID: op.RequestID, InstanceID: "unknown", Action: op.Action, RequestedBy: "unknown"}
	}
	hasRunEvidence := false
	for _, run := range s.State.Runs {
		if run != nil && run.RunID == op.RunID {
			hasRunEvidence = true
		}
	}
	code, message, status := "indeterminate", "service restarted during an applying operation; external side effects were not replayed", "indeterminate"
	if op.Action == "set_dispatch_mode" || op.Action == "pause_dispatch" || op.Action == "resume_dispatch" {
		code, message, status = "failed", "service restarted before the control state transition was durably saved", "failed"
	} else if _, receiptErr := ReadControlReceipt(s.Config, op.RequestID); os.IsNotExist(receiptErr) && !hasRunEvidence {
		// Both the receipt and the durable pre-claim run are absent. A missing
		// receipt alone cannot negate a run/claim that was already persisted.
		code, message, status = "persistence_failed", "applying receipt was not published; action was not dispatched", "failed"
	}
	if op.Action == "start_task" {
		taskID := op.TaskID
		if payload, e := decodeControlPayload(req); e == nil {
			taskID = payload.TaskID
		}
		if status == "indeterminate" {
			s.State.Control.Blockers = append(s.State.Control.Blockers, ControlBlocker{RequestID: op.RequestID, Action: op.Action, TaskID: taskID, RunID: op.RunID, AttemptID: op.AttemptID, Reason: message})
			s.State.SlotOccupied = true
		}
	}
	nowAt := now()
	receipt := s.baseReceipt(req, op.RequestDigest, status, code, message)
	receipt.CompletedAt = &nowAt
	if old, e := ReadControlReceipt(s.Config, op.RequestID); e == nil {
		receipt.AcceptedAt = old.AcceptedAt
	}
	receipt.RunID = op.RunID
	receipt.AttemptID = op.AttemptID
	receipt.ControlRevision = s.State.Control.Revision + 1
	op.Phase, op.Result = "completed", &receipt
	if err = s.save(); err != nil {
		return err
	}
	if err = s.writeReceipt(receipt); err != nil {
		return err
	}
	s.State.Control.Operation = nil
	return s.save()
}

func findStopRequest(c Config, instanceID string) (*ControlRequest, string) {
	req, digest, _ := findStopRequestCached(c, instanceID, nil)
	return req, digest
}

func findStopRequestCached(c Config, instanceID string, seen map[string]bool) (*ControlRequest, string, time.Time) {
	root := filepath.Join(c.RunDir, "control", "requests")
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, "", time.Time{}
	}
	for _, e := range entries {
		if !e.IsDir() || !controlIDPattern.MatchString(e.Name()) {
			continue
		}
		if seen[e.Name()] {
			continue
		}
		path := controlRequestPath(c.RunDir, e.Name())
		req, err := readControlRequest(path)
		if os.IsNotExist(err) {
			continue // A producer may still be preparing an unpublished request.
		}
		if err != nil {
			if seen != nil {
				if receipt, e := ReadControlReceipt(c, e.Name()); e == nil && terminalControlStatus(receipt.Status) {
					seen[receipt.RequestID] = true
				}
			}
			continue
		}
		if seen != nil {
			seen[e.Name()] = true
		}
		if err != nil || req.Action != "stop_service" || req.InstanceID != instanceID || req.RequestID != e.Name() {
			continue
		}
		if _, err = decodeControlPayload(req); err != nil {
			continue
		}
		checkedAt := now()
		if _, err = validateControlRequest(req, instanceID, c.identity, 0, checkedAt); err != nil {
			continue
		}
		if _, err = ReadControlReceipt(c, req.RequestID); err == nil {
			continue
		}
		return &req, rawRequestDigest(path), checkedAt
	}
	return nil, "", time.Time{}
}

func (s *Service) captureStopControl() {
	if s.stopControl == nil {
		req, _, checkedAt := findStopRequestCached(s.Config, s.State.Service.InstanceID, nil)
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
	digest := rawRequestDigest(controlRequestPath(s.Config.RunDir, req.RequestID))
	s.State.Service.Status = "stopped"
	s.State.Service.Ready = false
	if err := s.save(); err != nil {
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
	if old, err := ReadControlReceipt(s.Config, s.stopControl.RequestID); err == nil && old.Status == "applying" {
		if s.stopAcceptedAt == nil {
			s.stopAcceptedAt = old.AcceptedAt
		}
		return nil
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	digest := rawRequestDigest(controlRequestPath(s.Config.RunDir, s.stopControl.RequestID))
	receipt := s.baseReceipt(*s.stopControl, digest, "applying", "applying", "service is stopping after the active operation settles")
	receipt.AcceptedAt = s.stopAcceptedAt
	receipt.ControlRevision = s.State.Control.Revision
	return s.writeReceipt(receipt)
}
