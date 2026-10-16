package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	api "github.com/erranliu/SteerLine/internal/product/api"
)

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

type actionPayload struct {
	answer      answerPayload
	reason      reasonPayload
	profile     profilePayload
	stop        candidateStopPayload
	mode        modePayload
	environment environmentPayload
}

func validateActionEnvelope(envelope CommandEnvelope) error {
	_, err := parseActionEnvelope(envelope)
	return err
}

func parseActionEnvelope(envelope CommandEnvelope) (parsed actionPayload, err error) {
	if isEnvironmentCheckAction(envelope.Action) {
		return parsed, validateEnvironmentEnvelope(envelope)
	}
	switch envelope.Action {
	case "answer_decision":
		payload := &parsed.answer
		if err := decodeStrict(envelope.Payload, payload); err != nil {
			return parsed, invalidCommand(err.Error())
		}
		if payload.RunID == "" || payload.DecisionID == "" || payload.DecisionID != envelope.TargetID || envelope.ExpectedVersion == 0 ||
			payload.ExpectedRunRevision == 0 || payload.SourceAttemptID == "" || payload.ExpectedRecoveryVersion == nil {
			return parsed, invalidCommand("answer_decision requires exact run, decision, decision version, run revision and source attempt")
		}
	case "discard_task":
		payload := &parsed.reason
		if err := decodeStrict(envelope.Payload, payload); err != nil || strings.TrimSpace(payload.Reason) == "" || envelope.ExpectedVersion == 0 {
			return parsed, invalidCommand("discard_task requires a reason and expected task version")
		}
	case "start_task":
		payload := &parsed.profile
		if err := decodeStrict(envelope.Payload, payload); err != nil || payload.ExecutionProfileID == "" || envelope.ExpectedVersion == 0 || envelope.ExpectedControlRevision == 0 {
			return parsed, invalidCommand("start_task requires a profile, expected task version and displayed control revision")
		}
	case "stop_candidate":
		payload := &parsed.stop
		if err := decodeStrict(envelope.Payload, payload); err != nil || payload.TaskID == "" || envelope.ExpectedVersion == 0 || envelope.ExpectedControlRevision == 0 || envelope.ExpectedGraphRevision != 0 {
			return parsed, invalidCommand("stop_candidate requires task_id, target candidate and candidate/control revisions")
		}
	case "start_candidate":
		payload := &parsed.profile
		if err := decodeStrict(envelope.Payload, payload); err != nil || payload.ExecutionProfileID == "" || envelope.ExpectedVersion == 0 || envelope.ExpectedGraphRevision == 0 || envelope.ExpectedControlRevision == 0 {
			return parsed, invalidCommand("start_candidate requires a profile, task version, graph revision and control revision")
		}
	case "set_dispatch_mode":
		payload := &parsed.mode
		if err := decodeStrict(envelope.Payload, payload); err != nil || payload.Mode != "auto" && payload.Mode != "manual" || envelope.ExpectedVersion == 0 || envelope.ExpectedControlRevision == 0 {
			return parsed, invalidCommand("set_dispatch_mode requires mode=auto|manual and the displayed control revision")
		}
	case "open_terminal", "recheck_environment":
		payload := &parsed.environment
		if err := decodeStrict(envelope.Payload, payload); err != nil || payload.Target.RunID != envelope.TargetID || payload.Target.Version != envelope.ExpectedVersion ||
			payload.Target.ExecutionID == "" || payload.Target.AttemptID == "" || payload.Target.TurnID == "" || payload.Target.BlockerID == "" || envelope.ExpectedControlRevision == 0 {
			return parsed, invalidCommand(envelope.Action + " requires its exact run/execution/attempt/turn/blocker target")
		}
	case "pause_dispatch", "resume_dispatch":
		var empty map[string]json.RawMessage
		if err := decodeStrict(envelope.Payload, &empty); err != nil || len(empty) != 0 {
			return parsed, invalidCommand(envelope.Action + " accepts an empty JSON object")
		}
		if envelope.Action != "stop_service" && (envelope.ExpectedVersion == 0 || envelope.ExpectedControlRevision == 0) {
			return parsed, invalidCommand(envelope.Action + " requires the displayed target and control revisions")
		}
	case "create_task", "add_predecessors", "planning", "manual_acceptance", "custom_prerequisite":
		return parsed, &protocolError{status: http.StatusNotImplemented, code: "unsupported", reason: "action is outside the current local client protocol"}
	default:
		return parsed, &protocolError{status: http.StatusNotImplemented, code: "unsupported", reason: "action is unsupported by this service"}
	}
	return parsed, nil
}

func (s *commandRegistry) mapIntent(ctx context.Context, envelope CommandEnvelope, parsed actionPayload, record *requestRecord, requestedBy string) error {
	actor := requestedBy
	switch envelope.Action {
	case "discard_task":
		payload := parsed.reason
		record.Discard = &api.DiscardInput{TaskID: envelope.TargetID, RequestID: envelope.RequestID, Reason: strings.TrimSpace(payload.Reason), RequestedBy: actor, ExpectedVersion: envelope.ExpectedVersion}
	case "answer_decision":
		payload := parsed.answer
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
		control, err := s.mappedControlInput(ctx, envelope, parsed, actor)
		if err != nil {
			return err
		}
		record.Control = &control
	}
	return nil
}

func (s *commandRegistry) mappedControlInput(ctx context.Context, envelope CommandEnvelope, parsed actionPayload, requestedBy string) (api.ControlInput, error) {
	control := api.ControlInput{RequestID: envelope.RequestID, RequestedBy: requestedBy, InstanceID: envelope.InstanceID}
	if envelope.Action != "stop_service" || envelope.ExpectedControlRevision != 0 {
		revision := envelope.ExpectedControlRevision
		control.ExpectedRevision = &revision
	}
	switch envelope.Action {
	case "stop_candidate":
		payload := parsed.stop
		control.Action, control.Payload = api.StopCandidate, api.ControlPayload{TaskID: payload.TaskID, RunID: envelope.TargetID, ExpectedRunRevision: envelope.ExpectedVersion}
	case "start_task", "start_candidate":
		payload := parsed.profile
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
		payload := parsed.environment
		control.Action = api.ControlAction(envelope.Action)
		target := payload.Target
		control.Payload = api.ControlPayload{Environment: &target}
	case "set_dispatch_mode", "pause_dispatch", "resume_dispatch":
		if envelope.TargetID != "dispatch" && envelope.TargetID != envelope.ProjectID || envelope.ExpectedVersion != envelope.ExpectedControlRevision {
			return control, invalidCommand("dispatch control target and expected version must match the displayed control revision")
		}
		control.Action = api.ControlAction(envelope.Action)
		if envelope.Action == "set_dispatch_mode" {
			payload := parsed.mode
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

// persistIntent completes a registered request without changing an existing intent.
func (s *commandRegistry) persistIntent(ctx context.Context, envelope CommandEnvelope, parsed actionPayload, record *requestRecord, requestedBy string) error {
	if err := s.mapIntent(ctx, envelope, parsed, record, requestedBy); err != nil {
		return err
	}
	return s.writeRecord(*record)
}
