package execution

import (
	"context"
	"errors"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

func (s *Service) deliverLifecycleQuestionAnswer(ctx context.Context, run *reconstructiontypes.RunView, execution *executionmodel.ExecutionRecord, answer executionmodel.DecisionAnswer) error {
	if run != nil && s.lifecycleEffectPending(run.RunID) {
		return nil
	}

	if run == nil || run.Decision == nil || run.Decision.Request.QuestionRoute == nil || execution == nil || run.Decision.ID != answer.DecisionID {
		return errors.New("saved answer has no fixed original question")
	}
	lifecycle, fixed, err := s.lifecycleForRun(run)
	if err != nil {
		return err
	}
	if fixed.execution != execution && fixed.execution.ExecutionID != execution.ExecutionID {
		return executionmodel.ErrTurnStale
	}
	if lifecycle.PolicyCapabilities().LinkedWorkspaceWriteRoots {
		args, err := linkedWorktreeWriteArgs(ctx, fixed.config, s.runtime.Directory, execution.Workspace.Worktree)
		if err != nil {
			return err
		}
		if err := bindPolicyResources(ctx, fixed.config, s.runtime.Directory, s.runtime.ProcessHost, execution, execution.Workspace.Worktree, args); err != nil {
			return err
		}
	}
	key := "worker-question-answer:" + execution.ExecutionID + ":" + answer.DecisionID
	input := executionbackend.LifecycleInput{Feedback: &executionbackend.FeedbackInput{
		DecisionID: answer.DecisionID, QuestionID: run.Decision.Request.QuestionRoute.QuestionID,
		Message: workerFeedbackText(answer, run.Decision.Request), Answer: &executionbackend.Answer{Answers: answer.Answers},
	}}
	if operation := s.operationByKey(key); operation != nil {
		return s.reconcileLifecycleQuestionAnswer(ctx, run, lifecycle, fixed, operation, input)
	}
	operation, err := s.newLifecycleOperation(run, "reply_execution_question", key, input.Feedback)
	if err != nil {
		return err
	}
	snapshot, err := s.lifecycleSnapshot(run, fixed.workflow, execution, fixed.attempt, nil)
	if err != nil {
		return err
	}
	request, err := checkedLifecycleRequest(snapshot, operation, executionbackend.LifecycleDeliverFeedback, input)
	if err != nil {
		return err
	}
	err = s.dispatchLifecycleEffect(ctx, run, lifecycle, request, false, func(run *reconstructiontypes.RunView, result lifecycleEffectResult) error {
		if result.observer != nil {
			_ = result.observer.Close(context.Background())
		}
		return s.applyLifecycleQuestionAnswerReceipt(run, request, result.receipt, result.err)
	})
	if errors.Is(err, errLifecycleEffectPending) {
		return nil
	}
	return err
}

func (s *Service) reconcileLifecycleQuestionAnswer(ctx context.Context, run *reconstructiontypes.RunView, lifecycle executionbackend.LifecyclePort, fixed workerFlowContext, operation *executionmodel.OperationRecord, input executionbackend.LifecycleInput) error {
	if operation == nil || operation.Status == "pending" {
		return errors.New("original answer has no persisted send operation")
	}
	if err := validateSavedLifecycleOperation(operation, executionbackend.LifecycleDeliverFeedback, input); err != nil {
		return err
	}
	if operation.Status == "accepted" || operation.Status == "confirmed" {
		return nil // The original send may have reached the native question; never resend it.
	}
	snapshot, err := s.lifecycleSnapshot(run, fixed.workflow, fixed.execution, fixed.attempt, nil)
	if err != nil {
		return err
	}
	request, err := checkedLifecycleRequest(snapshot, operation, executionbackend.LifecycleDeliverFeedback, input)
	if err != nil {
		return err
	}
	receipt, observer, reconcileErr := lifecycle.Reconcile(ctx, request)
	if observer != nil {
		_ = observer.Close(context.Background())
	}
	if err := receipt.ValidateFor(request); err != nil {
		return err
	}
	if reconcileErr != nil || receipt.Disposition != executionbackend.OperationConfirmed || !receipt.State.InputAccepted {
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

func (s *Service) reconcileSavedLifecycleQuestionAnswer(ctx context.Context, run *reconstructiontypes.RunView) error {
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
	input := executionbackend.LifecycleInput{Feedback: &executionbackend.FeedbackInput{
		DecisionID: answer.DecisionID, QuestionID: run.Decision.Request.QuestionRoute.QuestionID,
		Message: workerFeedbackText(answer, run.Decision.Request), Answer: &executionbackend.Answer{Answers: answer.Answers},
	}}
	return s.reconcileLifecycleQuestionAnswer(ctx, run, lifecycle, fixed, operation, input)
}

func (s *Service) applyLifecycleQuestionAnswerReceipt(run *reconstructiontypes.RunView, request executionbackend.LifecycleRequest, receipt executionbackend.LifecycleReceipt, executeErr error) error {
	if err := receipt.ValidateFor(request); err != nil {
		return err
	}
	if run == nil || run.TaskStatus == "closed" || run.TaskStatus == "discarded" || run.Decision == nil ||
		run.Decision.Status != "answer_saved" || run.Decision.ID != request.Input.Feedback.DecisionID ||
		run.Decision.Request.QuestionRoute == nil || run.Decision.Request.QuestionRoute.QuestionID != request.Input.Feedback.QuestionID {
		return nil
	}
	if err := validateSavedLifecycleOperation(snapshotOperation(&s.domain, request.Operation.ID), executionbackend.LifecycleDeliverFeedback, request.Input); err != nil {
		return err
	}
	for _, operation := range s.domain.Operations {
		if operation.OperationID == request.Operation.ID && operation.Status == "confirmed" {
			return nil // The matching native tool item committed first.
		}
	}
	// Feedback confirms an input fact and attaches no resource candidate.
	// Observer cursors/seen-message updates may change opaque state meanwhile;
	// retain the exact physical/session/policy/question binding and current CAS.
	if receipt.Process != nil || receipt.Target != nil || receipt.Workspace != nil || len(receipt.Implementation) != 0 {
		return errors.New("question feedback receipt cannot replace execution resources")
	}
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return err
	}
	current, err := s.lifecycleSnapshot(run, fixed.workflow, fixed.execution, fixed.attempt, nil)
	if err != nil {
		return err
	}
	original := request.Snapshot
	if current.Ref != original.Ref || current.Session.SessionID != original.Session.SessionID || current.Session.Generation != original.Session.Generation || current.ConfigurationDigest != original.ConfigurationDigest || current.ResourceGeneration != original.ResourceGeneration || recordFingerprint(current.PolicyResources) != recordFingerprint(original.PolicyResources) || recordFingerprint(current.QuestionRoute) != recordFingerprint(original.QuestionRoute) {
		return executionmodel.ErrTurnStale
	}
	if executeErr != nil || receipt.Disposition == executionbackend.OperationUnknown || receipt.Disposition == executionbackend.OperationRejected {
		status, reason := lifecycleOutcome(receipt, executeErr)
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
	if receipt.Disposition == executionbackend.OperationConfirmed && receipt.State.InputAccepted {
		return s.confirmLifecycleQuestionAnswer(run, request, receipt)
	}
	if receipt.Disposition != executionbackend.OperationAccepted || !receipt.State.InputAccepted {
		return errors.New("question answer result has no accepted original request")
	}
	if err := s.setOperationState(request.Operation.ID, "accepted", receipt.RequestID, mustJSON(receipt), ""); err != nil {
		return err
	}
	s.note(run, "steer", "decision", "reply_sent", "answer was sent to the original execution question", receipt.RequestID, nil)
	return s.commitWorker(run, request.Operation.ID)
}

func (s *Service) confirmLifecycleQuestionAnswer(run *reconstructiontypes.RunView, request executionbackend.LifecycleRequest, receipt executionbackend.LifecycleReceipt) error {
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
		decision, err := WriteDecisionRequest(run, DecisionRequestFromEventFact(executionbackend.EventFact{Questions: next.Questions, QuestionRoute: &next.Route}))
		if err != nil {
			return err
		}
		s.setRunDecision(run, decision)
	} else if workflow.Status == "waiting_input" {
		if err := ContinueWorkerQuestion(workflow, execution, execution.CurrentTurnID); err != nil {
			return err
		}
		s.refreshRun(run)
	}
	return s.commitWorker(run, request.Operation.ID)
}
