package httpapi

import (
	"context"
	"net/http"
	"time"

	api "github.com/erranliu/SteerLine/internal/product/api"
)

func (s *commandRegistry) dispatch(ctx context.Context, envelope CommandEnvelope, parsed actionPayload, record *requestRecord, requestedBy string) (CommandResponse, error) {
	response := CommandResponse{RequestID: envelope.RequestID, ProjectID: envelope.ProjectID, InstanceID: envelope.InstanceID,
		Action: envelope.Action, TargetID: envelope.TargetID, ExpectedVersion: envelope.ExpectedVersion,
		Original: envelopePtr(envelope), SampledAt: time.Now().UTC()}
	if envelope.Action == "answer_decision" {
		payload := parsed.answer
		candidate, err := s.candidate(ctx, payload.RunID)
		if err != nil {
			return response, err
		}
		if candidate.Decision == nil || candidate.Decision.ID != payload.DecisionID || candidate.Decision.Version != envelope.ExpectedVersion ||
			candidate.ControlRevision != payload.ExpectedRunRevision || candidate.AttemptID != payload.SourceAttemptID || candidate.RecoveryVersion != *payload.ExpectedRecoveryVersion {
			return response, &protocolError{status: http.StatusConflict, code: "stale_target", reason: "decision, run revision, source attempt or recovery version changed; refresh before answering"}
		}
		if record.Decision == nil {
			if err := s.persistIntent(ctx, envelope, parsed, record, requestedBy); err != nil {
				return response, err
			}
		}
		receipt, _, err := s.commands.SubmitDecision(ctx, *record.Decision)
		if err != nil {
			return response, err
		}
		if !matchesCommand(envelope, record, receipt) {
			return response, &api.Error{Kind: api.IdentityConflict, Message: "submitted command receipt does not match its original mapped intent"}
		}
		response.Command, response.Status = &receipt, normalizedCommandStatus(envelope.Action, receipt.Status)
		return response, nil
	}
	if envelope.Action == "discard_task" {
		if record.Discard == nil {
			if err := s.persistIntent(ctx, envelope, parsed, record, requestedBy); err != nil {
				return response, err
			}
		}
		receipt, err := s.commands.DiscardTask(ctx, *record.Discard)
		if err != nil {
			return response, err
		}
		response.Discard = &receipt
		command, commandErr := s.authority.Command(ctx, envelope.RequestID)
		if commandErr != nil && !isUnknownRecord(commandErr) {
			return response, commandErr
		}
		if commandErr == nil {
			if !matchesCommand(envelope, record, command) {
				return response, &api.Error{Kind: api.IdentityConflict, Message: "discard receipt does not match its original mapped intent"}
			}
			response.Command = &command
		}
		response.Status = discardStatus(receipt.Status)
		return response, nil
	}
	if record.Control == nil {
		if err := s.persistIntent(ctx, envelope, parsed, record, requestedBy); err != nil {
			return response, err
		}
	}
	req, err := s.commands.SubmitControlInbox(ctx, *record.Control)
	if err != nil {
		return response, err
	}
	if !matchesControl(envelope, record, req) {
		return response, &api.Error{Kind: api.IdentityConflict, Message: "submitted control request does not match its original mapped intent"}
	}
	response.ControlRequest = &req
	response.Status = "submitted"
	if envelope.Action == "stop_service" {
		s.stopReceipt(ctx, &response)
	}
	return response, nil
}
