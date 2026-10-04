package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"os"

	productapi "github.com/erranliu/SteerLine/internal/product/api"
)

// Candidate mutations have durable per-request commands. The project-wide
// control checkpoint is never held while their worker starts or stops.
func (s *Service) processCandidateControl(ctx context.Context, req productapi.ControlRequest, payload productapi.ControlPayload, digest string) error {
	if s.controlSeen == nil {
		s.controlSeen = map[string]string{}
	}
	commands := CommandService{Store: s.commandStore, Project: s.Config.identity}
	result := s.baseReceipt(req, digest, "applied", "applied", "candidate left the experiment; worker stop is recorded separately")
	at := now()
	result.AcceptedAt = &at
	if req.Action == "start_candidate" {
		base := s.projectExecutionConfig()
		profileID := payload.ExecutionProfileID
		if profileID == "" {
			if len(base.ExecutionProfiles) != 0 {
				return s.rejectControl(req, digest, "invalid_request", errors.New("candidate start requires a profile selection"))
			}
			profileID = ProjectExecutionProfileID
		}
		config, profile, err := base.ResolveExecutionProfile(profileID, payload.ExecutionProfileDigest)
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

// Recover a committed candidate command before checking a new request's
// expiry/instance/revision. The full immutable inbox digest prevents retargeting.
func (s *Service) replayCandidateControl(ctx context.Context, req productapi.ControlRequest, digest string) (bool, error) {
	if s.commandStore == nil {
		return false, nil
	}
	old, found, err := s.commandStore.LookupCommand(ctx, s.Config.identity, req.RequestID)
	if err != nil || !found || old.Kind != "start_candidate" && old.Kind != "abandon_candidate" {
		return false, err
	}
	var binding struct {
		Digest string `json:"control_request_digest"`
	}
	if err := json.Unmarshal(old.Payload, &binding); err != nil {
		return true, err
	}
	if binding.Digest != digest || old.Kind != req.Action {
		return true, s.rejectControl(req, digest, "idempotency_conflict", errors.New("original candidate control request content changed"))
	}
	if old.Kind == "abandon_candidate" {
		payload, err := decodeControlPayload(req)
		if err != nil {
			return true, err
		}
		return true, s.processCandidateControl(ctx, req, payload, digest)
	}
	if old.Status != "accepted" {
		return true, nativeErr("outcome_unknown", "original candidate start command has no committed reservation")
	}
	var saved TaskClaimReceipt
	if err := json.Unmarshal(old.Result, &saved); err != nil {
		return true, err
	}
	result := s.baseReceipt(req, digest, "applying", "candidate_reserved", "original candidate reservation recovered; no replacement was started")
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

func (s *Service) pollCandidateControlReceipts(ctx context.Context) error {
	ids, err := s.controlDirectories()
	if err != nil {
		return err
	}
	for _, id := range ids {
		receipt, err := ReadControlReceipt(s.Config, id)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if receipt.Status != "applying" || receipt.Action != "start_candidate" && receipt.Action != "abandon_candidate" {
			continue
		}
		req, err := readControlRequest(controlRequestPath(s.Config.RunDir, id))
		if err != nil {
			return err
		}
		if req.RequestID != id || rawRequestDigest(controlRequestPath(s.Config.RunDir, id)) != receipt.RequestDigest {
			return errors.New("candidate request archive changed after admission")
		}
		if req.Action == "abandon_candidate" {
			p, err := decodeControlPayload(req)
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
		if s.candidateBusinessClosed(run) {
			receipt.Status, receipt.Code, receipt.Message = "applied", "candidate_reserved", "original candidate reservation retained; business progression ended"
		} else if e != nil && e.SessionID != "" && e.TurnNumber > 0 {
			op := s.operationByKey("worker-turn:" + e.ExecutionID + ":" + e.CurrentCommandID)
			if op == nil || op.Status != "confirmed" {
				continue
			}
			receipt.Status, receipt.Code, receipt.Message = "applied", "applied", "original candidate worker start is durably confirmed"
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
