package steerline

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/clientcontracts"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
)

const ControlProtocolVersion = 1
const maxControlRequestBytes = 3 << 20

var controlIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

type ControlRequest = clientcontracts.ControlRequest
type ControlPayload = clientcontracts.ControlPayload

func controlRequestPath(runDir, id string) string {
	return filepath.Join(runDir, "control", "requests", id, "request.json")
}
func controlReceiptPath(runDir, id string) string {
	return filepath.Join(runDir, "control", "receipts", id+".json")
}
func readBoundedFile(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, errors.New("file exceeds size limit")
	}
	return b, nil
}
func requestDigest(req ControlRequest) string {
	b, _ := json.Marshal(req)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func publishControlRequest(runDir string, req ControlRequest) error {
	if !controlIDPattern.MatchString(req.RequestID) {
		return errors.New("request ID must be 32 lowercase hexadecimal characters")
	}
	b, err := json.MarshalIndent(req, "", "  ")
	if err != nil {
		return err
	}
	if len(b) > maxControlRequestBytes {
		return errors.New("control request exceeds 3 MiB")
	}
	path := controlRequestPath(runDir, req.RequestID)
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	if existing, e := readBoundedFile(path, maxControlRequestBytes); e == nil {
		var old ControlRequest
		if decode(existing, &old) != nil || requestDigest(old) != requestDigest(req) {
			return errors.New("request ID conflict: an immutable request with different content already exists")
		}
		return nil
	} else if !os.IsNotExist(e) {
		return e
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".request-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err = tmp.Write(b); err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	// A hard link is an atomic create-if-absent publication. Unlike AtomicWrite,
	// it cannot replace a prior immutable request when two processes race.
	if err = os.Link(tmpName, path); err != nil {
		if !os.IsExist(err) {
			return err
		}
		existing, readErr := readBoundedFile(path, maxControlRequestBytes)
		if readErr != nil {
			return readErr
		}
		var old ControlRequest
		if decode(existing, &old) != nil || requestDigest(old) != requestDigest(req) {
			return errors.New("request ID conflict: an immutable request with different content already exists")
		}
	}
	return nil
}

func readControlRequest(path string) (ControlRequest, error) {
	var req ControlRequest
	b, err := readBoundedFile(path, maxControlRequestBytes)
	if err != nil {
		return req, err
	}
	if len(b) == 0 {
		return req, errors.New("invalid control request size")
	}
	if err = decode(b, &req); err != nil {
		return req, err
	}
	return req, nil
}

func validateControlRequest(req ControlRequest, instanceID, repositoryIdentity string, revision uint64, now time.Time) (string, error) {
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
	if req.Action == "" || !validControlAction(req.Action) {
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
func validControlAction(action string) bool {
	switch action {
	case "start_task", "start_candidate", "abandon_candidate", "set_dispatch_mode", "pause_dispatch", "resume_dispatch", "stop_service", "open_terminal", "recheck_environment":
		return true
	}
	return false
}

func terminalControlStatus(status string) bool {
	switch status {
	case "applied", "rejected", "failed", "indeterminate":
		return true
	}
	return false
}

func decodeControlPayload(req ControlRequest) (ControlPayload, error) {
	var p ControlPayload
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
		allowed = map[string]bool{"task_id": true, "execution_profile_id": true, "execution_profile_digest": true}
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
		if ValidateNativeTaskID(p.TaskID) != nil || p.ExpectedTaskVersion == 0 || p.ExpectedGraphRevision == 0 || len(fields) != 3 && len(fields) != 5 || p.ExecutionProfileID == "" && p.ExecutionProfileDigest != "" || p.ExecutionProfileID != "" && !evidenceDigest(p.ExecutionProfileDigest) {
			return p, errors.New("start_candidate requires task_id, current task/graph versions and a complete profile selection")
		}
	case "abandon_candidate":
		if ValidateNativeTaskID(p.TaskID) != nil || !controlIDPattern.MatchString(p.WorkflowRunID) || p.ExpectedTaskVersion == 0 || p.ExpectedCandidateVersion == 0 || strings.TrimSpace(p.Reason) == "" || len(p.Reason) > 4000 || len(fields) != 5 {
			return p, errors.New("abandon_candidate requires exact task/run versions and a reason")
		}
	case "open_terminal", "recheck_environment":
		target := p.Environment
		if len(fields) != 1 || target == nil || !controlIDPattern.MatchString(target.RunID) || !controlIDPattern.MatchString(target.ExecutionID) || !controlIDPattern.MatchString(target.AttemptID) || !controlIDPattern.MatchString(target.TurnID) || !controlIDPattern.MatchString(target.BlockerID) || target.Version == 0 {
			return p, errors.New("environment action requires its exact run, execution, attempt, turn and blocker version")
		}
	case "start_task":
		if strings.TrimSpace(p.TaskID) == "" || len(fields) != 1 && len(fields) != 3 || p.ExecutionProfileID == "" && p.ExecutionProfileDigest != "" || p.ExecutionProfileID != "" && !evidenceDigest(p.ExecutionProfileDigest) {
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

func DefaultRequestedBy() string {
	return AnswererName()
}

type ControlReceipt = contracts.ControlReceipt
