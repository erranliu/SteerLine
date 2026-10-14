package httpapi

import (
	"context"
	"net/http"

	api "github.com/erranliu/SteerLine/internal/product/api"
)

// Empty attempt selects the run's full history. An explicit historical attempt
// must be present in the formal conversation projection for this run/execution.
func (s *Server) conversationIdentity(ctx context.Context, candidate *api.CandidateSummary, execution, attempt string) (string, string, error) {
	if candidate == nil || candidate.ExecutionID == "" || execution != "" && execution != candidate.ExecutionID {
		return "", "", &protocolError{status: http.StatusConflict, code: "target_mismatch", reason: "conversation belongs to another execution"}
	}
	if attempt != "" && attempt != candidate.AttemptID {
		page, err := s.conversation.ConversationPage(ctx, api.ConversationFilter{WorkflowID: candidate.RunID, ExecutionID: candidate.ExecutionID, AttemptID: attempt}, "", 1)
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
		result, err := s.recovery.InspectRecovery(r.Context(), path[1])
		writeResult(w, result, err)
	} else {
		err := s.recovery.DecisionEligibility(r.Context(), candidate)
		writeResult(w, struct{}{}, err)
	}
	return true
}
