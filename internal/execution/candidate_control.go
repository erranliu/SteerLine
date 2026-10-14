package execution

import (
	"context"
	"encoding/json"
	"errors"
	"os"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
)

// Candidate mutations have durable per-request commands. The project-wide
// control checkpoint is never held while their worker starts or stops.
func (s *Service) processCandidateControl(ctx context.Context, req productapi.ControlRequest, payload productapi.ControlPayload, digest string) error {
	if s.State.Control.DispatchMode != "manual" {
		return s.rejectControl(req, digest, "wrong_mode", errors.New("start_candidate requires manual dispatch mode"))
	}
	return s.reserveControlStart(ctx, req, payload, true)
}

// Recover a committed task/candidate command before checking a new request's
// expiry/instance/revision. The full immutable inbox digest prevents retargeting.
func (s *Service) replayTaskControl(ctx context.Context, req productapi.ControlRequest, digest string) (bool, error) {
	if s.commandStore == nil {
		return false, nil
	}
	old, found, err := s.commandStore.LookupCommand(ctx, s.Config.identity, req.RequestID)
	if err != nil || !found || old.Kind != "claim_task" && old.Kind != "start_candidate" && old.Kind != "stop_candidate" {
		return false, err
	}
	if old.Kind == "stop_candidate" {
		var original productapi.ControlRequest
		var saved executionmodel.ControlReceipt
		if json.Unmarshal(old.Payload, &original) != nil || RequestDigest(original) != RequestDigest(req) || req.Action != old.Kind {
			return true, s.rejectControl(req, digest, "idempotency_conflict", errors.New("original stop request content changed"))
		}
		if err := json.Unmarshal(old.Result, &saved); err != nil {
			return true, err
		}
		if err := s.refreshNativeSnapshot(ctx); err != nil {
			return true, err
		}
		if err := s.writeReceipt(productControlReceipt(saved)); err != nil {
			return true, err
		}
		s.controlSeen[req.RequestID] = digest
		return true, nil
	}
	var binding struct {
		Digest string `json:"control_request_digest"`
	}
	if err := json.Unmarshal(old.Payload, &binding); err != nil {
		return true, err
	}
	action := old.Kind
	if action == "claim_task" {
		action = "start_task"
	}
	if binding.Digest != digest || action != req.Action {
		return true, s.rejectControl(req, digest, "idempotency_conflict", errors.New("original task control request content changed"))
	}
	if old.Status != "accepted" {
		return true, nativeErr("outcome_unknown", "original start command has no committed reservation")
	}
	var saved executionmodel.TaskClaimReceipt
	if err := json.Unmarshal(old.Result, &saved); err != nil {
		return true, err
	}
	result := s.baseReceipt(req, digest, "applying", "candidate_reserved", "original candidate reservation recovered; no replacement was started")
	if req.Action == "start_task" {
		result.Code, result.Message = "claim_reserved", "original task reservation recovered; no replacement was started"
	}
	result.AcceptedAt, result.RunID, result.AttemptID = ptr(old.CreatedAt), saved.WorkflowRunID, saved.AttemptID
	if err := s.refreshNativeSnapshot(ctx); err != nil {
		return true, err
	}
	if run := s.find(saved.WorkflowRunID); run != nil {
		result.RunRevision = run.ControlRevision
	}
	result.ControlRevision = s.State.Control.Revision
	if err := s.writeReceipt(result); err != nil {
		return true, err
	}
	s.controlSeen[req.RequestID] = digest
	return true, nil
}

func (s *Service) pollTaskControlReceipts(ctx context.Context) error {
	ids, err := s.host.ControlRequestIDs()
	if err != nil {
		return err
	}
	for _, id := range ids {
		receipt, err := s.host.ReadControlReceipt(id)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if receipt.Status != "applying" || receipt.Action != "start_task" && receipt.Action != "start_candidate" {
			continue
		}
		req, err := s.host.ReadControlRequest(id)
		if err != nil {
			return err
		}
		if req.RequestID != id || s.host.ControlRequestDigest(id) != receipt.RequestDigest {
			return errors.New("task control request archive changed after admission")
		}
		run := s.find(receipt.RunID)
		if run == nil {
			continue
		}
		e := s.executionRecord(run)
		started := false
		if e != nil && e.SessionID != "" && e.TurnNumber > 0 && (receipt.Action != "start_task" || e.Process != nil) {
			op := s.operationByKey("worker-turn:" + e.ExecutionID + ":" + e.CurrentCommandID)
			started = op != nil && op.Status == "confirmed"
		}
		if started {
			receipt.Status, receipt.Code, receipt.Message = "applied", "applied", "original candidate worker start is durably confirmed"
			if receipt.Action == "start_task" {
				receipt.Message = "original task worker start is durably confirmed"
			}
		} else if s.candidateBusinessClosed(run) {
			if s.workflowRecord(run).CandidateStopReason == "startup_failed" {
				receipt.Status, receipt.Code, receipt.Message = "failed", "failed", firstNonEmpty(deref(run.Error), "candidate startup failed; history retained")
			} else {
				receipt.Status, receipt.Code, receipt.Message = "applied", "candidate_reserved", "original reservation retained; candidate ended without further progression"
			}
		} else if receipt.Action == "start_task" && (e == nil || e.Status == "unknown" || s.hasUnknownWorkerOperation(run) && !s.lifecycleEffectPending(run.RunID)) {
			receipt.Status, receipt.Code, receipt.Message = "indeterminate", "indeterminate", "original worker start needs reconciliation; no replacement was started"
		} else if receipt.Action == "start_task" && (e.Status == "failed" || e.Status == "blocked" && e.Environment == nil) {
			receipt.Status, receipt.Code, receipt.Message = "failed", "failed", firstNonEmpty(deref(run.Error), "worker start was not confirmed")
		} else {
			continue
		}
		receipt.CompletedAt, receipt.RunRevision = ptr(now()), run.ControlRevision
		if err := s.writeReceipt(*receipt); err != nil {
			return err
		}
	}
	return nil
}
