package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"time"

	api "github.com/erranliu/SteerLine/internal/product/api"
)

func environmentPort(v any) api.EnvironmentCheckPort { p, _ := v.(api.EnvironmentCheckPort); return p }
func isEnvironmentCheckAction(action string) bool {
	return action == api.StartEnvironmentCheck || action == api.StopEnvironmentCheck
}
func environmentNotFound(err error) bool {
	var e *api.Error
	return errors.Is(err, api.ErrEnvironmentCheckNotFound) || errors.As(err, &e) && e.Kind == api.Unsupported
}
func validateEnvironmentEnvelope(e CommandEnvelope) error {
	if e.ExpectedControlRevision != 0 || e.ExpectedGraphRevision != 0 {
		return invalidCommand("environment checks do not target task/control revisions")
	}
	if e.Action == api.StartEnvironmentCheck {
		var p api.EnvironmentCheckStartPayload
		if err := decodeStrict(e.Payload, &p); err != nil {
			return invalidCommand(err.Error())
		}
		if e.TargetID != e.ProjectID || e.ExpectedVersion != 0 || p.Execution == nil && p.ProfileID == "" || p.Execution != nil && p.ProfileID != "" || p.AccessMode != "sandbox" && p.AccessMode != "full_access" {
			return invalidCommand("start requires this project, one execution configuration and explicit access mode")
		}
	} else {
		var p map[string]json.RawMessage
		if decodeStrict(e.Payload, &p) != nil || p == nil || len(p) != 0 || !requestIDPattern.MatchString(e.TargetID) || e.ExpectedVersion == 0 {
			return invalidCommand("stop requires exact check key/revision and empty payload")
		}
	}
	return nil
}
func (s *Server) readEnvironmentChecks(ctx context.Context, path []string, q url.Values) (any, error) {
	if len(q) != 0 {
		return nil, invalidCommand("environment check queries do not accept parameters")
	}
	if s.environmentChecks == nil {
		return nil, &protocolError{status: 501, code: "unsupported", reason: "environment checks unavailable"}
	}
	if len(path) == 0 {
		rows, err := s.environmentChecks.EnvironmentChecks(ctx)
		return map[string]any{"checks": rows}, err
	}
	if len(path) == 1 && path[0] == "options" {
		return s.environmentChecks.EnvironmentCheckCatalog(ctx)
	}
	if len(path) == 1 && requestIDPattern.MatchString(path[0]) {
		r, err := s.environmentChecks.EnvironmentCheck(ctx, path[0])
		if errors.Is(err, api.ErrEnvironmentCheckNotFound) {
			return nil, &protocolError{status: 404, code: "check_not_found", reason: err.Error()}
		}
		return map[string]any{"check": r}, err
	}
	return nil, &protocolError{status: 404, code: "route_not_found", reason: "unknown environment check route"}
}
func (c *commandRegistry) submitEnvironment(ctx context.Context, e CommandEnvelope) (CommandResponse, error) {
	if err := validateEnvironmentEnvelope(e); err != nil {
		return CommandResponse{}, err
	}
	if c.environmentChecks == nil {
		return CommandResponse{}, &protocolError{status: 501, code: "unsupported", reason: "environment check application port unavailable"}
	}
	canonical, _ := json.Marshal(e)
	record, err := c.readRecord(e.RequestID)
	if err == nil {
		if record.Digest != digestBytes(canonical) || !sameEnvelope(record.Envelope, e) {
			return CommandResponse{}, &protocolError{status: 409, code: "request_id_conflict", reason: "immutable request envelope differs"}
		}
		return c.readEnvironment(ctx, e, &record)
	}
	if !os.IsNotExist(err) {
		return CommandResponse{}, err
	}
	previous, lookupErr := c.lookupAuthoritative(ctx, e.RequestID, nil, nil)
	if lookupErr == nil && previous.Original != nil && sameEnvelope(*previous.Original, e) {
		return previous, nil // The durable domain binding survives transport-record loss.
	}
	var missing *protocolError
	if lookupErr == nil || !errors.As(lookupErr, &missing) || missing.code != "command_not_found" {
		if lookupErr == nil {
			lookupErr = &protocolError{status: 409, code: "request_id_conflict", reason: "request ID belongs to another operation"}
		}
		return CommandResponse{}, lookupErr
	}
	record = requestRecord{Version: 1, Digest: digestBytes(canonical), Envelope: e, CreatedAt: time.Now().UTC()}
	if err = c.registerRecord(record); err != nil {
		return CommandResponse{}, err
	}
	if !c.readiness.Ready() {
		return CommandResponse{}, c.rejectRegistered(&record, e, 503, "service_not_ready", "project runtime is not accepting new checks")
	}
	target := e.TargetID
	if e.Action == api.StartEnvironmentCheck {
		target = c.projectIdentity
	}
	r, submitErr := c.environmentChecks.SubmitEnvironmentCheck(ctx, api.EnvironmentCheckCommand{RequestID: e.RequestID, Action: e.Action, TargetID: target, ExpectedRevision: e.ExpectedVersion, Payload: e.Payload, Original: canonical})
	response := environmentResponse(e, r)
	if submitErr != nil {
		response.Status = "outcome_unknown"
		response.Diagnostic = submitErr.Error()
		var known *api.EnvironmentCheckError
		if errors.As(submitErr, &known) {
			response.Status = "rejected"
			response.Code = known.Code
		} else if errors.Is(submitErr, api.ErrEnvironmentCheckNotFound) {
			response.Status = "rejected"
			response.Code = "check_not_found"
		}
	}
	record.Response = outcomeOf(response)
	if err = c.writeRecord(record); err != nil {
		return CommandResponse{}, err
	}
	return response, nil
}
func (c *commandRegistry) readEnvironment(ctx context.Context, e CommandEnvelope, record *requestRecord) (CommandResponse, error) {
	r, err := c.environmentChecks.EnvironmentCheckRequest(ctx, e.RequestID)
	if err != nil {
		if record != nil && record.Response != nil && record.Response.Status == "rejected" {
			return record.response(), nil
		}
		response := environmentResponse(e, r)
		response.Status = "outcome_unknown"
		response.Diagnostic = "原请求结果未核实，仅查询原请求；不重新派发"
		return response, nil
	}
	canonical, _ := json.Marshal(e)
	matched := false
	for _, op := range r.Operations {
		if op.RequestID == e.RequestID {
			var original CommandEnvelope
			if json.Unmarshal(op.Original, &original) != nil || !sameEnvelope(original, e) || digestBytes(canonical) != digestBytes(op.Original) {
				return CommandResponse{}, &protocolError{status: 409, code: "request_id_conflict", reason: "immutable check envelope differs"}
			}
			matched = true
		}
	}
	if !matched || r.Project != c.projectIdentity {
		return CommandResponse{}, &protocolError{status: 409, code: "request_id_conflict", reason: "check request association differs"}
	}
	return environmentResponse(e, r), nil
}
func environmentResponse(e CommandEnvelope, r api.EnvironmentCheckRecord) CommandResponse {
	response := CommandResponse{RequestID: e.RequestID, ProjectID: e.ProjectID, InstanceID: e.InstanceID, Action: e.Action, TargetID: e.TargetID, ExpectedVersion: e.ExpectedVersion, Original: envelopePtr(e), Status: "outcome_unknown", SampledAt: time.Now().UTC(), Diagnostic: r.Diagnostic}
	if r.ID != "" {
		response.EnvironmentCheck = &r
	}
	for _, op := range r.Operations {
		if op.RequestID == e.RequestID {
			response.Status = op.Status
			if op.Diagnostic != "" {
				response.Diagnostic = op.Diagnostic
			}
			break
		}
	}
	return response
}
