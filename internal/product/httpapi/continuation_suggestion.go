package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"time"
)

// ContinuationSuggestionCommandPort retains business effects in the application owner while
// sharing the immutable HTTP request namespace with controls and native viewers.
type ContinuationSuggestionCommandPort interface {
	SubmitContinuationSuggestion(context.Context, CommandEnvelope) (CommandResponse, error)
	ContinuationSuggestionReceipt(context.Context, string) (*CommandResponse, error)
}

func (s *Server) CommandContinuationSuggestion(ctx context.Context, e CommandEnvelope, port ContinuationSuggestionCommandPort) (CommandResponse, error) {
	if err := e.Validate(ProjectID(s.options.ProjectIdentity), s.options.InstanceID); err != nil {
		return CommandResponse{}, &protocolError{status: 409, code: "stale_target", reason: err.Error()}
	}
	return s.commands.submitContinuationSuggestion(ctx, e, port)
}

func (c *commandRegistry) submitContinuationSuggestion(ctx context.Context, e CommandEnvelope, port ContinuationSuggestionCommandPort) (CommandResponse, error) {
	if port == nil || e.Action != "main_agent_cancel" {
		return CommandResponse{}, &protocolError{status: 400, code: "request_invalid", reason: "invalid ContinuationSuggestion application command"}
	}
	unlock, err := c.lockRequest(ctx, e.RequestID)
	if err != nil {
		return CommandResponse{}, err
	}
	defer unlock()
	raw, err := json.Marshal(e)
	if err != nil {
		return CommandResponse{}, err
	}
	record, err := c.readRecord(e.RequestID)
	if err == nil {
		if record.Digest != digestBytes(raw) || !sameEnvelope(record.Envelope, e) {
			return CommandResponse{}, &protocolError{status: 409, code: "request_id_conflict", reason: "immutable request envelope differs"}
		}
		receipt, err := port.ContinuationSuggestionReceipt(ctx, e.RequestID)
		if err != nil {
			return CommandResponse{}, err
		}
		if receipt != nil {
			if receipt.Original == nil || !sameEnvelope(*receipt.Original, e) {
				return CommandResponse{}, &protocolError{status: 409, code: "request_id_conflict", reason: "domain receipt differs from registered ContinuationSuggestion request"}
			}
			return *receipt, nil
		}
		if record.Response != nil {
			return record.response(), nil
		}
		return CommandResponse{}, &protocolError{status: 503, code: "outcome_unknown", reason: "original ContinuationSuggestion request registered; only observe, do not resubmit"}
	}
	if !os.IsNotExist(err) {
		return CommandResponse{}, err
	}
	// A lost transport file does not erase the durable domain association.
	receipt, err := port.ContinuationSuggestionReceipt(ctx, e.RequestID)
	if err != nil {
		return CommandResponse{}, err
	}
	if receipt != nil {
		if receipt.Original == nil || !sameEnvelope(*receipt.Original, e) {
			return CommandResponse{}, &protocolError{status: 409, code: "request_id_conflict", reason: "original ContinuationSuggestion request differs"}
		}
		return *receipt, nil
	}
	_, err = c.lookupAuthoritative(ctx, e.RequestID, nil, nil)
	var missing *protocolError
	if err == nil || !errors.As(err, &missing) || missing.code != "command_not_found" {
		if err != nil {
			return CommandResponse{}, err
		}
		return CommandResponse{}, &protocolError{status: 409, code: "request_id_conflict", reason: "request belongs to another operation"}
	}
	record = requestRecord{Version: 1, Digest: digestBytes(raw), Envelope: e, CreatedAt: time.Now().UTC()}
	if err = c.registerRecord(record); err != nil {
		return CommandResponse{}, err
	}
	out, err := port.SubmitContinuationSuggestion(ctx, e)
	if err != nil {
		return out, err
	}
	record.Response = outcomeOf(out)
	if err = c.writeRecord(record); err != nil {
		return out, err
	}
	return out, nil
}
