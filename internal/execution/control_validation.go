package execution

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	productapi "github.com/erranliu/SteerLine/internal/product/api"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func ValidateControlRequest(req productapi.ControlRequest, instanceID, repositoryIdentity string, revision uint64, now time.Time) (string, error) {
	if req.ProtocolVersion != ControlProtocolVersion {
		return "invalid_request", errors.New("unsupported control protocol version")
	}
	if !controlIDPattern.MatchString(req.RequestID) {
		return "invalid_request", errors.New("invalid request ID")
	}
	if req.InstanceID == "" || req.InstanceID != instanceID {
		return "stale_instance", errors.New("request targets a different service instance")
	}
	if canonical(req.RepositoryIdentity) != canonical(repositoryIdentity) {
		return "invalid_request", errors.New("repository identity does not match service")
	}
	if req.Action == "" || !ValidControlAction(req.Action) {
		return "invalid_request", errors.New("unsupported control action")
	}
	if strings.TrimSpace(req.RequestedBy) == "" {
		return "invalid_request", errors.New("requested_by is required")
	}
	if req.Action != "stop_service" && req.ExpectedControlRevision != revision {
		return "stale_revision", errors.New("control state changed; refresh before retrying")
	}
	_, createdOffset := req.CreatedAt.Zone()
	_, expiryOffset := req.ExpiresAt.Zone()
	if req.CreatedAt.IsZero() || req.ExpiresAt.IsZero() || req.CreatedAt.After(now.Add(30*time.Second)) || createdOffset != 0 || expiryOffset != 0 || !req.ExpiresAt.After(req.CreatedAt) || !req.ExpiresAt.After(now) {
		return "expired", errors.New("control request expired before service acceptance")
	}
	if req.ExpiresAt.Sub(req.CreatedAt) > 2*time.Minute+time.Second {
		return "invalid_request", errors.New("control request validity exceeds two minutes")
	}
	return "", nil
}

func ValidControlAction(action string) bool {
	switch action {
	case "start_task", "start_candidate", "abandon_candidate", "set_dispatch_mode", "pause_dispatch", "resume_dispatch", "stop_service", "open_terminal", "recheck_environment":
		return true
	}
	return false
}

func TerminalControlStatus(status string) bool {
	switch status {
	case "applied", "rejected", "failed", "indeterminate":
		return true
	}
	return false
}

func DecodeControlPayload(req productapi.ControlRequest) (productapi.ControlPayload, error) {
	var p productapi.ControlPayload
	var fields map[string]json.RawMessage
	if len(req.Payload) != 0 {
		raw := strings.TrimSpace(string(req.Payload))
		if raw == "" || raw[0] != '{' {
			return p, errors.New("action payload must be a JSON object")
		}
		if err := decode(req.Payload, &p); err != nil {
			return p, fmt.Errorf("invalid action payload: %w", err)
		}
		if err := json.Unmarshal(req.Payload, &fields); err != nil {
			return p, err
		}
	}
	if p.Environment != nil && req.Action != "open_terminal" && req.Action != "recheck_environment" {
		return p, errors.New("environment target is only valid for environment actions")
	}
	var allowed map[string]bool
	switch req.Action {
	case "start_task":
		allowed = map[string]bool{"task_id": true, "expected_task_version": true, "execution_profile_id": true, "execution_profile_digest": true}
	case "start_candidate":
		allowed = map[string]bool{"task_id": true, "expected_task_version": true, "expected_graph_revision": true, "execution_profile_id": true, "execution_profile_digest": true}
	case "abandon_candidate":
		allowed = map[string]bool{"task_id": true, "workflow_run_id": true, "expected_task_version": true, "expected_candidate_version": true, "reason": true}
	}
	if allowed != nil {
		for key := range fields {
			if !allowed[key] {
				return p, errors.New("payload field is not supported by this action: " + key)
			}
		}
	}
	switch req.Action {
	case "start_candidate":
		if taskmodel.ValidateID(p.TaskID) != nil || p.ExpectedTaskVersion == 0 || p.ExpectedGraphRevision == 0 || len(fields) != 3 && len(fields) != 5 || p.ExecutionProfileID == "" && p.ExecutionProfileDigest != "" || p.ExecutionProfileID != "" && !evidenceDigest(p.ExecutionProfileDigest) {
			return p, errors.New("start_candidate requires task_id, current task/graph versions and a complete profile selection")
		}
	case "abandon_candidate":
		if taskmodel.ValidateID(p.TaskID) != nil || !controlIDPattern.MatchString(p.WorkflowRunID) || p.ExpectedTaskVersion == 0 || p.ExpectedCandidateVersion == 0 || strings.TrimSpace(p.Reason) == "" || len(p.Reason) > 4000 || len(fields) != 5 {
			return p, errors.New("abandon_candidate requires exact task/run versions and a reason")
		}
	case "open_terminal", "recheck_environment":
		target := p.Environment
		if len(fields) != 1 || target == nil || !controlIDPattern.MatchString(target.RunID) || !controlIDPattern.MatchString(target.ExecutionID) || !controlIDPattern.MatchString(target.AttemptID) || !controlIDPattern.MatchString(target.TurnID) || !controlIDPattern.MatchString(target.BlockerID) || target.Version == 0 {
			return p, errors.New("environment action requires its exact run, execution, attempt, turn and blocker version")
		}
	case "start_task":
		if strings.TrimSpace(p.TaskID) == "" || len(fields) != 1 && len(fields) != 2 && len(fields) != 3 && len(fields) != 4 || p.ExecutionProfileID == "" && p.ExecutionProfileDigest != "" || p.ExecutionProfileID != "" && !evidenceDigest(p.ExecutionProfileDigest) {
			return p, errors.New("start_task requires task_id and a complete configured execution profile selection")
		}
	case "set_dispatch_mode":
		if (p.Mode != "manual" && p.Mode != "auto") || len(fields) != 1 {
			return p, errors.New("set_dispatch_mode requires mode=manual|auto")
		}
	default:
		if len(fields) != 0 {
			return p, errors.New("action does not accept payload fields")
		}
	}
	return p, nil
}
