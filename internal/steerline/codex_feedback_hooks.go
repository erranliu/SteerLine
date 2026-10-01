package steerline

import (
	"context"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

func deliverCodexFeedback(s *Service, ctx context.Context, run *Run, _ *WorkflowRunRecord, execution *ExecutionRecord, answer DecisionAnswer, commandID string) error {
	return s.deliverResumableWorkerFeedback(ctx, run, execution, answer, commandID)
}
func recoverCodexFeedback(s *Service, ctx context.Context, run *Run, execution *ExecutionRecord, answer DecisionAnswer, commandID string) (bool, error) {
	if execution.RecoveryState == "interrupted" && execution.Process == nil && execution.TurnNumber == 0 {
		return true, s.startManuallyRecoveredInitialCodex(ctx, run, answer, commandID)
	}
	if execution.RecoveryState == "interrupted" || execution.RecoveryState == "completed" {
		prompt := "The previous local Codex process stopped. Inspect the current worktree, Git status, existing work and persistent task history before continuing. Do not replay an earlier tool call blindly.\n\n" + workerFeedbackText(answer, run.Decision.Request)
		if err := s.beginLocalRecoveryAttempt(ctx, run, answer.OptionID); err != nil {
			return true, s.waitWorkerFlow(run, "手动继续条件未满足："+err.Error())
		}
		return true, s.startWorkerFlowTurn(ctx, run, prompt, commandID)
	}

	return false, nil
}

func codexWorkerWaitFacts(_ *Run, e *ExecutionRecord) workerWaitFacts {
	facts := workerWaitFacts{PreserveActiveStatus: true, PreserveUnresolvedTurn: !e.RecoveryRequired && (e.PendingCommandID != "" || e.CurrentTurnID != "" && e.CurrentTurnID != e.LastStoppedTurnID)}
	if e.CurrentTurnID != "" && e.CurrentTurnID == e.LastStoppedTurnID {
		facts.ContextPrefix = "本轮已结束，提交后将在原 Codex 会话开始下一轮。\n\n"
	}
	if e.RecoveryRequired && (e.RecoveryState == "interrupted" || e.RecoveryState == "completed") && e.Process != nil {
		facts.Options = []executionapi.DecisionOption{{ID: "continue", Label: "原 Codex 会话继续", Detail: "在新的本地进程中核实并恢复原 thread"}, {ID: "new_session", Label: "新会话继续", Detail: "使用现有工作区和持久历史作为上下文"}}
	} else if e.RecoveryRequired && e.RecoveryState == "interrupted" {
		facts.Options = []executionapi.DecisionOption{{ID: "continue", Label: "开始本地 Codex 执行", Detail: "先核实或准备原任务的工作区"}}
	}
	return facts
}
func settleCodexRequests(s *Service, run *Run, workflow *WorkflowRunRecord, execution *ExecutionRecord, evidence workerTurnEvidence) (bool, error) {
	execution.PendingQuestions = nil // Requests expire with this completed turn.
	if run.Decision != nil && run.Decision.Request.QuestionRoute != nil {
		if run.Decision.Status == "pending" {
			s.setDecisionStatus(run, "superseded")
			if err := s.commitWorker(run); err != nil {
				return true, err
			}
			workflow, execution = s.workflowRecords(run)
			s.setRunDecision(run, nil)
		} else if run.Decision.Status == "answer_saved" {
			if err := StopWorkerTurn(workflow, execution, evidence.TurnID, true); err != nil {
				return true, err
			}
			execution.TurnEndedAt = ptr(now())
			execution.Status = "blocked"
			s.refreshRun(run)
			execution.Error = ptr("Native turn ended before the original answer delivery was confirmed; the saved answer and request are retained")
			s.refreshRun(run)
			return true, s.commitWorker(run)
		}
	}

	return false, nil
}
