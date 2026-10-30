package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"sync"
	"time"

	api "github.com/erranliu/SteerLine/internal/product/api"
)

// commandRegistry owns one request-ID namespace across normal commands and
// native-viewer operations. It has no HTTP request/response or Server access.
type commandRegistry struct {
	projectIdentity string
	instanceID      string
	capabilities    []string
	query           ProjectQueryPort
	commands        CommandPort
	authority       AuthorityPort
	nativeViewer    NativeSessionPort
	readiness       *readinessState
	records         requestRecordAccess
	mu              sync.Locker
	requestGates    map[string]*requestGate
}

type requestGate struct {
	busy  chan struct{}
	users int
}

// Arbitration is per immutable request. A slow unrelated command cannot hold
// up stop publication; cancelled waiters leave no background lock acquisition.
func (c *commandRegistry) lockRequest(ctx context.Context, id string) (func(), error) {
	c.mu.Lock()
	if c.requestGates == nil {
		c.requestGates = make(map[string]*requestGate)
	}
	gate := c.requestGates[id]
	if gate == nil {
		gate = &requestGate{busy: make(chan struct{}, 1)}
		c.requestGates[id] = gate
	}
	gate.users++
	c.mu.Unlock()
	releaseUser := func() {
		c.mu.Lock()
		gate.users--
		if gate.users == 0 {
			delete(c.requestGates, id)
		}
		c.mu.Unlock()
	}
	select {
	case gate.busy <- struct{}{}:
		return func() { <-gate.busy; releaseUser() }, nil
	case <-ctx.Done():
		releaseUser()
		return nil, ctx.Err()
	}
}

func newCommandRegistry(options Options, ports Ports, readiness *readinessState, records requestRecordAccess) *commandRegistry {
	return &commandRegistry{projectIdentity: options.ProjectIdentity, instanceID: options.InstanceID,
		capabilities: append([]string(nil), options.Capabilities...), query: ports.Query, commands: ports.Commands,
		authority: ports.Authority, nativeViewer: ports.NativeViewer, readiness: readiness, records: records, mu: &sync.Mutex{}}
}

func (c *commandRegistry) readRecord(requestID string) (requestRecord, error) {
	return c.records.Read(requestID)
}

func (c *commandRegistry) writeRecord(record requestRecord) error { return c.records.Write(record) }

func (c *commandRegistry) registerRecord(record requestRecord) error {
	return c.records.Register(record)
}

func (c *commandRegistry) Submit(ctx context.Context, envelope CommandEnvelope, caller string) (CommandResponse, error) {
	if envelope.Action == "stop_candidate" {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
	}
	unlock, err := c.lockRequest(ctx, envelope.RequestID)
	if err != nil {
		return CommandResponse{}, err
	}
	defer unlock()
	canonical, err := json.Marshal(envelope)
	if err != nil {
		return CommandResponse{}, &protocolError{status: http.StatusBadRequest, code: "request_invalid", reason: err.Error()}
	}
	digest := digestBytes(canonical)
	var parsed actionPayload
	var validationErr error
	parsedSet := false
	record, readErr := c.readRecord(envelope.RequestID)
	if readErr == nil {
		if record.Digest != digest || !sameEnvelope(record.Envelope, envelope) {
			return CommandResponse{}, &protocolError{status: http.StatusConflict, code: "request_id_conflict", reason: "request_id is already bound to a different immutable envelope"}
		}
		previous, lookupErr := c.lookupAuthoritative(ctx, envelope.RequestID, &envelope, &record)
		if lookupErr == nil {
			return previous, nil
		}
		var missing *protocolError
		if !errors.As(lookupErr, &missing) || missing.code != "command_not_found" {
			return CommandResponse{}, lookupErr
		}
		if record.Response != nil {
			response := record.response()
			if response.Status != "rejected" && response.Status != "outcome_unknown" {
				response.Status = "outcome_unknown"
				response.Diagnostic = "the original transport accepted this envelope but the product authority no longer has its receipt; do not replay it"
			}
			return response, nil
		}
	} else if !os.IsNotExist(readErr) {
		return CommandResponse{}, &protocolError{status: http.StatusServiceUnavailable, code: "request_record_unreadable", reason: readErr.Error()}
	} else {
		_, lookupErr := c.lookupAuthoritative(ctx, envelope.RequestID, nil, nil)
		if lookupErr == nil {
			return CommandResponse{}, &protocolError{status: http.StatusConflict, code: "request_id_conflict", reason: "request_id already exists in a product receipt outside this transport"}
		}
		var notFound *protocolError
		if !errors.As(lookupErr, &notFound) || notFound.code != "command_not_found" {
			return CommandResponse{}, lookupErr
		}
		record = requestRecord{Version: 1, Digest: digest, Envelope: envelope, CreatedAt: time.Now().UTC()}
		var mappingErr error
		ready, _, _ := c.readiness.Snapshot()
		parsed, validationErr = parseActionEnvelope(envelope)
		parsedSet = true
		if validationErr == nil && (ready || envelope.Action == "stop_service") &&
			(!isControlAction(envelope.Action) && envelope.Action != "answer_decision" || contains(c.capabilities, envelope.Action)) {
			mappingErr = c.mapIntent(ctx, envelope, parsed, &record, caller)
		}
		if _, marshalErr := marshalRequestRecord(record); marshalErr != nil {
			return CommandResponse{}, marshalErr
		}
		if err := c.registerRecord(record); err != nil {
			return CommandResponse{}, &protocolError{status: http.StatusServiceUnavailable, code: "request_arbitration_failed", reason: err.Error()}
		}
		if mappingErr != nil {
			if definitiveCommandRejection(mappingErr) {
				status, code := protocolErrorStatus(mappingErr)
				return CommandResponse{}, c.rejectRegistered(&record, envelope, status, code, mappingErr.Error())
			}
			return CommandResponse{}, mappingErr
		}
	}
	record, readErr = c.readRecord(envelope.RequestID)
	if readErr != nil {
		return CommandResponse{}, &protocolError{status: http.StatusServiceUnavailable, code: "request_record_unreadable", reason: readErr.Error()}
	}
	if !parsedSet {
		parsed, validationErr = parseActionEnvelope(envelope)
	}
	if validationErr != nil {
		status, code := protocolErrorStatus(validationErr)
		return CommandResponse{}, c.rejectRegistered(&record, envelope, status, code, validationErr.Error())
	}
	if (isControlAction(envelope.Action) || envelope.Action == "answer_decision") && !contains(c.capabilities, envelope.Action) {
		return CommandResponse{}, c.rejectRegistered(&record, envelope, http.StatusNotImplemented, "unsupported", "service does not advertise "+envelope.Action)
	}
	ready, diagnostic, status := c.readiness.Snapshot()
	if !ready && envelope.Action != "stop_service" {
		return CommandResponse{}, c.rejectRegistered(&record, envelope, http.StatusServiceUnavailable, "service_not_ready", diagnosticOrStatus(diagnostic, status))
	}
	response, err := c.dispatch(ctx, envelope, parsed, &record, caller)
	if err != nil {
		reconciled, reconcileErr := c.lookupAuthoritative(ctx, envelope.RequestID, &envelope, &record)
		if reconcileErr == nil {
			response, err = reconciled, nil
		} else {
			var identityErr *api.Error
			if errors.As(reconcileErr, &identityErr) && identityErr.Kind == api.IdentityConflict {
				err = reconcileErr
			}
		}
	}
	if err != nil {
		if definitiveCommandRejection(err) {
			status, code := protocolErrorStatus(err)
			return CommandResponse{}, c.rejectRegistered(&record, envelope, status, code, err.Error())
		}
		return CommandResponse{}, err
	}
	record.Response = outcomeOf(response)
	if err := c.writeRecord(record); err != nil {
		return CommandResponse{}, &protocolError{status: http.StatusServiceUnavailable, code: "request_receipt_persist_failed", reason: err.Error()}
	}
	return response, nil
}

func (c *commandRegistry) rejectRegistered(record *requestRecord, envelope CommandEnvelope, status int, code, reason string) error {
	rejected := CommandResponse{RequestID: envelope.RequestID, ProjectID: envelope.ProjectID, InstanceID: envelope.InstanceID,
		Action: envelope.Action, TargetID: envelope.TargetID, ExpectedVersion: envelope.ExpectedVersion,
		Original: envelopePtr(envelope), Status: "rejected", Code: code, Diagnostic: reason, SampledAt: time.Now().UTC()}
	record.Response = outcomeOf(rejected)
	if err := c.writeRecord(*record); err != nil {
		return &protocolError{status: http.StatusServiceUnavailable, code: "request_receipt_persist_failed", reason: err.Error()}
	}
	return &protocolError{status: status, code: code, reason: reason}
}

func (c *commandRegistry) Read(ctx context.Context, requestID string) (CommandResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var original *CommandEnvelope
	var storedResponse *CommandResponse
	var originalRecord *requestRecord
	if record, err := c.readRecord(requestID); err == nil {
		original = envelopePtr(record.Envelope)
		originalRecord = &record
		if record.Response != nil {
			response := record.response()
			storedResponse = &response
		}
	} else if !os.IsNotExist(err) {
		return CommandResponse{}, &protocolError{status: http.StatusServiceUnavailable, code: "request_record_unreadable", reason: err.Error()}
	}
	response, err := c.lookupAuthoritative(ctx, requestID, original, originalRecord)
	if err != nil {
		var missing *protocolError
		if storedResponse != nil && errors.As(err, &missing) && missing.code == "command_not_found" {
			saved := *storedResponse
			if saved.Status != "rejected" && saved.Status != "outcome_unknown" {
				saved.Status = "outcome_unknown"
				saved.Diagnostic = "the original transport accepted this envelope but the product authority has no current receipt; do not replay it"
			}
			return saved, nil
		}
		return CommandResponse{}, err
	}
	return response, nil
}
