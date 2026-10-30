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
	"time"

	api "github.com/erranliu/SteerLine/internal/product/api"
)

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
