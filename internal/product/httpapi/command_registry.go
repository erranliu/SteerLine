package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/user"
	"reflect"
	"strings"
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
	c.mu.Lock()
	defer c.mu.Unlock()
	canonical, err := json.Marshal(envelope)
	if err != nil {
		return CommandResponse{}, &protocolError{status: http.StatusBadRequest, code: "request_invalid", reason: err.Error()}
	}
	digest := digestBytes(canonical)
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
		if validateActionEnvelope(envelope) == nil && (ready || envelope.Action == "stop_service") &&
			(!isControlAction(envelope.Action) && envelope.Action != "answer_decision" || contains(c.capabilities, envelope.Action)) {
			mappingErr = c.mapIntent(ctx, envelope, &record, caller)
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
	if err := validateActionEnvelope(envelope); err != nil {
		status, code := protocolErrorStatus(err)
		return CommandResponse{}, c.rejectRegistered(&record, envelope, status, code, err.Error())
	}
	if (isControlAction(envelope.Action) || envelope.Action == "answer_decision") && !contains(c.capabilities, envelope.Action) {
		return CommandResponse{}, c.rejectRegistered(&record, envelope, http.StatusNotImplemented, "unsupported", "service does not advertise "+envelope.Action)
	}
	ready, diagnostic, status := c.readiness.Snapshot()
	if !ready && envelope.Action != "stop_service" {
		return CommandResponse{}, c.rejectRegistered(&record, envelope, http.StatusServiceUnavailable, "service_not_ready", diagnosticOrStatus(diagnostic, status))
	}
	response, err := c.dispatch(ctx, envelope, &record, caller)
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

func (c *commandRegistry) OpenNative(ctx context.Context, envelope CommandEnvelope) (CommandResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
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

type answerPayload struct {
	RunID                   string              `json:"run_id"`
	DecisionID              string              `json:"decision_id"`
	OptionID                string              `json:"option_id,omitempty"`
	Note                    string              `json:"note,omitempty"`
	Answers                 map[string][]string `json:"answers,omitempty"`
	ExpectedRunRevision     uint64              `json:"expected_run_revision"`
	SourceAttemptID         string              `json:"source_attempt_id"`
	ExpectedRecoveryVersion *uint64             `json:"expected_recovery_version"`
}

type candidateStopPayload struct {
	TaskID string `json:"task_id"`
}

type reasonPayload struct {
	Reason string `json:"reason"`
}

type profilePayload struct {
	ExecutionProfileID string `json:"execution_profile_id"`
}

type modePayload struct {
	Mode string `json:"mode"`
}

type environmentPayload struct {
	Target api.HostActionTarget `json:"target"`
}

func protocolErrorStatus(err error) (int, string) {
	var protocolErr *protocolError
	if errors.As(err, &protocolErr) {
		return protocolErr.status, protocolErr.code
	}
	var productErr *api.Error
	if errors.As(err, &productErr) {
		switch productErr.Kind {
		case api.Unsupported:
			return http.StatusNotImplemented, "unsupported"
		case api.IdentityConflict:
			return http.StatusConflict, "identity_conflict"
		}
		message := strings.ToLower(productErr.Message)
		if strings.Contains(message, "permission") || strings.Contains(message, "access is denied") || strings.Contains(message, "access denied") {
			return http.StatusForbidden, "permission_denied"
		}
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "request id conflict") || strings.Contains(message, "immutable request with different content") {
		return http.StatusConflict, "identity_conflict"
	}
	if strings.Contains(message, "stale_version") || strings.Contains(message, "stale_revision") || strings.Contains(message, "stale_instance") ||
		strings.Contains(message, "stale_recovery") || strings.Contains(message, "stale_profile") || strings.Contains(message, "stale_target") ||
		strings.Contains(message, "answer_conflict") || strings.Contains(message, "idempotency_conflict") {
		return http.StatusConflict, "stale_target"
	}
	if strings.Contains(message, "permission") || strings.Contains(message, "access is denied") || strings.Contains(message, "access denied") {
		return http.StatusForbidden, "permission_denied"
	}
	return http.StatusConflict, "command_rejected"
}

func responseHTTPStatus(status string) int {
	if status == "submitted" || status == "accepted" {
		return http.StatusAccepted
	}
	return http.StatusOK
}

func validateActionEnvelope(envelope CommandEnvelope) error {
	switch envelope.Action {
	case "answer_decision":
		var payload answerPayload
		if err := decodeStrict(envelope.Payload, &payload); err != nil {
			return invalidCommand(err.Error())
		}
		if payload.RunID == "" || payload.DecisionID == "" || payload.DecisionID != envelope.TargetID || envelope.ExpectedVersion == 0 ||
			payload.ExpectedRunRevision == 0 || payload.SourceAttemptID == "" || payload.ExpectedRecoveryVersion == nil {
			return invalidCommand("answer_decision requires exact run, decision, decision version, run revision and source attempt")
		}
	case "discard_task":
		var payload reasonPayload
		if err := decodeStrict(envelope.Payload, &payload); err != nil || strings.TrimSpace(payload.Reason) == "" || envelope.ExpectedVersion == 0 {
			return invalidCommand("discard_task requires a reason and expected task version")
		}
	case "start_task":
		var payload profilePayload
		if err := decodeStrict(envelope.Payload, &payload); err != nil || payload.ExecutionProfileID == "" || envelope.ExpectedVersion == 0 || envelope.ExpectedControlRevision == 0 {
			return invalidCommand("start_task requires a profile, expected task version and displayed control revision")
		}
	case "stop_candidate":
		var payload candidateStopPayload
		if err := decodeStrict(envelope.Payload, &payload); err != nil || payload.TaskID == "" || envelope.ExpectedVersion == 0 || envelope.ExpectedControlRevision == 0 || envelope.ExpectedGraphRevision != 0 {
			return invalidCommand("stop_candidate requires task_id, target candidate and candidate/control revisions")
		}
	case "start_candidate":
		var payload profilePayload
		if err := decodeStrict(envelope.Payload, &payload); err != nil || payload.ExecutionProfileID == "" || envelope.ExpectedVersion == 0 || envelope.ExpectedGraphRevision == 0 || envelope.ExpectedControlRevision == 0 {
			return invalidCommand("start_candidate requires a profile, task version, graph revision and control revision")
		}
	case "set_dispatch_mode":
		var payload modePayload
		if err := decodeStrict(envelope.Payload, &payload); err != nil || payload.Mode != "auto" && payload.Mode != "manual" || envelope.ExpectedVersion == 0 || envelope.ExpectedControlRevision == 0 {
			return invalidCommand("set_dispatch_mode requires mode=auto|manual and the displayed control revision")
		}
	case "open_terminal", "recheck_environment":
		var payload environmentPayload
		if err := decodeStrict(envelope.Payload, &payload); err != nil || payload.Target.RunID != envelope.TargetID || payload.Target.Version != envelope.ExpectedVersion ||
			payload.Target.ExecutionID == "" || payload.Target.AttemptID == "" || payload.Target.TurnID == "" || payload.Target.BlockerID == "" || envelope.ExpectedControlRevision == 0 {
			return invalidCommand(envelope.Action + " requires its exact run/execution/attempt/turn/blocker target")
		}
	case "pause_dispatch", "resume_dispatch":
		var empty map[string]json.RawMessage
		if err := decodeStrict(envelope.Payload, &empty); err != nil || len(empty) != 0 {
			return invalidCommand(envelope.Action + " accepts an empty JSON object")
		}
		if envelope.Action != "stop_service" && (envelope.ExpectedVersion == 0 || envelope.ExpectedControlRevision == 0) {
			return invalidCommand(envelope.Action + " requires the displayed target and control revisions")
		}
	case "create_task", "add_predecessors", "planning", "manual_acceptance", "custom_prerequisite":
		return &protocolError{status: http.StatusNotImplemented, code: "unsupported", reason: "action is outside the current local client protocol"}
	default:
		return &protocolError{status: http.StatusNotImplemented, code: "unsupported", reason: "action is unsupported by this service"}
	}
	return nil
}

func (s *commandRegistry) mapIntent(ctx context.Context, envelope CommandEnvelope, record *requestRecord, requestedBy string) error {
	actor := requestedBy
	switch envelope.Action {
	case "discard_task":
		var payload reasonPayload
		if err := decodeStrict(envelope.Payload, &payload); err != nil {
			return err
		}
		record.Discard = &api.DiscardInput{TaskID: envelope.TargetID, RequestID: envelope.RequestID, Reason: strings.TrimSpace(payload.Reason), RequestedBy: actor, ExpectedVersion: envelope.ExpectedVersion}
	case "answer_decision":
		var payload answerPayload
		if err := decodeStrict(envelope.Payload, &payload); err != nil {
			return err
		}
		input := api.DecisionInput{CommandID: envelope.RequestID, RequestedBy: actor, Answer: api.DecisionAnswer{
			RunID: payload.RunID, DecisionID: payload.DecisionID, OptionID: payload.OptionID, Note: payload.Note, Answers: payload.Answers, AnsweredBy: actor,
			ExpectedDecisionVersion: envelope.ExpectedVersion, ExpectedRunRevision: payload.ExpectedRunRevision, SourceAttemptID: payload.SourceAttemptID,
			ExpectedRecoveryVersion: *payload.ExpectedRecoveryVersion, ServiceInstanceID: envelope.InstanceID}}
		mapped, err := s.commands.NormalizeDecision(ctx, input)
		if err != nil {
			return err
		}
		record.Decision = &mapped
	default:
		control, err := s.controlInput(ctx, envelope, actor)
		if err != nil {
			return err
		}
		record.Control = &control
	}
	return nil
}

func (s *commandRegistry) dispatch(ctx context.Context, envelope CommandEnvelope, record *requestRecord, requestedBy string) (CommandResponse, error) {
	response := CommandResponse{RequestID: envelope.RequestID, ProjectID: envelope.ProjectID, InstanceID: envelope.InstanceID,
		Action: envelope.Action, TargetID: envelope.TargetID, ExpectedVersion: envelope.ExpectedVersion,
		Original: envelopePtr(envelope), SampledAt: time.Now().UTC()}
	if envelope.Action == "answer_decision" {
		var payload answerPayload
		if err := decodeStrict(envelope.Payload, &payload); err != nil {
			return response, invalidCommand(err.Error())
		}
		candidate, err := s.candidate(ctx, payload.RunID)
		if err != nil {
			return response, err
		}
		if candidate.Decision == nil || candidate.Decision.ID != payload.DecisionID || candidate.Decision.Version != envelope.ExpectedVersion ||
			candidate.ControlRevision != payload.ExpectedRunRevision || candidate.AttemptID != payload.SourceAttemptID || candidate.RecoveryVersion != *payload.ExpectedRecoveryVersion {
			return response, &protocolError{status: http.StatusConflict, code: "stale_target", reason: "decision, run revision, source attempt or recovery version changed; refresh before answering"}
		}
		input := api.DecisionInput{CommandID: envelope.RequestID, RequestedBy: requestedBy, Answer: api.DecisionAnswer{
			RunID: payload.RunID, DecisionID: payload.DecisionID, OptionID: payload.OptionID, Note: payload.Note,
			Answers: payload.Answers, AnsweredBy: requestedBy, ExpectedDecisionVersion: envelope.ExpectedVersion,
			ExpectedRunRevision: payload.ExpectedRunRevision, SourceAttemptID: payload.SourceAttemptID,
			ExpectedRecoveryVersion: *payload.ExpectedRecoveryVersion, ServiceInstanceID: envelope.InstanceID,
		}}
		if record.Decision == nil {
			mapped, mapErr := s.commands.NormalizeDecision(ctx, input)
			if mapErr != nil {
				return response, mapErr
			}
			record.Decision = &mapped
			if err := s.records.Write(*record); err != nil {
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
		var payload reasonPayload
		if err := decodeStrict(envelope.Payload, &payload); err != nil {
			return response, invalidCommand(err.Error())
		}
		if record.Discard == nil {
			record.Discard = &api.DiscardInput{TaskID: envelope.TargetID, RequestID: envelope.RequestID,
				Reason: strings.TrimSpace(payload.Reason), RequestedBy: requestedBy, ExpectedVersion: envelope.ExpectedVersion}
			if err := s.records.Write(*record); err != nil {
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
		control, err := s.controlInput(ctx, envelope, requestedBy)
		if err != nil {
			return response, err
		}
		record.Control = &control
		if err := s.records.Write(*record); err != nil {
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

func (s *commandRegistry) controlInput(ctx context.Context, envelope CommandEnvelope, requestedBy string) (api.ControlInput, error) {
	control := api.ControlInput{RequestID: envelope.RequestID, RequestedBy: requestedBy, InstanceID: envelope.InstanceID}
	if envelope.Action != "stop_service" || envelope.ExpectedControlRevision != 0 {
		revision := envelope.ExpectedControlRevision
		control.ExpectedRevision = &revision
	}
	switch envelope.Action {
	case "stop_candidate":
		var payload candidateStopPayload
		if err := decodeStrict(envelope.Payload, &payload); err != nil {
			return control, invalidCommand(err.Error())
		}
		control.Action, control.Payload = api.StopCandidate, api.ControlPayload{TaskID: payload.TaskID, RunID: envelope.TargetID, ExpectedRunRevision: envelope.ExpectedVersion}
	case "start_task", "start_candidate":
		var payload profilePayload
		if err := decodeStrict(envelope.Payload, &payload); err != nil {
			return control, invalidCommand(err.Error())
		}
		payloadValue, err := s.commands.StartTaskPayload(ctx, envelope.TargetID, payload.ExecutionProfileID)
		if err != nil {
			return control, err
		}
		payloadValue.ExpectedTaskVersion = envelope.ExpectedVersion
		if envelope.ExpectedGraphRevision != 0 {
			payloadValue.ExpectedGraphRevision = envelope.ExpectedGraphRevision
		}
		control.Action, control.Payload = api.ControlAction(envelope.Action), payloadValue
	case "open_terminal", "recheck_environment":
		var payload environmentPayload
		if err := decodeStrict(envelope.Payload, &payload); err != nil {
			return control, invalidCommand(err.Error())
		}
		control.Action = api.ControlAction(envelope.Action)
		target := payload.Target
		control.Payload = api.ControlPayload{Environment: &target}
	case "set_dispatch_mode", "pause_dispatch", "resume_dispatch":
		if envelope.TargetID != "dispatch" && envelope.TargetID != envelope.ProjectID || envelope.ExpectedVersion != envelope.ExpectedControlRevision {
			return control, invalidCommand("dispatch control target and expected version must match the displayed control revision")
		}
		control.Action = api.ControlAction(envelope.Action)
		if envelope.Action == "set_dispatch_mode" {
			var payload modePayload
			if err := decodeStrict(envelope.Payload, &payload); err != nil {
				return control, invalidCommand(err.Error())
			}
			control.Payload = api.ControlPayload{Mode: payload.Mode}
		}
	case "stop_service":
		if envelope.TargetID != envelope.InstanceID {
			return control, &protocolError{status: http.StatusConflict, code: "stale_instance", reason: "stop target is not the discovered service instance"}
		}
		control.Action = api.StopService
	default:
		return control, &protocolError{status: http.StatusNotImplemented, code: "unsupported", reason: "action is unsupported by this service"}
	}
	return control, nil
}

func (s *commandRegistry) lookupAuthoritative(ctx context.Context, requestID string, original *CommandEnvelope, record *requestRecord) (CommandResponse, error) {
	if original != nil && original.Action == "open_native_session" && record != nil && record.Response != nil {
		return record.response(), nil
	}
	response := CommandResponse{RequestID: requestID, ProjectID: ProjectID(s.projectIdentity), InstanceID: s.instanceID,
		Original: original, SampledAt: time.Now().UTC()}
	controlChecked := false
	if original != nil && isControlAction(original.Action) {
		controlResponse, found, controlErr := s.lookupControlAuthority(ctx, requestID, original, record)
		if controlErr != nil {
			return response, controlErr
		}
		controlChecked = true
		if found {
			return controlResponse, nil
		}
	}
	command, err := s.authority.Command(ctx, requestID)
	if err == nil {
		if command.ProjectIdentity != s.projectIdentity || command.CommandID != requestID {
			return response, &api.Error{Kind: api.IdentityConflict, Message: "command receipt belongs to another project"}
		}
		if original != nil && !matchesCommand(*original, record, command) {
			return response, &api.Error{Kind: api.IdentityConflict, Message: "command receipt does not match its original transport envelope"}
		}
		if original != nil {
			response.Action, response.TargetID, response.ExpectedVersion = original.Action, original.TargetID, original.ExpectedVersion
			response.InstanceID = original.InstanceID
		} else {
			response.Action, response.TargetID, response.ExpectedVersion = command.Kind, command.TargetID, command.ExpectedVersion
		}
		response.Command, response.Discard = &command, command.Discard
		response.Status = normalizedCommandStatus(command.Kind, command.Status)
		response.Code = command.ResultCode
		if command.ResultError != "" {
			response.Diagnostic = command.ResultError
		}
		return response, nil
	}
	if !isUnknownRecord(err) {
		return response, err
	}
	if !controlChecked {
		controlResponse, found, controlErr := s.lookupControlAuthority(ctx, requestID, original, record)
		if controlErr != nil {
			return response, controlErr
		}
		if found {
			return controlResponse, nil
		}
	}
	return response, &protocolError{status: http.StatusNotFound, code: "command_not_found", reason: "the project database and control request authority contain no record for this request_id"}
}

func (s *commandRegistry) lookupControlAuthority(ctx context.Context, requestID string, original *CommandEnvelope, record *requestRecord) (CommandResponse, bool, error) {
	response := CommandResponse{RequestID: requestID, ProjectID: ProjectID(s.projectIdentity), InstanceID: s.instanceID,
		Original: original, SampledAt: time.Now().UTC()}
	request, requestErr := s.authority.Request(ctx, requestID)
	if requestErr != nil {
		if os.IsNotExist(requestErr) {
			return response, false, nil
		}
		return response, false, requestErr
	}
	if request == nil || request.RequestID != requestID || request.RepositoryIdentity != s.projectIdentity {
		return response, false, &api.Error{Kind: api.IdentityConflict, Message: "control request identity does not match this project"}
	}
	if original != nil && !matchesControl(*original, record, *request) {
		return response, false, &api.Error{Kind: api.IdentityConflict, Message: "control receipt does not match its original transport envelope"}
	}
	response.Action, response.TargetID = request.Action, controlTarget(*request)
	response.InstanceID = request.InstanceID
	if original != nil {
		response.TargetID, response.ExpectedVersion = original.TargetID, original.ExpectedVersion
	}
	response.ControlRequest = request
	response.Status = "submitted"
	receipt, receiptErr := s.authority.Receipt(ctx, requestID)
	if receiptErr == nil {
		response.ControlReceipt = receipt
		response.Status = normalizedControlStatus(receipt.Status)
		response.Code = receipt.Code
		response.Diagnostic = receipt.Message
	} else if !os.IsNotExist(receiptErr) {
		return response, false, receiptErr
	}
	return response, true, nil
}

// A receipt can only stand in for a registered HTTP envelope when the exact
// original mapped intent agrees. Missing mapped intent is not evidence of a match.

func matchesControl(original CommandEnvelope, record *requestRecord, request api.ControlRequest) bool {
	if original.ProjectID != ProjectID(request.RepositoryIdentity) || original.RequestID != request.RequestID ||
		original.InstanceID != request.InstanceID || original.Action != request.Action ||
		original.ExpectedControlRevision != request.ExpectedControlRevision || record == nil || record.Control == nil {
		return false
	}
	input := record.Control
	if !mappedControlMatchesEnvelope(original, *input) || request.ProtocolVersion != 1 || request.RunID != "" || request.DecisionID != "" || request.ExpectedRunRevision != 0 {
		return false
	}
	revision := uint64(0)
	if input.ExpectedRevision != nil {
		revision = *input.ExpectedRevision
	}
	if revision != request.ExpectedControlRevision || input.RequestedBy != request.RequestedBy {
		return false
	}
	var payload api.ControlPayload
	if len(request.Payload) != 0 && decodeStrict(request.Payload, &payload) != nil {
		return false
	}
	return reflect.DeepEqual(input.Payload, payload)
}

func mappedControlMatchesEnvelope(original CommandEnvelope, input api.ControlInput) bool {
	if input.RequestID != original.RequestID || input.InstanceID != original.InstanceID || string(input.Action) != original.Action || (original.Action != "stop_service" && validateActionEnvelope(original) != nil) {
		return false
	}
	payload := input.Payload
	switch original.Action {
	case "stop_candidate":
		var stop candidateStopPayload
		return decodeStrict(original.Payload, &stop) == nil && payload == (api.ControlPayload{TaskID: stop.TaskID, RunID: original.TargetID, ExpectedRunRevision: original.ExpectedVersion})
	case "start_task", "start_candidate":
		var profile profilePayload
		if decodeStrict(original.Payload, &profile) != nil {
			return false
		}
		// start_task may omit a displayed graph revision. Its command port
		// freezes the current graph in the immutable mapped intent instead.
		// matchesControl still requires the published payload to equal that intent.
		graphMatches := original.Action == "start_task" && original.ExpectedGraphRevision == 0 || payload.ExpectedGraphRevision == original.ExpectedGraphRevision
		return payload.TaskID == original.TargetID && payload.ExpectedTaskVersion == original.ExpectedVersion &&
			graphMatches && payload.ExecutionProfileID == profile.ExecutionProfileID && payload.ExecutionProfileDigest != ""
	case "open_terminal", "recheck_environment":
		var environment environmentPayload
		return decodeStrict(original.Payload, &environment) == nil && reflect.DeepEqual(payload.Environment, &environment.Target)
	case "set_dispatch_mode", "pause_dispatch", "resume_dispatch":
		if original.TargetID != "dispatch" && original.TargetID != original.ProjectID || original.ExpectedVersion != original.ExpectedControlRevision {
			return false
		}
		if original.Action == "set_dispatch_mode" {
			var mode modePayload
			return decodeStrict(original.Payload, &mode) == nil && payload.Mode == mode.Mode
		}
		return payload == (api.ControlPayload{})
	case "stop_service":
		var empty map[string]json.RawMessage
		return decodeStrict(original.Payload, &empty) == nil && len(empty) == 0 && original.TargetID == original.InstanceID && payload == (api.ControlPayload{})
	}
	return false
}

func matchesCommand(original CommandEnvelope, record *requestRecord, command api.CommandReceipt) bool {
	if original.ProjectID != ProjectID(command.ProjectIdentity) || original.RequestID != command.CommandID ||
		original.Action != command.Kind || original.TargetID != command.TargetID || original.ExpectedVersion != command.ExpectedVersion {
		return false
	}
	switch original.Action {
	case "answer_decision":
		if record == nil || record.Decision == nil || command.DecisionAnswer == nil {
			return false
		}
		wanted, actual := record.Decision.Answer, *command.DecisionAnswer
		if wanted.ServiceInstanceID != original.InstanceID || record.Decision.CommandID != original.RequestID ||
			wanted.DecisionID != original.TargetID || wanted.ExpectedDecisionVersion != original.ExpectedVersion ||
			record.Decision.RequestedBy != command.RequestedBy {
			return false
		}
		// Submission time is authority metadata, not client content.
		wanted.AnsweredAt, actual.AnsweredAt = time.Time{}, time.Time{}
		return reflect.DeepEqual(wanted, actual)
	case "discard_task":
		var payload reasonPayload
		if decodeStrict(original.Payload, &payload) != nil || command.DiscardRequest == nil || record == nil || record.Discard == nil {
			return false
		}
		request := command.DiscardRequest
		return reflect.DeepEqual(*record.Discard, *request) && request.RequestID == original.RequestID && request.TaskID == original.TargetID &&
			request.ExpectedVersion == original.ExpectedVersion && request.RequestedBy == command.RequestedBy &&
			request.Reason == strings.TrimSpace(payload.Reason)
	default:
		// Control commands require the formal ControlRequest's instance and revision;
		// the domain SQLite command alone does not contain those transport identities.
		return false
	}
}

func isControlAction(action string) bool {
	switch action {
	case "start_task", "start_candidate", "stop_candidate", "set_dispatch_mode", "pause_dispatch", "resume_dispatch", "stop_service", "open_terminal", "recheck_environment":
		return true
	default:
		return false
	}
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func definitiveCommandRejection(err error) bool {
	var protocolErr *protocolError
	if errors.As(err, &protocolErr) {
		return protocolErr.status == http.StatusBadRequest || protocolErr.status == http.StatusConflict || protocolErr.status == http.StatusUnprocessableEntity
	}
	var productErr *api.Error
	if errors.As(err, &productErr) {
		if productErr.Kind == api.IdentityConflict || productErr.Kind == api.Unsupported {
			return true
		}
		message := strings.ToLower(productErr.Message)
		return strings.Contains(message, "permission") || strings.Contains(message, "access is denied") || strings.Contains(message, "access denied")
	}
	message := strings.ToLower(err.Error())
	for _, marker := range []string{"stale_version", "stale_revision", "stale_instance", "stale_recovery", "stale_profile", "stale_target", "answer_conflict", "idempotency_conflict", "task_not_ready", "slot_busy", "dispatch_paused", "wrong_mode", "already_managed", "unsupported", "permission", "access is denied", "access denied"} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func sameEnvelope(a, b CommandEnvelope) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return bytes.Equal(x, y)
}

func invalidCommand(reason string) error {
	return &protocolError{status: http.StatusUnprocessableEntity, code: "invalid_command", reason: reason}
}

func envelopePtr(value CommandEnvelope) *CommandEnvelope { return &value }

func isUnknownRecord(err error) bool {
	var productErr *api.Error
	return errors.As(err, &productErr) && productErr.Kind == api.UnknownRecord
}

func normalizedCommandStatus(action, status string) string {
	switch strings.ToLower(status) {
	case "pending", "submitted":
		return "submitted"
	case "accepted":
		if action == "discard_task" {
			return "applied"
		}
		return "accepted"
	case "confirmed", "applied", "discarded":
		return "applied"
	case "rejected", "failed":
		return "rejected"
	case "indeterminate", "pending_verification", "outcome_unknown":
		return "outcome_unknown"
	default:
		return "outcome_unknown"
	}
}

func discardStatus(status string) string {
	switch status {
	case "discarded":
		return "applied"
	case "rejected":
		return "rejected"
	case "pending_verification":
		return "outcome_unknown"
	default:
		return "outcome_unknown"
	}
}

func normalizedControlStatus(status string) string {
	switch status {
	case "applying":
		return "accepted"
	case "applied":
		return "applied"
	case "rejected", "failed":
		return "rejected"
	case "indeterminate":
		return "outcome_unknown"
	default:
		return "submitted"
	}
}

func controlTarget(request api.ControlRequest) string {
	var payload api.ControlPayload
	if len(request.Payload) != 0 && json.Unmarshal(request.Payload, &payload) == nil {
		if payload.TaskID != "" {
			return payload.TaskID
		}
		if payload.Environment != nil {
			return payload.Environment.RunID
		}
	}
	if request.Action == "stop_service" {
		return request.InstanceID
	}
	return "dispatch"
}

func currentActor() string {
	if current, err := user.Current(); err == nil && strings.TrimSpace(current.Username) != "" {
		return strings.TrimSpace(current.Username)
	}
	for _, key := range []string{"USERNAME", "USER"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return "unknown"
}
