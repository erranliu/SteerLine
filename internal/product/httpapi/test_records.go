package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"net/url"
	"os"
	"strconv"
	"time"
)

func (s *Server) readTestRecords(ctx context.Context, path []string, q url.Values) (any, error) {
	p, ok := s.query.(api.TestRecordsPort)
	if !ok {
		return nil, &api.Error{Kind: api.Unsupported, Message: "test records unavailable"}
	}
	if len(path) == 2 && path[1] == "records" {
		allowed := map[string]bool{"test_id": true, "search": true, "level": true, "outcome": true, "retest": true, "environment": true, "source": true, "from": true, "to": true, "cursor": true, "limit": true}
		for k, values := range q {
			if !allowed[k] || len(values) != 1 {
				return nil, invalidCommand("unsupported or repeated test record filter")
			}
		}
		limit := 0
		if q.Has("limit") {
			var err error
			limit, err = strconv.Atoi(q.Get("limit"))
			if err != nil || limit < 1 || limit > 100 {
				return nil, invalidCommand("invalid limit")
			}
		}
		return p.TestRecords(ctx, api.TestRecordFilter{TestID: q.Get("test_id"), Search: q.Get("search"), Level: q.Get("level"), Outcome: q.Get("outcome"), Retest: q.Get("retest"), Environment: q.Get("environment"), Source: q.Get("source"), From: q.Get("from"), To: q.Get("to"), Cursor: q.Get("cursor"), Limit: limit})
	}
	if len(q) != 0 {
		return nil, invalidCommand("test detail query accepts no parameters")
	}
	if len(path) == 2 && path[1] == "catalogue" {
		return p.TestCatalogue(ctx)
	}
	if len(path) >= 3 && path[1] == "records" && requestIDPattern.MatchString(path[2]) {
		if len(path) == 3 {
			return p.TestRecord(ctx, path[2])
		}
		if len(path) == 4 && path[3] == "evidence" {
			return p.TestRecordEvidence(ctx, path[2])
		}
	}
	return nil, &protocolError{status: 404, code: "route_not_found", reason: "unknown test record route"}
}

func (c *commandRegistry) submitTestRecord(ctx context.Context, e CommandEnvelope) (CommandResponse, error) {
	out := CommandResponse{RequestID: e.RequestID, ProjectID: e.ProjectID, InstanceID: e.InstanceID, Action: e.Action, TargetID: e.TargetID, Original: &e, Status: "outcome_unknown", SampledAt: time.Now().UTC()}
	p, ok := c.query.(api.TestRecordsPort)
	if !ok {
		return out, &api.Error{Kind: api.Unsupported, Message: "test record registration unavailable"}
	}
	var in api.TestRecordRegistration
	if err := decodeStrict(e.Payload, &in); err != nil {
		return out, invalidCommand(err.Error())
	}
	var identity struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(in.Report, &identity) != nil || identity.ID != e.TargetID || !requestIDPattern.MatchString(identity.ID) || e.ExpectedVersion != 0 || e.ExpectedControlRevision != 0 || e.ExpectedGraphRevision != 0 {
		return out, invalidCommand("register requires exact report ID, with no task/control revisions")
	}
	canonical, _ := json.Marshal(e)
	record, err := c.readRecord(e.RequestID)
	if err == nil {
		if record.Digest != digestBytes(canonical) || !sameEnvelope(record.Envelope, e) {
			return out, &protocolError{status: 409, code: "request_id_conflict", reason: "immutable record registration differs"}
		}
		return c.readTestRegistration(ctx, e, &record)
	}
	if !os.IsNotExist(err) {
		return out, err
	}
	// Keep the transport's shared request namespace, even for non-task writes.
	_, lookupErr := c.lookupAuthoritative(ctx, e.RequestID, nil, nil)
	var missing *protocolError
	if lookupErr == nil || !errors.As(lookupErr, &missing) || missing.code != "command_not_found" {
		return out, &protocolError{status: 409, code: "request_id_conflict", reason: "request already belongs to another operation"}
	}
	record = requestRecord{Version: 1, Digest: digestBytes(canonical), Envelope: e, CreatedAt: time.Now().UTC()}
	if err = c.registerRecord(record); err != nil {
		return out, err
	}
	if !c.readiness.Ready() {
		return out, c.rejectRegistered(&record, e, 503, "service_not_ready", "project is not accepting records")
	}
	result, err := p.RegisterTestRecord(ctx, in)
	if err != nil {
		out.Status = "outcome_unknown"
		out.Code = "record_unverified"
		if registrationRejected(err) {
			out.Status = "rejected"
			out.Code = "record_rejected"
		}
		out.Diagnostic = err.Error()
	} else {
		out.Status = "applied"
		out.TestRecord = result
	}
	record.Response = outcomeOf(out)
	if err = c.writeRecord(record); err != nil {
		return out, err
	}
	return out, nil
}
func (c *commandRegistry) readTestRegistration(ctx context.Context, e CommandEnvelope, record *requestRecord) (CommandResponse, error) {
	out := record.response()
	out.Original = &e
	if record.Response != nil && record.Response.Status == "rejected" {
		return out, nil
	}
	p, ok := c.query.(api.TestRecordsPort)
	if !ok {
		return out, errors.New("test records unavailable")
	}
	var in api.TestRecordRegistration
	if decodeStrict(e.Payload, &in) != nil {
		return out, errors.New("invalid original registration")
	}
	raw, err := p.TestRecordReceipt(ctx, in)
	if err != nil {
		var business *api.Error
		if errors.As(err, &business) && business.Kind == api.IdentityConflict {
			return out, &protocolError{status: 409, code: "identity_conflict", reason: business.Message}
		}
		out.Status = "outcome_unknown"
		out.Code = "record_unverified"
		out.Diagnostic = "原登记尚未核实，请显式补登记原报告；不会重跑测试"
		if registrationRejected(err) {
			out.Status = "rejected"
			out.Code = "record_rejected"
			out.Diagnostic = err.Error()
		}
		return out, nil
	}
	var receipt struct {
		ID         string `json:"id"`
		Status     string `json:"status"`
		Diagnostic string `json:"diagnostic"`
	}
	if json.Unmarshal(raw, &receipt) != nil || receipt.ID != e.TargetID || receipt.Status != "applied" && receipt.Status != "outcome_unknown" {
		return out, errors.New("invalid registration receipt")
	}
	out.Status = receipt.Status
	out.Code = ""
	out.Diagnostic = receipt.Diagnostic
	out.TestRecord = raw
	return out, nil
}

func registrationRejected(err error) bool {
	var business *api.Error
	return errors.As(err, &business) && (business.Kind == api.InvalidInput || business.Kind == api.IdentityConflict)
}
