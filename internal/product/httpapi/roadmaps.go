package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"
)

// RoadmapCommandPort retains business effects in the application owner while
// sharing the immutable HTTP request namespace with controls and native viewers.
type RoadmapCommandPort interface {
	SubmitRoadmap(context.Context, CommandEnvelope) (CommandResponse, error)
	RoadmapReceipt(context.Context, string) (*CommandResponse, error)
}

func (s *Server) CommandRoadmap(ctx context.Context, e CommandEnvelope, port RoadmapCommandPort) (CommandResponse, error) {
	if err := e.Validate(ProjectID(s.options.ProjectIdentity), s.options.InstanceID); err != nil {
		return CommandResponse{}, &protocolError{status: 409, code: "stale_target", reason: err.Error()}
	}
	return s.commands.submitRoadmap(ctx, e, port)
}

func (c *commandRegistry) submitRoadmap(ctx context.Context, e CommandEnvelope, port RoadmapCommandPort) (CommandResponse, error) {
	if port == nil || !strings.HasPrefix(e.Action, "roadmap_") {
		return CommandResponse{}, &protocolError{status: 400, code: "request_invalid", reason: "invalid Roadmap application command"}
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
		receipt, err := port.RoadmapReceipt(ctx, e.RequestID)
		if err != nil {
			return CommandResponse{}, err
		}
		if receipt != nil {
			if receipt.Original == nil || !sameEnvelope(*receipt.Original, e) {
				return CommandResponse{}, &protocolError{status: 409, code: "request_id_conflict", reason: "domain receipt differs from registered Roadmap request"}
			}
			return *receipt, nil
		}
		if record.Response != nil {
			return record.response(), nil
		}
		return CommandResponse{}, &protocolError{status: 503, code: "outcome_unknown", reason: "original Roadmap request registered; only observe, do not resubmit"}
	}
	if !os.IsNotExist(err) {
		return CommandResponse{}, err
	}
	// A lost transport file does not erase the durable domain association.
	receipt, err := port.RoadmapReceipt(ctx, e.RequestID)
	if err != nil {
		return CommandResponse{}, err
	}
	if receipt != nil {
		if receipt.Original == nil || !sameEnvelope(*receipt.Original, e) {
			return CommandResponse{}, &protocolError{status: 409, code: "request_id_conflict", reason: "original Roadmap request differs"}
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
	out, err := port.SubmitRoadmap(ctx, e)
	if err != nil {
		return out, err
	}
	record.Response = outcomeOf(out)
	if err = c.writeRecord(record); err != nil {
		return out, err
	}
	return out, nil
}
