package steerline

import "strings"

// Every platform reaches the same business transition after its own adapter
// has verified the original turn's terminal evidence.
func (s *Service) stopVerifiedWorkerTurn(run *Run, workflow *WorkflowRunRecord, execution *ExecutionRecord, turnID, outcome string, question bool) error {
	if err := StopWorkerTurn(workflow, execution, turnID, question); err != nil {
		return err
	}
	execution.TurnEndedAt = ptr(now())
	execution.LastTurnOutcome = outcome
	s.refreshRun(run)
	return nil
}

type stoppedWorkerResolution struct {
	TurnID       string
	LastMessage  string
	Issue        string
	Question     bool
	WaitReason   string
	RecordPRStop bool
}

func (s *Service) resolveStoppedWorkerTurn(run *Run, workflow *WorkflowRunRecord, execution *ExecutionRecord, resolution stoppedWorkerResolution) error {
	if workflow.Phase == "waiting_pr_merge" {
		if err := s.offerWorkerPRFeedback(run, workflow.PRURL); err != nil {
			return err
		}
		if resolution.Issue != "" {
			execution.Error = ptr(clip(resolution.Issue, 2000))
			s.refreshRun(run)
		}
		if resolution.RecordPRStop {
			s.note(run, "steer", "worker", "stopped", "worker turn ended with a verified PR", workflow.PRURL, map[string]any{"turn_id": resolution.TurnID})
		}
		return s.save()
	}
	context := resolution.WaitReason
	if context == "" {
		context = strings.TrimSpace(resolution.LastMessage)
		if context == "" {
			context = "The worker stopped without a final reply."
		}
		if resolution.Issue != "" {
			context += "\n\nStop reason: " + resolution.Issue
		}
	}
	if resolution.Question {
		decision, err := WriteDecisionRequest(run, DecisionRequest{Question: clip(strings.TrimSpace(resolution.LastMessage), 1800), Context: clip(context, 7000), Options: []DecisionOption{{ID: "continue", Label: "Continue current work"}}, AllowFreeText: true})
		if err == nil {
			s.setRunDecision(run, decision)
		}
	}
	return s.waitWorkerFlow(run, context)
}
