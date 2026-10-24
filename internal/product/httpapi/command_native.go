package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"time"

	api "github.com/erranliu/SteerLine/internal/product/api"
)

func (c *commandRegistry) OpenNative(ctx context.Context, envelope CommandEnvelope) (CommandResponse, error) {
	unlock, err := c.lockRequest(ctx, envelope.RequestID)
	if err != nil {
		return CommandResponse{}, err
	}
	defer unlock()
	canonical, _ := json.Marshal(envelope)
	digest := digestBytes(canonical)
	record, err := c.readRecord(envelope.RequestID)
	if err == nil {
		if record.Digest != digest || !sameEnvelope(record.Envelope, envelope) {
			return CommandResponse{}, &protocolError{status: http.StatusConflict, code: "request_id_conflict", reason: "original envelope differs"}
		}
		if record.Response == nil {
			return CommandResponse{}, &protocolError{status: http.StatusServiceUnavailable, code: "outcome_unknown", reason: "native viewer result unverified"}
		}
		return record.response(), nil
	} else if !os.IsNotExist(err) {
		return CommandResponse{}, &protocolError{status: http.StatusServiceUnavailable, code: "request_record_failed", reason: err.Error()}
	}
	_, authorityErr := c.lookupAuthoritative(ctx, envelope.RequestID, nil, nil)
	var missing *protocolError
	if authorityErr == nil {
		return CommandResponse{}, &protocolError{status: http.StatusConflict, code: "request_id_conflict", reason: "request ID already belongs to a product command"}
	}
	if !errors.As(authorityErr, &missing) || missing.code != "command_not_found" {
		return CommandResponse{}, authorityErr
	}
	response := CommandResponse{RequestID: envelope.RequestID, ProjectID: envelope.ProjectID, InstanceID: envelope.InstanceID,
		Action: envelope.Action, TargetID: envelope.TargetID, Original: envelopePtr(envelope), Status: "rejected", SampledAt: time.Now().UTC()}
	var target api.NativeSessionTarget
	err = decodeStrict(envelope.Payload, &target)
	if err == nil && (envelope.ExpectedVersion != 0 || target.RunID != envelope.TargetID || target.ExecutionID == "" || target.AttemptID == "" || target.SessionID == "" || target.SessionGeneration == "") {
		err = errors.New("exact native session target required")
	}
	if err == nil && !c.nativeReady(ctx) {
		err = errors.New("service is not ready")
	}
	if err == nil {
		var candidate *api.CandidateSummary
		candidate, err = c.candidate(ctx, target.RunID)
		if err == nil && (candidate.ExecutionID != target.ExecutionID || candidate.AttemptID != target.AttemptID || candidate.SessionID != target.SessionID || candidate.SessionGeneration != target.SessionGeneration) {
			err = errors.New("stale native session target")
		}
	}
	record = requestRecord{Version: 1, Digest: digest, Envelope: envelope, CreatedAt: time.Now().UTC()}
	if err != nil {
		response.Code = "stale_target"
		response.Diagnostic = err.Error()
		record.Response = outcomeOf(response)
		if err := c.registerRecord(record); err != nil {
			return CommandResponse{}, &protocolError{status: http.StatusServiceUnavailable, code: "request_persist_failed", reason: err.Error()}
		}
		return response, nil
	}
	response.Status = "outcome_unknown"
	response.Diagnostic = "native viewer may have opened; original operation must not be replayed"
	record.Response = outcomeOf(response)
	if err := c.registerRecord(record); err != nil {
		return CommandResponse{}, &protocolError{status: http.StatusServiceUnavailable, code: "request_persist_failed", reason: err.Error()}
	}
	if err := c.nativeViewer.OpenNativeSession(ctx, target); err == nil {
		response.Status = "applied"
		response.Diagnostic = "original native viewer opened"
	} else {
		response.Diagnostic = err.Error()
	}
	record.Response = outcomeOf(response)
	if err := c.writeRecord(record); err != nil {
		return CommandResponse{}, &protocolError{status: http.StatusServiceUnavailable, code: "request_persist_failed", reason: err.Error()}
	}
	return response, nil
}

// nativeReady is sampled only after acquiring the shared command arbitration
// lock. Both the local lifecycle and the current runtime identity must be ready.
func (c *commandRegistry) nativeReady(ctx context.Context) bool {
	if !c.readiness.Ready() {
		return false
	}
	identity := runtimeIdentity(Identity{InstanceID: c.instanceID}, c.query.ReadProject(ctx, "", api.ProjectSnapshot{}))
	return identity.Ready
}

func (c *commandRegistry) candidate(ctx context.Context, runID string) (*api.CandidateSummary, error) {
	snapshot := c.query.ReadProject(ctx, "", api.ProjectSnapshot{})
	page := snapshot.Connection.Snapshot.Page
	if page == nil {
		return nil, &api.Error{Kind: api.Unavailable, Message: firstNonempty(snapshot.Connection.Diagnostic, "service runtime snapshot is unavailable")}
	}
	for _, run := range page.Runs {
		if run != nil && run.RunID == runID {
			copy := *run
			return &copy, nil
		}
	}
	return nil, &api.Error{Kind: api.UnknownRecord, Message: "execution run was not found in the current project snapshot"}
}

func (c *commandRegistry) stopReceipt(ctx context.Context, response *CommandResponse) {
	wait, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	receipt, err := c.authority.WaitControl(wait, response.RequestID, 10*time.Millisecond)
	if err == nil && receipt != nil {
		response.ControlReceipt = receipt
		response.Status = normalizedControlStatus(receipt.Status)
		response.Code = receipt.Code
		response.Diagnostic = receipt.Message
	}
}
