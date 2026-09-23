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
	"sort"
	"strings"
)

var errClaimConflict = errors.New("Beads claim conflict")
var errTaskNotReady = errors.New("target task is no longer ready")

func (s *Service) writeReceipt(r ControlReceipt) error {
	path := controlReceiptPath(s.Config.RunDir, r.RequestID)
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
	root := filepath.Join(s.Config.RunDir, "control", "requests")
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() && controlIDPattern.MatchString(e.Name()) {
			ids = append(ids, e.Name())
		}
	}
	sort.Strings(ids)
	return ids, nil
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
		if s.controlSeen[id] == digest {
			continue
		}
		if existing, e := ReadControlReceipt(s.Config, id); e == nil {
			if existing.RequestDigest == digest {
				s.controlSeen[id] = digest
				continue
			}
			return nil, "", errors.New("control request ID conflicts with its existing receipt")
		} else if !os.IsNotExist(e) {
			return nil, "", e
		}
		if req.RequestID != id {
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
	req, digest, err := s.nextControlRequest()
	if err != nil || req == nil {
		return err
	}
	code, validationErr := validateControlRequest(*req, s.State.Service.InstanceID, s.State.Repository.Identity, s.State.Control.Revision, now())
	if validationErr != nil {
		return s.rejectControl(*req, digest, code, validationErr)
	}
	payload, err := decodeControlPayload(*req)
	if err != nil {
		return s.rejectControl(*req, digest, "invalid_request", err)
	}
	if req.Action == "stop_service" {
		receipt := s.baseReceipt(*req, digest, "applying", "applying", "service is stopping after the active operation settles")
		accepted := now()
		receipt.AcceptedAt = &accepted
		receipt.ControlRevision = s.State.Control.Revision
		if err = s.writeReceipt(receipt); err != nil {
			return fmt.Errorf("publish stop acceptance: %w", err)
		}
		s.stopControl = req
		s.controlSeen[req.RequestID] = digest
		cancel()
		return nil
	}
	if s.State.Service.Status != "running" || !s.State.Service.Ready {
		return s.rejectControl(*req, digest, "stale_instance", errors.New("service is stopping or not ready"))
	}
	if s.State.Control.Operation != nil {
		return s.rejectControl(*req, digest, "slot_busy", errors.New("another control operation has not settled"))
	}
	switch req.Action {
	case "start_task":
		if s.State.Control.DispatchMode != "manual" {
			return s.rejectControl(*req, digest, "wrong_mode", errors.New("start_task requires manual dispatch mode"))
		}
		if s.State.Control.DispatchPaused {
			return s.rejectControl(*req, digest, "dispatch_paused", errors.New("dispatch is paused"))
		}
		if reason := s.slotReason(""); reason != "" {
			return s.rejectControl(*req, digest, "slot_busy", errors.New(reason))
		}
		if err = s.validateReadyTarget(ctx, payload.TaskID); err != nil {
			code := "task_not_ready"
			if errors.Is(err, errAlreadyManaged) {
				code = "already_managed"
			}
			return s.rejectControl(*req, digest, code, err)
		}
	case "set_dispatch_mode":
	case "pause_dispatch":
		if s.State.Control.DispatchPaused {
			return s.rejectControl(*req, digest, "invalid_request", errors.New("dispatch is already paused"))
		}
	case "resume_dispatch":
		if !s.State.Control.DispatchPaused {
			return s.rejectControl(*req, digest, "invalid_request", errors.New("dispatch is not paused"))
		}
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
	accepted := now()
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
			result.Status, result.Code, result.Message = "rejected", "claim_conflict", "task was claimed by another actor; no other task was selected"
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
			} else if r.ExecutionStatus == "unknown" || r.BeadsStatus == "in_progress" && r.ExecutionStatus == "pending" {
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

func controlFailureCode(err error) string {
	if errors.Is(err, context.Canceled) {
		return "failed"
	}
	if strings.Contains(err.Error(), "Orca") {
		return "host_unsupported"
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
	ready, err := s.beads.Ready(ctx)
	if err != nil {
		return fmt.Errorf("task readiness could not be verified: %w", err)
	}
	found := false
	for _, task := range ready {
		if task.ID == taskID && executable(task, s.Config.ExecutableTypes) {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("%w: %s is not in the current ready executable set", errTaskNotReady, taskID)
	}
	task, err := s.beads.Show(ctx, taskID)
	if err != nil {
		return fmt.Errorf("task detail verification failed: %w", err)
	}
	if !executable(task, s.Config.ExecutableTypes) {
		return fmt.Errorf("%w: no longer ready, unassigned and executable", errTaskNotReady)
	}
	return nil
}

func (s *Service) applyControlAction(ctx context.Context, req ControlRequest, payload ControlPayload, op *ControlOperation, result *ControlReceipt) error {
	switch req.Action {
	case "set_dispatch_mode":
		s.State.Control.DispatchMode = payload.Mode
	case "pause_dispatch":
		s.State.Control.DispatchPaused = true
	case "resume_dispatch":
		s.State.Control.DispatchPaused = false
	case "start_task":
		if s.Config.hostName() == "orca" && s.Config.Orca.Fallback == "unsupported" {
			return errors.New("Orca explicit start is unsupported by this host configuration")
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
			return errClaimConflict
		}
		result.Message = "target task claimed, prepared and worker started"
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
	if err != nil {
		req = ControlRequest{RequestID: op.RequestID, InstanceID: s.State.Service.InstanceID, Action: op.Action, RequestedBy: "unknown", CreatedAt: now(), ExpiresAt: now()}
	}
	code, message, status := "indeterminate", "service restarted during an applying operation; external side effects were not replayed", "indeterminate"
	if op.Action == "set_dispatch_mode" || op.Action == "pause_dispatch" || op.Action == "resume_dispatch" {
		code, message, status = "failed", "service restarted before the control state transition was durably saved", "failed"
	} else if _, receiptErr := ReadControlReceipt(s.Config, op.RequestID); os.IsNotExist(receiptErr) {
		// Dispatch is ordered after the applying receipt. If it was never published,
		// the service could not have begun the task side effects.
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
	receipt.AcceptedAt = &nowAt
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
	root := filepath.Join(c.RunDir, "control", "requests")
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, ""
	}
	for _, e := range entries {
		if !e.IsDir() || !controlIDPattern.MatchString(e.Name()) {
			continue
		}
		path := controlRequestPath(c.RunDir, e.Name())
		req, err := readControlRequest(path)
		if err != nil || req.Action != "stop_service" || req.InstanceID != instanceID || req.RequestID != e.Name() {
			continue
		}
		if len(req.Payload) != 0 {
			continue
		}
		if _, err = decodeControlPayload(req); err != nil {
			continue
		}
		if _, err = validateControlRequest(req, instanceID, c.identity, 0, now()); err != nil {
			continue
		}
		if _, err = ReadControlReceipt(c, req.RequestID); err == nil {
			continue
		}
		return &req, rawRequestDigest(path)
	}
	return nil, ""
}

func (s *Service) finalizeStopControl() error {
	if s.stopControl == nil {
		req, _ := findStopRequest(s.Config, s.State.Service.InstanceID)
		s.stopControl = req
	}
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
	receipt.AcceptedAt = &completed
	receipt.CompletedAt = &completed
	receipt.ControlRevision = s.State.Control.Revision
	return s.writeReceipt(receipt)
}

func (s *Service) ensureStopApplyingReceipt() error {
	if s.stopControl == nil {
		req, _ := findStopRequest(s.Config, s.State.Service.InstanceID)
		s.stopControl = req
	}
	if s.stopControl == nil {
		return nil
	}
	if old, err := ReadControlReceipt(s.Config, s.stopControl.RequestID); err == nil && old.Status == "applying" {
		return nil
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	digest := rawRequestDigest(controlRequestPath(s.Config.RunDir, s.stopControl.RequestID))
	receipt := s.baseReceipt(*s.stopControl, digest, "applying", "applying", "service is stopping after the active operation settles")
	accepted := now()
	receipt.AcceptedAt = &accepted
	receipt.ControlRevision = s.State.Control.Revision
	return s.writeReceipt(receipt)
}
