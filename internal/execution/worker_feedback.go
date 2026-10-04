package execution

import (
	contracts "github.com/erranliu/SteerLine/internal/steerline/contracts"
)

import (
	"context"
	"errors"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

func (s *Service) waitWorkerFlow(run *contracts.Run, reason string, operationIDs ...string) error {
	workflow, execution := s.workflowRecords(run)
	if workflow == nil || execution == nil || workflow.WorkflowVersion != executionmodel.Version {
		return errors.New("worker flow wait has no fixed execution")
	}
	if s.candidateBusinessClosed(run) {
		return nil
	}
	facts := workerWaitFacts{
		PreserveActiveStatus:   execution.Status == "active" && execution.CurrentTurnID != "" && execution.CurrentTurnID != execution.LastStoppedTurnID,
		PreserveUnresolvedTurn: !execution.RecoveryRequired && (execution.PendingCommandID != "" || execution.CurrentTurnID != "" && execution.CurrentTurnID != execution.LastStoppedTurnID),
	}
	if execution.CurrentTurnID != "" && execution.CurrentTurnID == execution.LastStoppedTurnID {
		facts.ContextPrefix = "The previous turn ended. Further input will use the same fixed execution session.\n\n"
	}
	if execution.RecoveryRequired && (execution.RecoveryState == "interrupted" || execution.RecoveryState == "completed") && execution.Process != nil {
		facts.Options = []executionbackend.DecisionOption{{ID: "continue", Label: "Continue the original session", Detail: "Verify and resume the original session identity"}, {ID: "new_session", Label: "Continue in a new session", Detail: "Use the existing workspace and saved history"}}
	} else if execution.RecoveryRequired && execution.RecoveryState == "interrupted" {
		facts.Options = []executionbackend.DecisionOption{{ID: "continue", Label: "Continue this execution", Detail: "Verify or prepare its original workspace first"}}
	}
	unresolvedNativeTurn := facts.PreserveUnresolvedTurn
	if !unresolvedNativeTurn && (run.Decision == nil || run.Decision.Status != "pending" && run.Decision.Status != "answer_saved") {
		contextText := facts.ContextPrefix + reason
		options := []executionbackend.DecisionOption{{ID: "continue", Label: "Continue current work"}}
		if facts.Options != nil {
			options = facts.Options
		}
		question := "The worker stopped. What should it do next?"
		if execution.RecoveryRequired && (execution.RecoveryState == "unverified" || execution.RecoveryState == "") {
			question = "Worker state is unverified. Review the original execution evidence."
			options = nil
		}
		request := executionmodel.DecisionRequest{Question: question, Context: clip(contextText, 7000), Options: options, AllowFreeText: true}
		decision, err := WriteDecisionRequest(run, request)
		if err != nil {
			return err
		}
		s.setRunDecision(run, decision)
		if execution.RecoveryRequired {
			if record := s.decisionRecord(run); record != nil {
				record.RecoveryVersion = execution.RecoveryVersion
				record.RecoveryState = execution.RecoveryState
			}
		}
	}
	workflow.Phase, workflow.UpdatedAt = "waiting_decision", now()
	if unresolvedNativeTurn && (run.Decision == nil || run.Decision.Status != "pending" && run.Decision.Status != "answer_saved") {
		workflow.Phase = "executing"
		if execution.CurrentTurnID == "" || execution.CurrentTurnID == execution.LastStoppedTurnID {
			workflow.Phase = "queued"
		}
	}
	if execution.Status == "active" && !facts.PreserveActiveStatus {
		execution.Status = "idle"
	}
	execution.UpdatedAt = now()
	execution.Error = ptr(clip(reason, 2000))
	s.refreshRun(run)
	if unresolvedNativeTurn && workflow.Phase != "waiting_decision" {
		s.note(run, "steer", "worker", "unknown", "original worker turn needs reconciliation", reason, nil)
	} else {
		s.note(run, "steer", "decision", "pending", "worker needs input", reason, decisionMeta(run))
	}
	return s.commitWorker(run, operationIDs...)
}

func (s *Service) applyWorkerFlowDecision(ctx context.Context, run *contracts.Run) error {
	if s.candidateBusinessClosed(run) {
		return nil
	}
	fixed, contextErr := s.workerFlowContext(run)
	if contextErr != nil {
		return contextErr
	}
	c := fixed.config
	if run == nil || run.WorkflowVersion != executionmodel.Version || run.Decision == nil || run.Decision.Status != "answer_saved" {
		return nil
	}
	workflow, execution := s.workflowRecords(run)
	if workflow == nil || execution == nil || workflow.Phase != "waiting_decision" && workflow.Phase != "waiting_pr_merge" {
		return errors.New("saved feedback no longer belongs to a waiting worker flow")
	}
	config, err := s.executionConfigForRun(run)
	if err != nil {
		return err
	}
	if _, err := workerFlowRegistrationFor(config, run, execution); err != nil {
		return err
	}
	answer, err := ReadDecisionAnswer(run)
	if err != nil {
		return err
	}
	command, err := s.commandStore.CommandByIdempotency(ctx, c.identity, DecisionAnswerKey(answer))
	if err != nil || command.Kind != "answer_decision" || command.TargetID != answer.DecisionID || command.Status != "accepted" || !executionbackend.StableExecutionID(command.CommandID) {
		return errors.Join(err, errors.New("saved worker feedback has no fixed accepted command identity"))
	}
	if err := s.checkCurrentExecutionRequirements(ctx, fixed); err != nil {
		return err
	}
	if execution.Environment != nil && execution.Environment.Blocker != nil {
		return nil
	}
	if execution.RecoveryRequired && (answer.SourceAttemptID != run.AttemptID || answer.ExpectedRecoveryVersion != execution.RecoveryVersion ||
		execution.RecoveryMarkedAt == nil || !command.CreatedAt.After(*execution.RecoveryMarkedAt)) {
		return nil // Prior-service input cannot advance the feedback phase or send.
	}
	if execution.RecoveryRequired && (execution.RecoveryState == "unverified" || execution.RecoveryState == "") {
		return nil // Verify the original worker and pending operations first.
	}
	if execution.RecoveryRequired && s.hasUnknownWorkerOperation(run) {
		execution.RecoveryState = "unverified"
		return s.waitWorkerFlow(run, "An earlier external operation has an unknown outcome; no recovery input was sent")
	}
	if workflow.Phase == "waiting_pr_merge" {
		decision := s.decisionRecord(run)
		if decision == nil || s.store == nil || snapshotWorkflow(&s.committed, workflow.WorkflowRunID) == nil || recordFingerprint(*snapshotWorkflow(&s.committed, workflow.WorkflowRunID)) != recordFingerprint(*workflow) {
			return errors.New("worker feedback phase has unpublished or missing actor state")
		}
		if err := s.store.Err(); err != nil {
			return err
		}
		previousWorkflow := *workflow
		if err := BeginWorkerFeedbackDelivery(workflow, *execution, *decision, now()); err != nil {
			return err
		}
		s.refreshRun(run)
		if err := s.commitWorker(run); err != nil {
			*workflow = previousWorkflow
			s.refreshRun(run)
			return err
		}
		workflow, execution = s.workflowRecords(run)
		if workflow == nil || execution == nil || run.ControlRevision != workflow.ControlRevision || run.FinishTaskStatus != "waiting_decision" {
			return errors.New("committed worker feedback phase could not be projected")
		}
	}
	if execution.RecoveryRequired {
		if execution.RecoveryState == "interrupted" || execution.RecoveryState == "completed" {
			if execution.Process == nil && execution.SessionID == "" && execution.TurnNumber == 0 {
				return s.startManuallyRecoveredInitialWorker(ctx, run, answer, command.CommandID)
			}
			prompt := "The previous worker process stopped. Inspect the current worktree, Git status, existing work, and persistent task history before continuing. Do not replay an earlier request blindly.\n\n" + workerFeedbackText(answer, run.Decision.Request)
			if err := s.beginLocalRecoveryAttempt(ctx, run, answer.OptionID); err != nil {
				return s.waitWorkerFlow(run, "Manual execution recovery did not pass its original identity checks: "+err.Error())
			}
			return s.startWorkerFlowTurn(ctx, run, prompt, command.CommandID)
		}
		if execution.RecoveryState != "live" && execution.RecoveryState != "running" && execution.RecoveryState != "question" {
			return nil
		}
		execution.RecoveryRequired = false
		if err := s.commitWorker(run); err != nil {
			return err
		}
	}
	if run.Decision.Request.QuestionRoute != nil {
		return s.deliverLifecycleQuestionAnswer(ctx, run, execution, answer)
	}
	return s.deliverResumableWorkerFeedback(ctx, run, execution, answer, command.CommandID)
}
