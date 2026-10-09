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
	if s.controlSeen == nil {
		s.controlSeen = map[string]string{}
	}
	commands := s.commandService(s.Config.identity)
	result := s.baseReceipt(req, digest, "applied", "applied", "candidate left the experiment; worker stop is recorded separately")
	at := now()
	result.AcceptedAt = &at
	if req.Action == "start_candidate" {
		profileID := payload.ExecutionProfileID
		if profileID == "" {
			if s.runtime.Profiles.HasMultipleProfiles() {
				return s.rejectControl(req, digest, "invalid_request", errors.New("candidate start requires a profile selection"))
			}
			profileID = s.runtime.Profiles.DefaultProfileID()
		}
		config, profile, err := s.runtime.Profiles.ResolveExecutionProfile(profileID, payload.ExecutionProfileDigest)
		if err != nil {
			return s.rejectControl(req, digest, "stale_profile", err)
		}
		// The persisted request owns the candidate ID; request replays reuse it.
		runID := req.RequestID
		receipt, err := commands.StartNativeCandidate(ctx, config, TaskClaimOptions{TaskID: payload.TaskID, RequestID: req.RequestID, WorkflowRunID: runID, ExpectedTaskVersion: payload.ExpectedTaskVersion, ExpectedGraphRevision: payload.ExpectedGraphRevision, ExpectedAuthorityVersion: req.ExpectedControlRevision, RequestedBy: req.RequestedBy, ExecutionProfile: &profile, ServiceInstanceID: req.InstanceID, controlRequestDigest: digest})
		if err != nil {
			return s.rejectControl(req, digest, controlFailureCode(err), err)
		}
		result.Status, result.Code, result.Message = "applying", "candidate_reserved", "candidate reserved; original worker start awaits its persisted identity"
		result.RunID, result.AttemptID = receipt.WorkflowRunID, receipt.AttemptID
	} else {
		receipt, err := commands.AbandonNativeCandidate(ctx, s.Config, CandidateAbandonOptions{TaskID: payload.TaskID, WorkflowRunID: payload.WorkflowRunID, RequestID: req.RequestID, ExpectedTaskVersion: payload.ExpectedTaskVersion, ExpectedCandidateVersion: payload.ExpectedCandidateVersion, RequestedBy: req.RequestedBy, Reason: payload.Reason, ControlRequestDigest: digest})
		if err != nil {
			return s.rejectControl(req, digest, controlFailureCode(err), err)
		}
		result.RunID = receipt.WorkflowRunID
		if receipt.Status == "pending_verification" {
			result.Status, result.Code, result.Message = "applying", receipt.Code, receipt.Message
		}
		if receipt.Status == "rejected" {
			result.Status, result.Code, result.Message = "rejected", receipt.Code, receipt.Message
		}
	}
	if err := s.refreshNativeSnapshot(ctx); err != nil {
		return err
	}
	if run := s.find(result.RunID); run != nil {
		result.AttemptID, result.RunRevision = run.AttemptID, run.ControlRevision
	}
	result.ControlRevision = s.State.Control.Revision
	if result.Status != "applying" {
		result.CompletedAt = ptr(now())
	}
	if err := s.writeReceipt(result); err != nil {
		return err
	}
	s.controlSeen[req.RequestID] = digest
	return nil
}

// Recover a committed task/candidate command before checking a new request's
// expiry/instance/revision. The full immutable inbox digest prevents retargeting.
func (s *Service) replayTaskControl(ctx context.Context, req productapi.ControlRequest, digest string) (bool, error) {
	if s.commandStore == nil {
		return false, nil
	}
	old, found, err := s.commandStore.LookupCommand(ctx, s.Config.identity, req.RequestID)
	if err != nil || !found || old.Kind != "claim_task" && old.Kind != "start_candidate" && old.Kind != "abandon_candidate" {
		return false, err
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
	if old.Kind == "abandon_candidate" {
		payload, err := DecodeControlPayload(req)
		if err != nil {
			return true, err
		}
		return true, s.processCandidateControl(ctx, req, payload, digest)
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
		if receipt.Status != "applying" || receipt.Action != "start_task" && receipt.Action != "start_candidate" && receipt.Action != "abandon_candidate" {
			continue
		}
		req, err := s.host.ReadControlRequest(id)
		if err != nil {
			return err
		}
		if req.RequestID != id || s.host.ControlRequestDigest(id) != receipt.RequestDigest {
			return errors.New("task control request archive changed after admission")
		}
		if req.Action == "abandon_candidate" {
			p, err := DecodeControlPayload(req)
			if err != nil {
				return err
			}
			// This is read-only PR verification followed by the same business CAS.
			if err := s.processCandidateControl(ctx, req, p, receipt.RequestDigest); err != nil {
				return err
			}
			continue
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
		} else if receipt.Action == "start_task" && (e == nil || e.Status == "unknown" || s.hasUnknownWorkerOperation(run) && !s.lifecycleEffectPending(run.RunID)) {
			receipt.Status, receipt.Code, receipt.Message = "indeterminate", "indeterminate", "original worker start needs reconciliation; no replacement was started"
		} else if receipt.Action == "start_task" && (s.candidateBusinessClosed(run) || e.Status == "failed" || e.Status == "blocked" && e.Environment == nil) {
			receipt.Status, receipt.Code, receipt.Message = "failed", "failed", firstNonEmpty(deref(run.Error), "task ended before its worker start was confirmed")
		} else if s.candidateBusinessClosed(run) {
			receipt.Status, receipt.Code, receipt.Message = "applied", "candidate_reserved", "original candidate reservation retained; business progression ended"
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
