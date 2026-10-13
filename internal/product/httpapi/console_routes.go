package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"time"

	api "github.com/erranliu/SteerLine/internal/product/api"
)

// Empty attempt selects the run's full history. An explicit historical attempt
// must be present in the formal conversation projection for this run/execution.
func (s *Server) conversationIdentity(ctx context.Context, candidate *api.CandidateSummary, execution, attempt string) (string, string, error) {
	if candidate == nil || candidate.ExecutionID == "" || execution != "" && execution != candidate.ExecutionID {
		return "", "", &protocolError{status: http.StatusConflict, code: "target_mismatch", reason: "conversation belongs to another execution"}
	}
	if attempt != "" && attempt != candidate.AttemptID {
		page, err := s.product.ConversationPage(ctx, api.ConversationFilter{WorkflowID: candidate.RunID, ExecutionID: candidate.ExecutionID, AttemptID: attempt}, "", 1)
		if err != nil {
			return "", "", err
		}
		if len(page.Events) == 0 {
			return "", "", &protocolError{status: http.StatusConflict, code: "target_mismatch", reason: "attempt has no conversation in the original run"}
		}
	}
	return candidate.ExecutionID, attempt, nil
}

func (s *Server) consoleQuery(w http.ResponseWriter, r *http.Request, path []string) bool {
	if len(path) != 3 || path[0] != "runs" || (path[2] != "recovery" && path[2] != "decision-eligibility") {
		return false
	}
	candidate, err := s.candidate(r.Context(), path[1])
	if err != nil {
		writeResult(w, nil, err)
		return true
	}
	if r.URL.Query().Get("execution_id") != candidate.ExecutionID || r.URL.Query().Get("attempt_id") != candidate.AttemptID || candidate.ExecutionID == "" || candidate.AttemptID == "" {
		writeError(w, http.StatusConflict, "stale_target", "original execution/attempt binding changed", "")
		return true
	}
	if path[2] == "recovery" {
		result, err := s.product.InspectRecovery(r.Context(), path[1])
		writeResult(w, result, err)
	} else {
		err := s.product.DecisionEligibility(r.Context(), candidate)
		writeResult(w, struct{}{}, err)
	}
	return true
}

// Locating a native viewer is a host side effect, not a Task/Execution
// transition. The immutable HTTP operation is marked unknown before invoking it;
// loss of the result never reopens the window on replay.
func (s *Server) openNative(w http.ResponseWriter, r *http.Request, envelope CommandEnvelope) {
	s.commandMu.Lock()
	defer s.commandMu.Unlock()
	canonical, _ := json.Marshal(envelope)
	digest := digestBytes(canonical)
	path := filepath.Join(s.commandsDir, envelope.RequestID+".json")
	if old, err := readRequestRecord(path); err == nil {
		if old.Digest != digest || !sameEnvelope(old.Envelope, envelope) {
			writeError(w, http.StatusConflict, "request_id_conflict", "original envelope differs", envelope.RequestID)
			return
		}
		if old.Response == nil {
			writeError(w, http.StatusServiceUnavailable, "outcome_unknown", "native viewer result unverified", envelope.RequestID)
			return
		}
		writeJSON(w, responseHTTPStatus(old.Response.Status), old.response())
		return
	} else if !os.IsNotExist(err) {
		writeError(w, http.StatusServiceUnavailable, "request_record_failed", err.Error(), envelope.RequestID)
		return
	}
	// IDs already owned by a formal command/control outside HTTP cannot be
	// repurposed for a viewer side effect.
	_, authorityErr := s.lookupAuthoritative(r.Context(), envelope.RequestID, nil, nil)
	var missing *protocolError
	if authorityErr == nil {
		writeError(w, http.StatusConflict, "request_id_conflict", "request ID already belongs to a product command", envelope.RequestID)
		return
	}
	if !errors.As(authorityErr, &missing) || missing.code != "command_not_found" {
		writeProtocolError(w, authorityErr, envelope.RequestID)
		return
	}
	response := CommandResponse{RequestID: envelope.RequestID, ProjectID: envelope.ProjectID, InstanceID: envelope.InstanceID, Action: envelope.Action, TargetID: envelope.TargetID, Original: envelopePtr(envelope), Status: "rejected", SampledAt: time.Now().UTC()}
	var target api.NativeSessionTarget
	err := decodeStrict(envelope.Payload, &target)
	if err == nil && (envelope.ExpectedVersion != 0 || target.RunID != envelope.TargetID || target.ExecutionID == "" || target.AttemptID == "" || target.SessionID == "" || target.SessionGeneration == "") {
		err = errors.New("exact native session target required")
	}
	if err == nil && !s.identityView(r.Context()).Ready {
		err = errors.New("service is not ready")
	}
	if err == nil {
		var candidate *api.CandidateSummary
		candidate, err = s.candidate(r.Context(), target.RunID)
		if err == nil && (candidate.ExecutionID != target.ExecutionID || candidate.AttemptID != target.AttemptID || candidate.SessionID != target.SessionID || candidate.SessionGeneration != target.SessionGeneration) {
			err = errors.New("stale native session target")
		}
	}
	record := requestRecord{Version: 1, Digest: digest, Envelope: envelope, CreatedAt: time.Now().UTC()}
	if err != nil {
		response.Code = "stale_target"
		response.Diagnostic = err.Error()
		record.Response = outcomeOf(response)
		if err = registerNativeRecord(s.commandsDir, path, record); err != nil {
			writeError(w, http.StatusServiceUnavailable, "request_persist_failed", err.Error(), envelope.RequestID)
			return
		}
		writeJSON(w, responseHTTPStatus(response.Status), response)
		return
	}
	response.Status = "outcome_unknown"
	response.Diagnostic = "native viewer may have opened; original operation must not be replayed"
	record.Response = outcomeOf(response)
	if err = registerNativeRecord(s.commandsDir, path, record); err != nil {
		writeError(w, http.StatusServiceUnavailable, "request_persist_failed", err.Error(), envelope.RequestID)
		return
	}
	err = s.product.OpenNativeSession(r.Context(), target)
	if err == nil {
		response.Status = "applied"
		response.Diagnostic = "original native viewer opened"
	} else {
		response.Diagnostic = err.Error()
	}
	record.Response = outcomeOf(response)
	if err = writeRequestRecord(s.commandsDir, path, record); err != nil {
		writeError(w, http.StatusServiceUnavailable, "request_persist_failed", err.Error(), envelope.RequestID)
		return
	}
	writeJSON(w, responseHTTPStatus(response.Status), response)
}

func registerNativeRecord(dir, path string, record requestRecord) error {
	data, err := marshalRequestRecord(record)
	if err != nil {
		return err
	}
	return writePrivateExclusive(dir, path, data)
}

func (s *Server) stopReceipt(ctx context.Context, response *CommandResponse) {
	wait, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	receipt, err := s.product.WaitControl(wait, response.RequestID, 10*time.Millisecond)
	if err == nil && receipt != nil {
		response.ControlReceipt = receipt
		response.Status = normalizedControlStatus(receipt.Status)
		response.Code = receipt.Code
		response.Diagnostic = receipt.Message
	}
}
