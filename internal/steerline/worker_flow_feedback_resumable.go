package steerline

import "context"

func (s *Service) deliverResumableWorkerFeedback(ctx context.Context, run *Run, execution *ExecutionRecord, answer DecisionAnswer, commandID string, route workerFeedbackRoute) error {
	if execution.SessionID == "" || run.SessionID == nil || *run.SessionID != execution.SessionID {
		execution.Error = ptr("Feedback is saved, but the original worker session is not yet available for delivery")
		s.refreshRun(run)
		return s.save()
	}
	if run.Decision.Request.NativeAsk != nil {
		if route != feedbackLocalAppServer {
			return &BackendError{Code: BackendUnsupported, Diagnostic: "native Codex question belongs to a different worker feedback protocol"}
		}
		if s.workerMessages != nil && s.workers[run.RunID] == nil {
			return nil // Recovery must attach the original observer before replying.
		}
		return s.replyAppServerAsk(ctx, run, execution, answer)
	}
	if execution.Status == "active" || s.workers[run.RunID] != nil {
		return nil // Preserve the answer until the original turn stops.
	}
	for _, operation := range s.State.Operations {
		if operation.IdempotencyKey != "worker-turn:"+execution.ExecutionID+":"+commandID {
			continue
		}
		if operation.Status == "confirmed" || operation.Status == "accepted" || operation.Status == "in_flight" {
			return nil // The original request may have started. Never resend.
		}
		execution.Error = ptr("Feedback delivery remains unverified at the original request ID; no duplicate message was sent")
		s.refreshRun(run)
		return s.save()
	}
	return s.startWorkerFlowTurn(ctx, run, workerFeedbackText(answer, run.Decision.Request), commandID)
}

func workerFeedbackText(answer DecisionAnswer, request DecisionRequest) string {
	if answer.OptionID == "" {
		return answer.Note
	}
	label := answer.OptionID
	for _, option := range request.Options {
		if option.ID == answer.OptionID {
			label = option.Label
			break
		}
	}
	message := "Selected option: " + label
	if answer.Note != "" {
		message += "\n\nAdditional instructions:\n" + answer.Note
	}
	return message
}
