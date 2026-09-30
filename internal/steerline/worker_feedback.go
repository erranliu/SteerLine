package steerline

import (
	"context"
	"errors"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

func (s *Service) waitWorkerFlow(run *Run, reason string, operationIDs ...string) error {
	workflow, execution := s.workflowRecords(run)
	if workflow == nil || execution == nil || workflow.WorkflowVersion != WorkerFlowVersion {
		return errors.New("worker flow wait has no fixed execution")
	}
	if workflow.Phase == "completed" || run.TaskStatus == "closed" || run.TaskStatus == "discarded" {
		return nil
	}
	entry, err := s.workerFlowRegistration(run)
	if err != nil {
		return err
	}
	facts := workerWaitFacts{}
	if entry.WaitFacts != nil {
		facts = entry.WaitFacts(run, execution)
	}
	unresolvedNativeTurn := facts.PreserveUnresolvedTurn
	if !unresolvedNativeTurn && (run.Decision == nil || run.Decision.Status != "pending" && run.Decision.Status != "answer_saved") {
		contextText := facts.ContextPrefix + reason
		options := []executionapi.DecisionOption{{ID: "continue", Label: "Continue current work"}}
		if facts.Options != nil {
			options = facts.Options
		}
		question := "The worker stopped. What should it do next?"
		if execution.RecoveryRequired && (execution.RecoveryState == "unverified" || execution.RecoveryState == "") {
			question = "Worker state is unverified. Review the original execution evidence."
			options = nil
		}
		request := DecisionRequest{Question: question, Context: clip(contextText, 7000), Options: options, AllowFreeText: true}
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

func (s *Service) applyWorkerFlowDecision(ctx context.Context, run *Run) error {
	fixed, contextErr := s.workerFlowContext(run)
	if contextErr != nil {
		return contextErr
	}
	c := fixed.config
	if run == nil || run.WorkflowVersion != WorkerFlowVersion || run.Decision == nil || run.Decision.Status != "answer_saved" {
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
	route, err := workerFlowRegistrationFor(config, run, execution)
	if err != nil {
		return err
	}
	answer, err := ReadDecisionAnswer(run)
	if err != nil {
		return err
	}
	command, err := s.domainStore.CommandByIdempotency(ctx, c.identity, decisionAnswerKey(answer))
	if err != nil || command.Kind != "answer_decision" || command.TargetID != answer.DecisionID || command.Status != "accepted" || !stableExecutionID(command.CommandID) {
		return errors.Join(err, errors.New("saved worker feedback has no fixed accepted command identity"))
	}
	if err := s.checkCurrentExecutionRequirements(ctx, fixed); err != nil {
		return err
	}
	if route.PrepareFeedback != nil {
		handled, err := route.PrepareFeedback(s, ctx, run, execution, answer, command.CommandID)
		if handled || err != nil {
			return err
		}
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
		if route.RecoverFeedback != nil {
			handled, err := route.RecoverFeedback(s, ctx, run, execution, answer, command.CommandID)
			if handled || err != nil {
				return err
			}
		}
		if execution.RecoveryState != "live" && execution.RecoveryState != "running" && execution.RecoveryState != "question" {
			return nil
		}
		execution.RecoveryRequired = false
		if err := s.commitWorker(run); err != nil {
			return err
		}
	}
	return route.DeliverFeedback(s, ctx, run, workflow, execution, answer, command.CommandID)
}
