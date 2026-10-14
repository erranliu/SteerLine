package execution

import (
"github.com/erranliu/SteerLine/internal/execution/workflow"
	"errors"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
)

func asyncContinuationStep(w *executionmodel.WorkflowRunRecord, request executionmodel.DecisionRequest) (string, error) {
	if request.QuestionRoute == nil {
		return "", errors.New("async continuation lost its original question route")
	}
 step,_,err:=workflow.FeedbackStep(workflowSnapshot(w),workflow.FeedbackIntent{Async:true,SourceTurn:request.QuestionRoute.TurnID,SourceAttempt:request.QuestionRoute.AttemptID})
 return step,err
}

func (s *Service) expireAsyncQuestions(run *reconstructiontypes.RunView, e *executionmodel.ExecutionRecord) {
	if unresolvedAsyncDecision(run) {
		s.setDecisionStatus(run, "superseded")
	}
	pending := e.PendingQuestions[:0]
	for _, q := range e.PendingQuestions {
		if q.InteractionKind != "async" {
			pending = append(pending, q)
		}
	}
	e.PendingQuestions = pending
}

func unresolvedAsyncDecision(run *reconstructiontypes.RunView) bool {
	return run != nil && run.Decision != nil && run.Decision.Request.InteractionKind == "async" && (run.Decision.Status == "pending" || run.Decision.Status == "answer_saved")
}

func hasBlockingPendingQuestions(e *executionmodel.ExecutionRecord) bool {
	for _, q := range e.PendingQuestions {
		if q.InteractionKind != "async" {
			return true
		}
	}
	return false
}
