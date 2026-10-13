package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	api "github.com/erranliu/SteerLine/internal/product/api"
)

type requestRecord struct {
	Version   int                `json:"version"`
	Digest    string             `json:"digest"`
	Envelope  CommandEnvelope    `json:"envelope"`
	Control   *api.ControlInput  `json:"control,omitempty"`
	Discard   *api.DiscardInput  `json:"discard,omitempty"`
	Decision  *api.DecisionInput `json:"decision,omitempty"`
	Response  *requestOutcome    `json:"response,omitempty"`
	CreatedAt time.Time          `json:"created_at"`
}

// Authority DTOs and Original are reconstructed on read, never duplicated in
// transport storage. Reserve a bounded outcome before registering any intent.
const (
	maxRequestRecordBytes  = maxCommandBytes + (32 << 10)
	maxRequestOutcomeBytes = 16 << 10
	maxOutcomeTextBytes    = 1024
)

type requestOutcome struct {
	Status     string    `json:"status"`
	Code       string    `json:"code,omitempty"`
	Diagnostic string    `json:"diagnostic,omitempty"`
	SampledAt  time.Time `json:"sampled_at"`
}

func outcomeOf(response CommandResponse) *requestOutcome {
	return &requestOutcome{Status: response.Status, Code: boundedOutcomeText(response.Code),
		Diagnostic: boundedOutcomeText(response.Diagnostic), SampledAt: response.SampledAt}
}

func boundedOutcomeText(value string) string {
	if len(value) <= maxOutcomeTextBytes {
		return value
	}
	end := maxOutcomeTextBytes - len(" [truncated]")
	for end > 0 && !utf8.RuneStart(value[end]) {
		end--
	}
	return value[:end] + " [truncated]"
}

func (record requestRecord) response() CommandResponse {
	envelope := record.Envelope
	return CommandResponse{RequestID: envelope.RequestID, ProjectID: envelope.ProjectID, InstanceID: envelope.InstanceID,
		Action: envelope.Action, TargetID: envelope.TargetID, ExpectedVersion: envelope.ExpectedVersion,
		Original: envelopePtr(envelope), Status: record.Response.Status, Code: record.Response.Code,
		Diagnostic: record.Response.Diagnostic, SampledAt: record.Response.SampledAt}
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

type reasonPayload struct {
	Reason string `json:"reason"`
}

type profilePayload struct {
	ExecutionProfileID string `json:"execution_profile_id"`
}

type abandonPayload struct {
	TaskID              string `json:"task_id"`
	ExpectedTaskVersion uint64 `json:"expected_task_version"`
	Reason              string `json:"reason"`
}

type modePayload struct {
	Mode string `json:"mode"`
}

type environmentPayload struct {
	Target api.HostActionTarget `json:"target"`
}

func (s *Server) serveCommands(w http.ResponseWriter, r *http.Request, path []string) {
	if len(path) == 0 {
		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodPost)
			return
		}
		s.submitCommand(w, r)
		return
	}
	if len(path) == 1 {
		if r.Method != http.MethodGet {
			methodNotAllowed(w, http.MethodGet)
			return
		}
		s.readCommand(w, r, path[0])
		return
	}
	writeError(w, http.StatusNotFound, "route_not_found", "command route is not part of steerline-client/v1", "")
}

func (s *Server) submitCommand(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Content-Type") == "" || !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		writeError(w, http.StatusUnsupportedMediaType, "content_type_required", "POST /commands requires application/json", "")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxCommandBytes+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "request_body_failed", err.Error(), "")
		return
	}
	if len(body) == 0 || len(body) > maxCommandBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "request_body_too_large", "command envelope must be between 1 byte and 1 MiB", "")
		return
	}
	var envelope CommandEnvelope
	if err = decodeStrict(body, &envelope); err != nil {
		writeError(w, http.StatusBadRequest, "request_invalid", err.Error(), envelope.RequestID)
		return
	}
	projectID := ProjectID(s.options.ProjectIdentity)
	if err = envelope.Validate(projectID, s.options.InstanceID); err != nil {
		writeProtocolError(w, &protocolError{status: http.StatusBadRequest, code: "request_invalid", reason: err.Error()}, envelope.RequestID)
		return
	}
	if envelope.Action == "open_native_session" {
		s.openNative(w, r, envelope)
		return
	}
	s.commandMu.Lock()
	defer s.commandMu.Unlock()
	canonical, err := json.Marshal(envelope)
	if err != nil {
		writeError(w, http.StatusBadRequest, "request_invalid", err.Error(), envelope.RequestID)
		return
	}
	digest := digestBytes(canonical)
	path := filepath.Join(s.commandsDir, envelope.RequestID+".json")
	old, readErr := readRequestRecord(path)
	if readErr == nil {
		if old.Digest != digest || !sameEnvelope(old.Envelope, envelope) {
			writeError(w, http.StatusConflict, "request_id_conflict", "request_id is already bound to a different immutable envelope", envelope.RequestID)
			return
		}
		previous, lookupErr := s.lookupAuthoritative(r.Context(), envelope.RequestID, &envelope, &old)
		if lookupErr == nil {
			writeJSON(w, responseHTTPStatus(previous.Status), previous)
			return
		}
		var missing *protocolError
		if !errors.As(lookupErr, &missing) || missing.code != "command_not_found" {
			writeProtocolError(w, lookupErr, envelope.RequestID)
			return
		}
		if old.Response != nil {
			response := old.response()
			if response.Status != "rejected" && response.Status != "outcome_unknown" {
				response.Status = "outcome_unknown"
				response.Diagnostic = "the original transport accepted this envelope but the product authority no longer has its receipt; do not replay it"
			}
			writeJSON(w, responseHTTPStatus(response.Status), response)
			return
		}
	} else if !os.IsNotExist(readErr) {
		writeError(w, http.StatusServiceUnavailable, "request_record_unreadable", readErr.Error(), envelope.RequestID)
		return
	} else {
		_, lookupErr := s.lookupAuthoritative(r.Context(), envelope.RequestID, nil, nil)
		if lookupErr == nil {
			writeError(w, http.StatusConflict, "request_id_conflict", "request_id already exists in a product receipt outside this transport", envelope.RequestID)
			return
		}
		var notFound *protocolError
		if !errors.As(lookupErr, &notFound) || notFound.code != "command_not_found" {
			writeProtocolError(w, lookupErr, envelope.RequestID)
			return
		}
		record := requestRecord{Version: 1, Digest: digest, Envelope: envelope, CreatedAt: time.Now().UTC()}
		// Bind derived content in the first unique record, before another formal
		// consumer can win the request ID between transport registration and submit.
		var mappingErr error
		if validateActionEnvelope(envelope) == nil && (s.discoveryIdentity().Ready || envelope.Action == "stop_service") &&
			(!isControlAction(envelope.Action) && envelope.Action != "answer_decision" || contains(s.options.Capabilities, envelope.Action)) {
			mappingErr = s.mapIntent(r.Context(), envelope, &record)
		}
		data, marshalErr := marshalRequestRecord(record)
		if marshalErr != nil {
			writeProtocolError(w, marshalErr, envelope.RequestID)
			return
		}
		if err = writePrivateExclusive(s.commandsDir, path, data); err != nil {
			writeError(w, http.StatusServiceUnavailable, "request_arbitration_failed", err.Error(), envelope.RequestID)
			return
		}
		if mappingErr != nil {
			if definitiveCommandRejection(mappingErr) {
				status, code := protocolErrorStatus(mappingErr)
				s.rejectRegisteredEnvelope(w, path, record, envelope, status, code, mappingErr.Error())
			} else {
				writeProtocolError(w, mappingErr, envelope.RequestID)
			}
			return
		}
	}
	record, recordErr := readRequestRecord(path)
	if recordErr != nil {
		writeError(w, http.StatusServiceUnavailable, "request_record_unreadable", recordErr.Error(), envelope.RequestID)
		return
	}
	if err = validateActionEnvelope(envelope); err != nil {
		status, code := protocolErrorStatus(err)
		s.rejectRegisteredEnvelope(w, path, record, envelope, status, code, err.Error())
		return
	}
	if (isControlAction(envelope.Action) || envelope.Action == "answer_decision") && !contains(s.options.Capabilities, envelope.Action) {
		s.rejectRegisteredEnvelope(w, path, record, envelope, http.StatusNotImplemented, "unsupported", "service does not advertise "+envelope.Action)
		return
	}
	if identity := s.discoveryIdentity(); !identity.Ready && envelope.Action != "stop_service" {
		s.rejectRegisteredEnvelope(w, path, record, envelope, http.StatusServiceUnavailable, "service_not_ready", diagnosticOrStatus(identity.Diagnostic, identity.Status))
		return
	}
	response, err := s.dispatch(r.Context(), envelope, &record, path)
	if err != nil {
		// A formal consumer may have published between unique HTTP registration and
		// the original submit. Reconcile only; the authority determines same/conflict.
		reconciled, reconcileErr := s.lookupAuthoritative(r.Context(), envelope.RequestID, &envelope, &record)
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
			s.rejectRegisteredEnvelope(w, path, record, envelope, status, code, err.Error())
			return
		}
		writeProtocolError(w, err, envelope.RequestID)
		return
	}
	record.Response = outcomeOf(response)
	if persistErr := writeRequestRecord(s.commandsDir, path, record); persistErr != nil {
		writeError(w, http.StatusServiceUnavailable, "request_receipt_persist_failed", persistErr.Error(), envelope.RequestID)
		return
	}
	writeJSON(w, responseHTTPStatus(response.Status), response)
}

func (s *Server) rejectRegisteredEnvelope(w http.ResponseWriter, path string, record requestRecord, envelope CommandEnvelope, status int, code, reason string) {
	rejected := CommandResponse{RequestID: envelope.RequestID, ProjectID: envelope.ProjectID, InstanceID: envelope.InstanceID,
		Action: envelope.Action, TargetID: envelope.TargetID, ExpectedVersion: envelope.ExpectedVersion,
		Original: envelopePtr(envelope), Status: "rejected", Code: code, Diagnostic: reason, SampledAt: time.Now().UTC()}
	record.Response = outcomeOf(rejected)
	if err := writeRequestRecord(s.commandsDir, path, record); err != nil {
		writeError(w, http.StatusServiceUnavailable, "request_receipt_persist_failed", err.Error(), envelope.RequestID)
		return
	}
	writeError(w, status, code, reason, envelope.RequestID)
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
	case "start_candidate":
		var payload profilePayload
		if err := decodeStrict(envelope.Payload, &payload); err != nil || payload.ExecutionProfileID == "" || envelope.ExpectedVersion == 0 || envelope.ExpectedGraphRevision == 0 || envelope.ExpectedControlRevision == 0 {
			return invalidCommand("start_candidate requires a profile, task version, graph revision and control revision")
		}
	case "abandon_candidate":
		var payload abandonPayload
		if err := decodeStrict(envelope.Payload, &payload); err != nil || payload.TaskID == "" || strings.TrimSpace(payload.Reason) == "" ||
			payload.ExpectedTaskVersion == 0 || envelope.ExpectedVersion == 0 || envelope.ExpectedControlRevision == 0 {
			return invalidCommand("abandon_candidate requires exact task and candidate versions, reason, and control revision")
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
	case "pause_dispatch", "resume_dispatch", "stop_service":
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

func (s *Server) mapIntent(ctx context.Context, envelope CommandEnvelope, record *requestRecord) error {
	actor := currentActor()
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
		mapped, err := s.product.NormalizeDecision(ctx, input)
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

func (s *Server) dispatch(ctx context.Context, envelope CommandEnvelope, record *requestRecord, path string) (CommandResponse, error) {
	requestedBy := currentActor()
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
			mapped, mapErr := s.product.NormalizeDecision(ctx, input)
			if mapErr != nil {
				return response, mapErr
			}
			record.Decision = &mapped
			if err := writeRequestRecord(s.commandsDir, path, *record); err != nil {
				return response, err
			}
		}
		receipt, _, err := s.product.SubmitDecision(ctx, *record.Decision)
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
			if err := writeRequestRecord(s.commandsDir, path, *record); err != nil {
				return response, err
			}
		}
		receipt, err := s.product.DiscardTask(ctx, *record.Discard)
		if err != nil {
			return response, err
		}
		response.Discard = &receipt
		command, commandErr := s.product.Command(ctx, envelope.RequestID)
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
		if err := writeRequestRecord(s.commandsDir, path, *record); err != nil {
			return response, err
		}
	}
	req, err := s.product.SubmitControlInbox(ctx, *record.Control)
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

func (s *Server) controlInput(ctx context.Context, envelope CommandEnvelope, requestedBy string) (api.ControlInput, error) {
	control := api.ControlInput{RequestID: envelope.RequestID, RequestedBy: requestedBy, InstanceID: envelope.InstanceID}
	if envelope.Action != "stop_service" || envelope.ExpectedControlRevision != 0 {
		revision := envelope.ExpectedControlRevision
		control.ExpectedRevision = &revision
	}
	switch envelope.Action {
	case "start_task", "start_candidate":
		var payload profilePayload
		if err := decodeStrict(envelope.Payload, &payload); err != nil {
			return control, invalidCommand(err.Error())
		}
		payloadValue, err := s.product.StartTaskPayload(ctx, envelope.TargetID, payload.ExecutionProfileID)
		if err != nil {
			return control, err
		}
		payloadValue.ExpectedTaskVersion = envelope.ExpectedVersion
		if envelope.Action == "start_candidate" {
			payloadValue.ExpectedGraphRevision = envelope.ExpectedGraphRevision
		}
		control.Action, control.Payload = api.ControlAction(envelope.Action), payloadValue
	case "abandon_candidate":
		var payload abandonPayload
		if err := decodeStrict(envelope.Payload, &payload); err != nil {
			return control, invalidCommand(err.Error())
		}
		control.Action = api.AbandonCandidate
		control.Payload = api.ControlPayload{TaskID: payload.TaskID, WorkflowRunID: envelope.TargetID, ExpectedTaskVersion: payload.ExpectedTaskVersion,
			ExpectedCandidateVersion: envelope.ExpectedVersion, Reason: payload.Reason}
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

func (s *Server) readCommand(w http.ResponseWriter, r *http.Request, requestID string) {
	if !requestIDPattern.MatchString(requestID) {
		writeError(w, http.StatusBadRequest, "request_id_invalid", "request_id must be 32 lowercase hexadecimal characters", requestID)
		return
	}
	var original *CommandEnvelope
	var storedResponse *CommandResponse
	var originalRecord *requestRecord
	if record, err := readRequestRecord(filepath.Join(s.commandsDir, requestID+".json")); err == nil {
		original = envelopePtr(record.Envelope)
		originalRecord = &record
		if record.Response != nil {
			response := record.response()
			storedResponse = &response
		}
	} else if !os.IsNotExist(err) {
		writeError(w, http.StatusServiceUnavailable, "request_record_unreadable", err.Error(), requestID)
		return
	}
	response, err := s.lookupAuthoritative(r.Context(), requestID, original, originalRecord)
	if err != nil {
		var missing *protocolError
		if storedResponse != nil && errors.As(err, &missing) && missing.code == "command_not_found" {
			saved := *storedResponse
			if saved.Status != "rejected" && saved.Status != "outcome_unknown" {
				saved.Status = "outcome_unknown"
				saved.Diagnostic = "the original transport accepted this envelope but the product authority has no current receipt; do not replay it"
			}
			writeJSON(w, http.StatusOK, saved)
			return
		}
		writeProtocolError(w, err, requestID)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) lookupAuthoritative(ctx context.Context, requestID string, original *CommandEnvelope, record *requestRecord) (CommandResponse, error) {
	if original != nil && original.Action == "open_native_session" && record != nil && record.Response != nil {
		return record.response(), nil
	}
	response := CommandResponse{RequestID: requestID, ProjectID: ProjectID(s.options.ProjectIdentity), InstanceID: s.options.InstanceID,
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
	command, err := s.product.Command(ctx, requestID)
	if err == nil {
		if command.ProjectIdentity != s.options.ProjectIdentity || command.CommandID != requestID {
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

func (s *Server) lookupControlAuthority(ctx context.Context, requestID string, original *CommandEnvelope, record *requestRecord) (CommandResponse, bool, error) {
	response := CommandResponse{RequestID: requestID, ProjectID: ProjectID(s.options.ProjectIdentity), InstanceID: s.options.InstanceID,
		Original: original, SampledAt: time.Now().UTC()}
	request, requestErr := s.product.Request(ctx, requestID)
	if requestErr != nil {
		if os.IsNotExist(requestErr) {
			return response, false, nil
		}
		return response, false, requestErr
	}
	if request == nil || request.RequestID != requestID || request.RepositoryIdentity != s.options.ProjectIdentity {
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
	receipt, receiptErr := s.product.Receipt(ctx, requestID)
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
	if input.RequestID != original.RequestID || input.InstanceID != original.InstanceID || string(input.Action) != original.Action || validateActionEnvelope(original) != nil {
		return false
	}
	payload := input.Payload
	switch original.Action {
	case "start_task", "start_candidate":
		var profile profilePayload
		if decodeStrict(original.Payload, &profile) != nil {
			return false
		}
		graph := uint64(0)
		if original.Action == "start_candidate" {
			graph = original.ExpectedGraphRevision
		}
		return payload.TaskID == original.TargetID && payload.ExpectedTaskVersion == original.ExpectedVersion &&
			payload.ExpectedGraphRevision == graph && payload.ExecutionProfileID == profile.ExecutionProfileID && payload.ExecutionProfileDigest != ""
	case "abandon_candidate":
		var abandon abandonPayload
		if decodeStrict(original.Payload, &abandon) != nil {
			return false
		}
		return payload.WorkflowRunID == original.TargetID && payload.ExpectedCandidateVersion == original.ExpectedVersion &&
			payload.TaskID == abandon.TaskID && payload.ExpectedTaskVersion == abandon.ExpectedTaskVersion && payload.Reason == abandon.Reason
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
		return original.TargetID == original.InstanceID && payload == (api.ControlPayload{})
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
	case "start_task", "start_candidate", "abandon_candidate", "set_dispatch_mode", "pause_dispatch", "resume_dispatch", "stop_service", "open_terminal", "recheck_environment":
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

func readRequestRecord(path string) (requestRecord, error) {
	var record requestRecord
	if err := verifyPrivateFile(path); err != nil {
		return record, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return record, err
	}
	if len(data) == 0 || len(data) > maxRequestRecordBytes {
		return record, errors.New("request arbitration record is empty or too large")
	}
	if err = decodeStrict(data, &record); err != nil {
		return record, err
	}
	if record.Version != 1 || !requestIDPattern.MatchString(record.Envelope.RequestID) || record.Envelope.RequestID+".json" != filepath.Base(path) || record.Digest == "" {
		return requestRecord{}, errors.New("request arbitration record identity is invalid")
	}
	canonical, err := json.Marshal(record.Envelope)
	if err != nil || digestBytes(canonical) != record.Digest {
		return requestRecord{}, errors.New("request arbitration record digest does not match its immutable envelope")
	}
	if _, err := marshalRequestRecord(record); err != nil {
		return requestRecord{}, err
	}
	return record, nil
}

func writeRequestRecord(dir, path string, record requestRecord) error {
	data, err := marshalRequestRecord(record)
	if err != nil {
		return err
	}
	return writePrivateAtomic(dir, path, data)
}

func marshalRequestRecord(record requestRecord) ([]byte, error) {
	base := record
	base.Response = nil
	data, err := json.Marshal(base)
	if err != nil {
		return nil, err
	}
	// Include newline and the response field framing in the reservation. The
	// complete mapped intent must pass here before any authority submit occurs.
	if len(data)+1 > maxRequestRecordBytes-maxRequestOutcomeBytes {
		return nil, &protocolError{status: http.StatusRequestEntityTooLarge, code: "request_record_too_large",
			reason: "encoded envelope and mapped intent exceed the 1064960-byte request record admission budget"}
	}
	if record.Response != nil {
		switch record.Response.Status {
		case "submitted", "accepted", "applied", "rejected", "outcome_unknown":
		default:
			return nil, errors.New("request arbitration outcome status is invalid")
		}
		outcome, err := json.Marshal(record.Response)
		if err != nil {
			return nil, err
		}
		if len(outcome)+len(`,"response":`) > maxRequestOutcomeBytes {
			return nil, errors.New("request arbitration outcome exceeds its reserved budget")
		}
		data, err = json.Marshal(record)
		if err != nil {
			return nil, err
		}
	}
	data = append(data, '\n')
	if len(data) > maxRequestRecordBytes {
		return nil, errors.New("request arbitration record exceeds its read budget")
	}
	return data, nil
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

func digestBytes(body []byte) string {
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
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
		if request.Action == "abandon_candidate" && payload.WorkflowRunID != "" {
			return payload.WorkflowRunID
		}
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
