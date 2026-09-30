package steerline

import (
	"context"
	"errors"
	"github.com/erranliu/SteerLine/internal/steerline/workerflow"
)

func (s *Service) waitWorkerFlow(run *Run, reason string, operationIDs ...string) error {
	workflow, execution := s.workflowRecords(run)
	if workflow == nil || execution == nil || workflow.WorkflowVersion != WorkerFlowVersion {
		return errors.New("worker flow wait has no fixed execution")
	}
	if workflow.Phase == "completed" || run.TaskStatus == "closed" || run.TaskStatus == "discarded" {
		return nil
	}
	unresolvedNativeTurn := run.Protocol == string(ProtocolLocalCodexAppServer) && !execution.RecoveryRequired && (execution.PendingCommandID != "" || execution.CurrentTurnID != "" && execution.CurrentTurnID != execution.LastStoppedTurnID)
	if !unresolvedNativeTurn && (run.Decision == nil || run.Decision.Status != "pending" && run.Decision.Status != "answer_saved") {
		contextText := reason
		if run.Protocol == string(ProtocolLocalCodexAppServer) && execution.CurrentTurnID != "" && execution.CurrentTurnID == execution.LastStoppedTurnID {
			contextText = "本轮已结束，提交后将在原 Codex 会话开始下一轮。\n\n" + reason
		}
		options := []DecisionOption{{ID: "continue", Label: "Continue current work"}}
		if execution.RecoveryRequired && (execution.RecoveryState == "interrupted" || execution.RecoveryState == "completed") && run.Protocol == string(ProtocolLocalCodexAppServer) && execution.Process != nil {
			options = []DecisionOption{{ID: "continue", Label: "原 Codex 会话继续", Detail: "在新的本地进程中核实并恢复原 thread"}, {ID: "new_session", Label: "新会话继续", Detail: "使用现有工作区和持久历史作为上下文"}}
		} else if execution.RecoveryRequired && execution.RecoveryState == "interrupted" && run.Protocol == string(ProtocolLocalCodexAppServer) {
			options = []DecisionOption{{ID: "continue", Label: "开始本地 Codex 执行", Detail: "先核实或准备原任务的工作区"}}
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
	if execution.Status == "active" && run.Protocol != string(ProtocolLocalCodexAppServer) {
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
	route, err := workerFlowModeFor(config, run, execution)
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
	if route == workerflow.NativeOrca && run.Decision.Request.HostTarget != nil {
		return s.applyEnvironmentAction(ctx, run, answer, command.CommandID)
	}
	if route == workerflow.NativeOrca && execution.Environment != nil {
		if err := s.observeWorkerEnvironment(ctx, run); err != nil {
			return err
		}
		if execution.Environment.Blocker != nil {
			return nil
		}
		if !execution.Environment.Ready {
			return s.advanceOrcaPreparation(ctx, run)
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
		if route == workerflow.NativeOrca && execution.RecoveryState == "relay_pending" {
			if answer.OptionID != "forward_result" {
				return errors.New("Orca result relay requires explicit forwarding approval")
			}
			s.setDecisionStatus(run, "applied")
			s.setRunDecision(run, nil)
			execution.RecoveryRequired = false
			execution.RecoveryState = "running"
			if err := s.commitWorker(run); err != nil {
				return err
			}
			turn := nativeOrcaCurrentTurn(execution)
			if turn == nil || turn.DispatchID == "" {
				return errors.New("original Orca Dispatch disappeared before relay")
			}
			if err := s.relayNativeOrcaDone(ctx, run, execution, turn); err != nil {
				return s.nativeOrcaUncertain(run, "Manual Orca result relay needs reconciliation: "+err.Error())
			}
			return nil // The original inbox will prove delivery on a later poll.
		}
		if route == workerflow.LocalCodex && execution.RecoveryState == "interrupted" && execution.Process == nil && execution.TurnNumber == 0 {
			return s.startManuallyRecoveredInitialCodex(ctx, run, answer, command.CommandID)
		}
		if route == workerflow.LocalCodex && (execution.RecoveryState == "interrupted" || execution.RecoveryState == "completed") {
			prompt := "The previous local Codex process stopped. Inspect the current worktree, Git status, existing work and persistent task history before continuing. Do not replay an earlier tool call blindly.\n\n" + workerFeedbackText(answer, run.Decision.Request)
			if err := s.beginLocalRecoveryAttempt(ctx, run, answer.OptionID); err != nil {
				return s.waitWorkerFlow(run, "手动继续条件未满足："+err.Error())
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
	if route == workerflow.NativeOrca {
		return s.applyNativeOrcaDecision(ctx, run, workflow, execution, answer, command.CommandID)
	}
	return s.deliverResumableWorkerFeedback(ctx, run, execution, answer, command.CommandID, route)
}
