package httpapi

import (
	"context"
	"net/http"
	"net/url"

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

func (s *Server) readRecovery(ctx context.Context, path []string, values url.Values) (any, error) {
	candidate, err := s.candidate(ctx, path[1])
	if err != nil {
		return nil, err
	}
	if values.Get("execution_id") != candidate.ExecutionID || values.Get("attempt_id") != candidate.AttemptID || candidate.ExecutionID == "" || candidate.AttemptID == "" {
		return nil, &protocolError{status: 409, code: "stale_target", reason: "original execution/attempt binding changed"}
	}
	if path[2] == "recovery" {
		return s.recovery.InspectRecovery(ctx, path[1])
	}
	return struct{}{}, s.recovery.DecisionEligibility(ctx, candidate)
}
