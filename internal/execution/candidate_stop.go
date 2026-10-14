package execution

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	m "github.com/erranliu/SteerLine/internal/execution/model"
	api "github.com/erranliu/SteerLine/internal/product/api"
	p "github.com/erranliu/SteerLine/internal/storage/ports"
)

// This mutation runs on the same service actor as lifecycle dispatch. Only its
// durable candidate transaction grants success; it never calls an adapter.
func (s *Service) processCandidateStop(ctx context.Context, req api.ControlRequest, payload api.ControlPayload, digest string) error {
	receipt := s.baseReceipt(req, digest, "applied", "applied", "candidate stopped participating; resources retained")
	at := now()
	receipt.AcceptedAt, receipt.CompletedAt, receipt.RunID = &at, &at, payload.RunID
	receipt.ControlRevision = s.State.Control.Revision
	saved, err := s.commandService(s.Config.identity).StopCandidate(ctx, req, payload, receipt)
	if err != nil {
		return s.rejectControl(req, digest, controlFailureCode(err), err)
	}
	if err = s.refreshNativeSnapshot(ctx); err != nil {
		return err
	}
	if err = s.writeReceipt(productControlReceipt(saved)); err != nil {
		return err // SQLite keeps the original result for publication recovery.
	}
	if s.controlSeen == nil {
		s.controlSeen = map[string]string{}
	}
	s.controlSeen[req.RequestID] = digest
	return nil
}

func (c CommandService) StopCandidate(ctx context.Context, req api.ControlRequest, payload api.ControlPayload, receipt api.ControlReceipt) (m.ControlReceipt, error) {
	var result m.ControlReceipt
	if req.Action != "stop_candidate" || req.RepositoryIdentity != c.Project || payload.TaskID == "" || payload.RunID == "" || payload.ExpectedRunRevision == 0 || req.RequestID == "" || req.RequestedBy == "" {
		return result, nativeErr("invalid_request", "stop requires its exact project, request and candidate identity")
	}
	bound, err := DecodeControlPayload(req)
	if err != nil || bound != payload {
		return result, nativeErr("invalid_request", "stop payload differs from its original request")
	}
	receipt.RequestID, receipt.InstanceID, receipt.Action, receipt.RunID = req.RequestID, req.InstanceID, req.Action, payload.RunID
	receipt.Status, receipt.Code = "applied", "applied"
	command := p.CommandRecord{ProjectIdentity: c.Project, CommandID: req.RequestID, IdempotencyKey: req.RequestID,
		Kind: "stop_candidate", TargetID: payload.RunID, ExpectedVersion: payload.ExpectedRunRevision, RequestedBy: req.RequestedBy,
		Payload: mustJSON(req), Status: "accepted", CreatedAt: now()}
	err = c.Store.TransactCandidate(ctx, func(tx p.CandidateTx) error {
		if old, found, err := tx.LookupCommand(ctx, c.Project, req.RequestID); err != nil {
			return err
		} else if found {
			if old.Kind != command.Kind || old.TargetID != command.TargetID || old.RequestedBy != command.RequestedBy || !bytes.Equal(old.Payload, command.Payload) || old.Status != "accepted" || len(old.Result) == 0 {
				return nativeErr("idempotency_conflict", "stop request identity was reused with different content")
			}
			return json.Unmarshal(old.Result, &result)
		}
		w, err := tx.Workflow(ctx, c.Project, payload.RunID, payload.TaskID)
		if err != nil {
			return nativeErr("stale_target", "candidate does not belong to the requested task")
		}
		if w.ControlRevision != payload.ExpectedRunRevision {
			return nativeErr("stale_revision", "candidate changed before stop; refresh its version")
		}
		receipt.RunID, receipt.RunRevision = w.WorkflowRunID, w.ControlRevision
		if !m.CandidateParticipates(w.Status) {
			receipt.Code, receipt.Message = "already_ended", "candidate already ended; original terminal facts retained"
		} else {
			count, err := tx.ParticipatingCandidateCount(ctx, c.Project, payload.TaskID)
			if err != nil {
				return err
			}
			if count <= 1 {
				return nativeErr("last_candidate", "at least one participating candidate must remain")
			}
			m.StopCandidate(&w, "manual", req.RequestedBy, command.CreatedAt)
			if err = tx.PutWorkflowRun(w); err != nil {
				return err
			}
			updated, err := tx.Workflow(ctx, c.Project, payload.RunID, payload.TaskID)
			if err != nil {
				return err
			}
			receipt.RunRevision = updated.ControlRevision
		}
		result = checkpointControlReceipt(receipt)
		command.Result = mustJSON(result)
		if _, made, err := tx.EnqueueCommand(command); err != nil {
			return err
		} else if !made {
			return errors.New("stop request was already used")
		}
		eventType := "candidate.stopped"
		if receipt.Code == "already_ended" {
			eventType = "candidate.stop.already_ended"
		}
		_, err = tx.AppendEvent(p.DomainEvent{ProjectIdentity: c.Project, TaskID: payload.TaskID, WorkflowRunID: w.WorkflowRunID,
			ExecutionID: w.SelectedResult, ExternalSource: "candidate-control", ExternalID: req.RequestID,
			Type: eventType, ReceivedAt: command.CreatedAt, Payload: command.Result})
		return err
	})
	return result, err
}
