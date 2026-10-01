package steerline

import (
	"context"
	"errors"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

func (s *Service) deliverLifecycleQuestionAnswer(ctx context.Context, run *Run, execution *ExecutionRecord, answer DecisionAnswer) error {
	if run == nil || run.Decision == nil || run.Decision.Request.QuestionRoute == nil || execution == nil || run.Decision.ID != answer.DecisionID {
		return errors.New("saved answer has no fixed original question")
	}
	lifecycle, fixed, err := s.lifecycleForRun(run)
	if err != nil {
		return err
	}
	if fixed.execution != execution && fixed.execution.ExecutionID != execution.ExecutionID {
		return ErrWorkerTurnStale
	}
	if lifecycle.PolicyCapabilities().LinkedWorkspaceWriteRoots {
		args, err := codexLinkedWorktreeWriteArgs(ctx, fixed.config, execution.Workspace.Worktree)
		if err != nil {
			return err
		}
		if err := bindPolicyResources(ctx, fixed.config, execution, execution.Workspace.Worktree, args); err != nil {
			return err
		}
	}
	key := "worker-question-answer:" + execution.ExecutionID + ":" + answer.DecisionID
	input := executionapi.LifecycleInput{Feedback: &executionapi.FeedbackInput{
		DecisionID: answer.DecisionID, QuestionID: run.Decision.Request.QuestionRoute.QuestionID,
		Message: workerFeedbackText(answer, run.Decision.Request), Answer: &executionapi.Answer{Answers: answer.Answers},
	}}
	if operation := s.operationByKey(key); operation != nil {
		return s.reconcileLifecycleQuestionAnswer(ctx, run, lifecycle, fixed, operation, input)
	}
	operationID, err := s.startWorkerOperation(run, "reply_execution_question", key, input.Feedback)
	if err != nil {
		return err
	}
	operation := s.operationByKey(key)
	if operation == nil || operation.OperationID != operationID {
		return errors.New("saved answer operation disappeared before delivery")
	}
	if err := validateSavedLifecycleOperation(operation, executionapi.LifecycleDeliverFeedback, input); err != nil {
		return err
	}
	snapshot, err := s.lifecycleSnapshot(run, fixed.workflow, execution, fixed.attempt, nil)
	if err != nil {
		return err
	}
	request := lifecycleRequest(snapshot, *operation, executionapi.LifecycleDeliverFeedback, input)
	if err := request.Validate(); err != nil {
		return err
	}
	if s.workerMessages != nil {
		runID := run.RunID
		ctx = s.runContext
		s.forwarders.Add(1)
		go func() {
			defer s.forwarders.Done()
			receipt, observer, executeErr := lifecycle.Execute(ctx, request.Clone())
			if observer != nil {
				_ = observer.Close(context.Background())
			}
			message := &lifecycleReplyMessage{request: request, receipt: receipt.Clone(), err: executeErr}
			select {
			case s.workerMessages <- workerMessage{runID: runID, reply: message}:
			case <-ctx.Done():
			}
		}()
		return nil
	}
	receipt, observer, executeErr := lifecycle.Execute(ctx, request.Clone())
	if observer != nil {
		_ = observer.Close(context.Background())
	}
	return s.applyLifecycleQuestionAnswerReceipt(run, request, receipt, executeErr)
}

func (s *Service) reconcileLifecycleQuestionAnswer(ctx context.Context, run *Run, lifecycle executionapi.LifecyclePort, fixed workerFlowContext, operation *OperationRecord, input executionapi.LifecycleInput) error {
	if operation == nil || operation.Status == "pending" {
		return errors.New("original answer has no persisted send operation")
	}
	if err := validateSavedLifecycleOperation(operation, executionapi.LifecycleDeliverFeedback, input); err != nil {
		return err
	}
	if operation.Status == "accepted" || operation.Status == "confirmed" {
		return nil // The original send may have reached the native question; never resend it.
	}
	snapshot, err := s.lifecycleSnapshot(run, fixed.workflow, fixed.execution, fixed.attempt, nil)
	if err != nil {
		return err
	}
	request := lifecycleRequest(snapshot, *operation, executionapi.LifecycleDeliverFeedback, input)
	if err := request.Validate(); err != nil {
		return err
	}
	receipt, observer, reconcileErr := lifecycle.Reconcile(ctx, request)
	if observer != nil {
		_ = observer.Close(context.Background())
	}
	if err := receipt.ValidateFor(request); err != nil {
		return err
	}
	if reconcileErr != nil || receipt.Disposition != executionapi.OperationConfirmed || !receipt.State.InputAccepted {
		if reconcileErr != nil {
			return reconcileErr
		}
		if receipt.Diagnostic != "" {
			return errors.New(receipt.Diagnostic)
		}
		return nil // The original unknown operation remains read-only and requires later evidence.
	}
	return s.confirmLifecycleQuestionAnswer(run, request, receipt)
}

func (s *Service) reconcileSavedLifecycleQuestionAnswer(ctx context.Context, run *Run) error {
	if run == nil || run.Decision == nil || run.Decision.Status != "answer_saved" || run.Decision.Request.QuestionRoute == nil {
		return nil
	}
	key := "worker-question-answer:" + run.ExecutionID + ":" + run.Decision.ID
	operation := s.operationByKey(key)
	if operation == nil || operation.Status == "accepted" || operation.Status == "confirmed" {
		return nil
	}
	if operation.Status != "in_flight" && operation.Status != "outcome_unknown" {
		return errors.New("saved app-server answer has no reconcileable original send operation")
	}
	lifecycle, fixed, err := s.lifecycleForRun(run)
	if err != nil {
		return err
	}
	answer, err := ReadDecisionAnswer(run)
	if err != nil {
		return err
	}
	input := executionapi.LifecycleInput{Feedback: &executionapi.FeedbackInput{
		DecisionID: answer.DecisionID, QuestionID: run.Decision.Request.QuestionRoute.QuestionID,
		Message: workerFeedbackText(answer, run.Decision.Request), Answer: &executionapi.Answer{Answers: answer.Answers},
	}}
	return s.reconcileLifecycleQuestionAnswer(ctx, run, lifecycle, fixed, operation, input)
}

func (s *Service) applyLifecycleQuestionAnswerReceipt(run *Run, request executionapi.LifecycleRequest, receipt executionapi.LifecycleReceipt, executeErr error) error {
	if err := receipt.ValidateFor(request); err != nil {
		return err
	}
	if err := s.validateLifecycleResultBinding(run, request); err != nil {
		return err
	}
	if run == nil || run.TaskStatus == "closed" || run.TaskStatus == "discarded" || run.Decision == nil ||
		run.Decision.Status != "answer_saved" || run.Decision.ID != request.Input.Feedback.DecisionID ||
		run.Decision.Request.QuestionRoute == nil || run.Decision.Request.QuestionRoute.QuestionID != request.Input.Feedback.QuestionID {
		return nil
	}
	if err := validateSavedLifecycleOperation(snapshotOperation(&s.domain, request.Operation.ID), executionapi.LifecycleDeliverFeedback, request.Input); err != nil {
		return err
	}
	for _, operation := range s.domain.Operations {
		if operation.OperationID == request.Operation.ID && operation.Status == "confirmed" {
			return nil // The matching native tool item committed first.
		}
	}
	if executeErr != nil || receipt.Disposition == executionapi.OperationUnknown || receipt.Disposition == executionapi.OperationRejected {
		reason := receipt.Diagnostic
		if executeErr != nil {
			reason = executeErr.Error()
		}
		status := string(receipt.Disposition)
		if status != "rejected" {
			status = "outcome_unknown"
		}
		if err := s.setOperationState(request.Operation.ID, status, receipt.RequestID, mustJSON(receipt), reason); err != nil {
			return err
		}
		if execution := s.executionRecord(run); execution != nil {
			execution.Error = ptr("Original question answer delivery is unverified: " + reason)
			s.refreshRun(run)
		}
		return s.commitWorker(run, request.Operation.ID)
	}
	if receipt.Disposition == executionapi.OperationConfirmed && receipt.State.InputAccepted {
		return s.confirmLifecycleQuestionAnswer(run, request, receipt)
	}
	if receipt.Disposition != executionapi.OperationAccepted || !receipt.State.InputAccepted {
		return errors.New("question answer result has no accepted original request")
	}
	if err := s.setOperationState(request.Operation.ID, "accepted", receipt.RequestID, mustJSON(receipt), ""); err != nil {
		return err
	}
	s.note(run, "steer", "decision", "reply_sent", "answer was sent to the original execution question", receipt.RequestID, nil)
	return s.commitWorker(run, request.Operation.ID)
}

func (s *Service) confirmLifecycleQuestionAnswer(run *Run, request executionapi.LifecycleRequest, receipt executionapi.LifecycleReceipt) error {
	if run == nil || run.Decision == nil || run.Decision.ID != request.Input.Feedback.DecisionID || run.Decision.Status != "answer_saved" {
		return nil
	}
	if err := s.setOperationState(request.Operation.ID, "confirmed", receipt.RequestID, mustJSON(receipt), ""); err != nil {
		return err
	}
	s.setDecisionStatus(run, "reply_accepted")
	workflow, execution := s.workflowRecords(run)
	if workflow == nil || execution == nil {
		return errors.New("confirmed answer lost its original execution")
	}
	if len(execution.PendingQuestions) > 0 {
		next := execution.PendingQuestions[0]
		execution.PendingQuestions = execution.PendingQuestions[1:]
		decision, err := WriteDecisionRequest(run, decisionRequestFromEventFact(executionapi.EventFact{Questions: next.Questions, QuestionRoute: &next.Route}))
		if err != nil {
			return err
		}
		s.setRunDecision(run, decision)
	} else if workflow.Phase == "waiting_decision" {
		if err := ContinueWorkerQuestion(workflow, execution, execution.CurrentTurnID); err != nil {
			return err
		}
		s.refreshRun(run)
	}
	return s.commitWorker(run, request.Operation.ID)
}

func (s *Service) handleLifecycleFeedbackMessage(runID string, message *lifecycleReplyMessage) error {
	if message == nil {
		return errors.New("lifecycle reply result is required")
	}
	var run *Run
	for _, candidate := range s.runs {
		if candidate != nil && candidate.RunID == runID {
			run = candidate
			break
		}
	}
	if run == nil {
		return nil
	}
	feedback := message.request.Input.Feedback
	if run.TaskStatus == "closed" || run.TaskStatus == "discarded" || run.Decision == nil || run.Decision.Status != "answer_saved" ||
		feedback == nil || run.Decision.ID != feedback.DecisionID || run.Decision.Request.QuestionRoute == nil ||
		run.Decision.Request.QuestionRoute.QuestionID != feedback.QuestionID {
		return nil // A terminal or replaced Decision cannot be changed by a late native result.
	}
	if message.request.Operation.ID == "" {
		return errors.New("lifecycle reply result lacks its original durable operation")
	}
	return s.applyLifecycleQuestionAnswerReceipt(run, message.request, message.receipt, message.err)
}
