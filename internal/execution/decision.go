package execution

import (
	"context"
	"errors"
	"fmt"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
	"strings"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

// WriteDecisionRequest creates a new decision identity in the service-owned
// runtime projection. The caller persists it to SQLite with the run transition.
func WriteDecisionRequest(r *reconstructiontypes.RunView, req executionmodel.DecisionRequest) (*executionmodel.DecisionState, error) {
	return writeDecisionRequestWithID(r, req, newID())
}

func DecisionRequestFromEventFact(fact executionbackend.EventFact) executionmodel.DecisionRequest {
	request := executionmodel.DecisionRequest{FormatVersion: executionmodel.DecisionRequestFormatVersion, Question: fact.QuestionText,
		Context: fact.QuestionContext, Options: append([]executionbackend.DecisionOption(nil), fact.QuestionOptions...), AllowFreeText: fact.AllowFreeText}
	if len(fact.Questions) == 0 {
		return request
	}
	request.Questions = cloneExecutionQuestions(fact.Questions)
	if fact.QuestionRoute != nil {
		route := fact.QuestionRoute.Clone()
		request.QuestionRoute = &route
	}
	if len(fact.Questions) == 1 {
		question := fact.Questions[0]
		request.Question = question.Question
		request.Options = append([]executionbackend.DecisionOption(nil), question.Options...)
		request.AllowFreeText = question.AllowFreeText
		if request.Context == "" && fact.QuestionRoute != nil {
			request.Context = "A human answer is required to continue the original execution."
		}
		return request
	}
	request.Question = fmt.Sprintf("Current execution has %d questions requiring an answer", len(fact.Questions))
	var body strings.Builder
	if request.Context == "" && fact.QuestionRoute != nil {
		request.Context = "A human answer is required to continue the original execution."
	}
	if request.Context != "" {
		body.WriteString(request.Context)
		body.WriteString("\n\n")
	}
	body.WriteString("Answer every question using JSON keyed by question ID.")
	for _, question := range fact.Questions {
		fmt.Fprintf(&body, "\n\n[%s] %s: %s", question.ID, question.Header, question.Question)
		for _, option := range question.Options {
			fmt.Fprintf(&body, "\n- %s: %s", option.ID, option.Label)
			if option.Detail != "" {
				fmt.Fprintf(&body, " (%s)", option.Detail)
			}
		}
		if question.AllowFreeText {
			body.WriteString("\nFree text is allowed.")
		}
	}
	request.Context = body.String()
	request.AllowFreeText = true
	return request
}

func writeDecisionRequestWithID(r *reconstructiontypes.RunView, req executionmodel.DecisionRequest, decisionID string) (*executionmodel.DecisionState, error) {
	if r == nil || strings.TrimSpace(decisionID) == "" {
		return nil, errors.New("decision requires a run and stable ID")
	}
	req.FormatVersion = executionmodel.DecisionRequestFormatVersion
	if e := executionmodel.ValidateDecisionRequest(&req); e != nil {
		return nil, e
	}
	if req.QuestionRoute != nil && (req.QuestionRoute.WorkflowRunID != r.RunID || req.QuestionRoute.ExecutionID != r.ExecutionID || req.QuestionRoute.AttemptID != r.AttemptID) {
		return nil, errors.New("question route does not belong to the decision's fixed execution and attempt")
	}
	req.Questions = cloneExecutionQuestions(req.Questions)
	if req.QuestionRoute != nil {
		route := req.QuestionRoute.Clone()
		req.QuestionRoute = &route
	}
	st := &executionmodel.DecisionState{
		ID:        decisionID,
		AttemptID: r.AttemptID,
		Version:   1,
		Request:   req,
		Status:    "pending",
	}
	return st, nil
}

func NormalizeNativeDecisionAnswer(req *executionmodel.DecisionRequest, answer *executionmodel.DecisionAnswer) error {
	if req == nil || len(executionmodel.DecisionQuestions(req)) == 0 || answer == nil {
		return errors.New("native answer has no fixed question")
	}
	questions := executionmodel.DecisionQuestions(req)
	if len(answer.Answers) != 0 {
		return executionmodel.ValidateDecisionAnswer(req, answer)
	}
	note := strings.TrimSpace(answer.Note)
	if strings.HasPrefix(note, "{") {
		var values map[string][]string
		if err := decode([]byte(note), &values); err != nil {
			return fmt.Errorf("native multi-question answer must be JSON keyed by question ID: %w", err)
		}
		answer.Answers = values
	} else if len(questions) == 1 {
		q := questions[0]
		values := []string{}
		if answer.OptionID != "" {
			for _, option := range q.Options {
				if option.ID == answer.OptionID {
					values = append(values, option.Label)
					break
				}
			}
			if len(values) == 0 {
				return errors.New("native answer option is not offered")
			}
		}
		if note != "" {
			values = append(values, note)
		}
		answer.Answers = map[string][]string{q.ID: values}
	} else {
		return errors.New("answer all native questions using -note JSON, keyed by question ID")
	}
	return executionmodel.ValidateDecisionAnswer(req, answer)
}

func cloneExecutionQuestions(questions []executionbackend.Question) []executionbackend.Question {
	copy := make([]executionbackend.Question, len(questions))
	for i := range questions {
		copy[i] = questions[i]
		copy[i].Options = append([]executionbackend.DecisionOption(nil), questions[i].Options...)
	}
	return copy
}

// ReadDecisionAnswer returns the answer already committed to the current
// service projection. A pending decision has no answer; clients cannot create
// one by writing a file.
func ReadDecisionAnswer(r *reconstructiontypes.RunView) (executionmodel.DecisionAnswer, error) {
	if r.Decision == nil {
		return executionmodel.DecisionAnswer{}, errors.New("run has no decision")
	}
	if r.Decision.Status == "pending" {
		return executionmodel.DecisionAnswer{}, errors.New("decision has no accepted answer")
	}
	a := executionmodel.DecisionAnswer{DecisionID: r.Decision.ID, RunID: r.RunID, OptionID: r.Decision.OptionID, Note: r.Decision.Note, Answers: r.Decision.Answers, AnsweredBy: r.Decision.AnsweredBy, SourceAttemptID: r.Decision.SourceAttemptID, ExpectedRecoveryVersion: r.Decision.RecoveryVersion}
	a.ExpectedDecisionVersion, a.ExpectedRunRevision = r.Decision.AnswerDecisionVersion, r.Decision.AnswerRunRevision
	if r.Decision.AnsweredAt != nil {
		a.AnsweredAt = *r.Decision.AnsweredAt
	}
	if e := executionmodel.ValidateDecisionAnswer(&r.Decision.Request, &a); e != nil {
		return executionmodel.DecisionAnswer{}, e
	}
	return a, nil
}

func decisionAnswerMeta(r *reconstructiontypes.RunView) map[string]any {
	if r.Decision == nil {
		return nil
	}
	return map[string]any{
		"decision_id": r.Decision.ID,
		"option_id":   r.Decision.OptionID,
		"answered_by": r.Decision.AnsweredBy,
	}
}

func (s *Service) requireDecisionReplyCapability(ctx context.Context, run *reconstructiontypes.RunView) error {
	return DecisionReplyEligibility(ctx, s.Config, run)
}

// DecisionReplyEligibility is the shared read-only qualification used by the
// CLI, console client and service handler. It checks the immutable execution
// binding and verified reply capabilities for this exact target.
func DecisionReplyEligibility(ctx context.Context, c Config, run *reconstructiontypes.RunView) error {
	if err := c.RequireNativeTaskAuthority(); err != nil {
		return err
	}
	if run == nil || run.Decision == nil || run.Decision.Status != "pending" {
		return errors.New("decision is not pending on the current native run")
	}
	if run.Abandonment != nil || run.TaskStatus == "closed" || run.TaskStatus == "discarded" {
		return nativeErr("candidate_inactive", "candidate no longer accepts feedback")
	}
	if run.EnvironmentBlocker != nil {
		return nativeErr("environment_blocked", "resolve the environment blocker before sending feedback")
	}
	if run.WorkflowVersion != executionmodel.Version || !executionbackend.StableExecutionID(run.ExecutionID) || run.AttemptID == "" || run.Decision.AttemptID != run.AttemptID {
		return &executionbackend.BackendError{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "feedback does not belong to the fixed worker execution"}
	}

	if run.RecoveryRequired && (run.RecoveryState == "unverified" || run.RecoveryState == "" ||
		run.RecoveryState == "interrupted" && run.Decision.Request.QuestionRoute != nil) {
		return &executionbackend.BackendError{Code: executionbackend.BackendUnsupported, Diagnostic: "original worker state or native question is not verified; wait for a recovery decision"}
	}
	return ctx.Err()
}

func (s *Service) applyDecisions(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, run := range s.runs {
		if run == nil || run.FinishTaskStatus != "waiting_decision" && run.FinishTaskStatus != "waiting_pr_merge" || run.Decision == nil || run.Decision.Status != "answer_saved" {
			continue
		}
		blocked := false
		for _, blocker := range s.State.Control.Blockers {
			if blocker.RunID == run.RunID || blocker.RunID == "" && blocker.TaskID == run.TaskID {
				blocked = true
				break
			}
		}
		if blocked {
			continue
		}
		if run.WorkflowVersion != executionmodel.Version {
			return nativeErr("unsupported_schema", "saved decision is not bound to a current worker execution")
		}
		if err := s.applyWorkerFlowDecision(ctx, run); err != nil {
			return err
		}
	}
	return nil
}
