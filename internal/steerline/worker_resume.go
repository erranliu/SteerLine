package steerline

import (
	"context"
	"encoding/json"
	"os"
)

// resume returns true only when the center should inspect another run.
func (s *Service) resumeCodexWorker(ctx context.Context, task *TaskRecord, run *Run) (bool, error) {
	_, execution := s.workflowRecords(run)
	if execution.PendingCommandID != "" || execution.CurrentTurnID != "" && execution.CurrentTurnID != execution.LastStoppedTurnID {
		// Active local turns were checked concurrently before this pass.
		return true, nil
	}
	reconciled, err := s.reconcileLocalWorkerPublication(ctx, run)
	if err != nil {
		return false, s.waitWorkerFlow(run, "Host publication needs reconciliation: "+err.Error())
	}
	if reconciled {
		return true, nil
	}
	if execution.RecoveryRequired {
		if execution.Process != nil {
			switch inspectProcess(execution.Process) {
			case "matching process still exists":
				execution.RecoveryState = "live"
			case "process absent":
				if execution.CurrentTurnID != "" && execution.CurrentTurnID == execution.LastStoppedTurnID {
					execution.RecoveryState = "completed"
				} else {
					execution.RecoveryState = "interrupted"
				}
			default:
				execution.RecoveryState = "unverified"
			}
			s.refreshRun(run)
			if err := s.commitWorker(run); err != nil {
				return true, err
			}
			if run.Decision != nil && run.Decision.Status == "answer_saved" {
				return true, s.refreshSavedRecoveryDecision(run, false, "服务重开后请确认旧答案；原轮次不会自动继续。")
			}
			if execution.RecoveryState == "completed" && run.Decision != nil && run.Decision.Status == "pending" {
				if record := s.decisionRecord(run); record != nil && (record.RecoveryVersion != execution.RecoveryVersion || record.RecoveryState != execution.RecoveryState) {
					oldID := record.DecisionID
					s.setDecisionStatus(run, "superseded")
					if err := s.commitWorker(run); err != nil {
						return true, err
					}
					s.setRunDecision(run, nil)
					if err := s.waitWorkerFlow(run, "原本地 worker 已退出，原轮次已结束。核对当前成果后选择原 thread 或明确使用新会话继续。"); err != nil {
						return true, err
					}
					if next := s.decisionRecord(run); next != nil {
						next.RecoveryOfDecision = oldID
						s.markWorkerDecisionWrite(next.DecisionID)
						return true, s.commitWorker(run)
					}
				}
			}
			return true, nil
		}
		if execution.Process == nil && execution.PendingCommandID == "" && execution.CurrentTurnID == "" {
			execution.RecoveryState = "interrupted"
			return true, s.waitWorkerFlow(run, "服务重开后，原 Codex worker 尚未开始执行。请核对工作区并手动继续。")
		}
		return true, nil
	}
	return s.resumeCodexWorkspace(ctx, task, run)
}

func (s *Service) resumeNativeOrcaWorker(ctx context.Context, task *TaskRecord, run *Run) (bool, error) {
	workflow, execution := s.workflowRecords(run)
	if execution.Environment != nil {
		if execution.Process != nil {
			if err := s.verifyNativeOrcaWorker(ctx, run, execution); err == nil {
				execution.RecoveryRequired = false
			}
		}
		if turn := nativeOrcaCurrentTurn(execution); turn != nil && turn.DispatchID != "" {
			return true, s.pollNativeOrcaWorker(ctx, run)
		}
		if execution.Environment.Blocker != nil {
			return true, s.observeWorkerEnvironment(ctx, run)
		}
		return true, s.advanceOrcaPreparation(ctx, run)
	}
	if execution.RecoveryRequired && execution.Process == nil {
		execution.RecoveryState = "unverified"
		return true, s.waitWorkerFlow(run, "原 Orca agent 创建状态未核实；重开服务不会自动准备或派发")
	}
	if execution.Process == nil {
		if execution.OrcaNative != nil {
			return false, s.nativeOrcaUncertain(run, "Original Orca agent creation has no confirmed terminal; preserve the worktree")
		}
		return false, s.prepareWorkerFlow(ctx, *task, run)
	}
	if execution.Process.Kind != "orca-native" {
		return false, s.nativeOrcaUncertain(run, "Old shell-based Orca execution cannot be resumed as a native Dispatch")
	}
	if turn := nativeOrcaCurrentTurn(execution); turn != nil && turn.DoneMessageID != "" && (execution.PendingTurnID == "" || turn.TurnID == execution.PendingTurnID) {
		// A delivered result can settle after the agent exits; the original
		// terminal, incarnation, worktree and Dispatch are still verified.
		return false, s.pollNativeOrcaWorker(ctx, run)
	}
	if err := s.verifyNativeOrcaWorker(ctx, run, execution); err != nil {
		execution.RecoveryState = "unverified"
		if execution.RecoveryRequired {
			return true, s.waitWorkerFlow(run, "Original Orca agent session is unavailable: "+err.Error())
		}
		return false, s.nativeOrcaUncertain(run, "Original Orca agent session is unavailable: "+err.Error())
	}
	if execution.RecoveryRequired {
		if execution.RecoveryState != "live" {
			execution.RecoveryState = "live"
			s.refreshRun(run)
			if err := s.commitWorker(run); err != nil {
				return true, err
			}
		}
		if run.Decision != nil && run.Decision.Status == "answer_saved" {
			if err := s.refreshSavedRecoveryDecision(run, run.Decision.Request.OrcaAsk != nil, "服务重开后保留了原 Orca 输入；请重新确认后才投递。 "); err != nil {
				return true, err
			}
		}
		if turn := nativeOrcaCurrentTurn(execution); turn != nil && turn.DispatchID != "" {
			return true, s.pollNativeOrcaWorker(ctx, run)
		}
		return true, s.waitWorkerFlow(run, "原 Orca agent 仍在；未确认的派发输入需人工核对后继续")
	}
	if execution.Target.OrchestrationRun == "" {
		if err := s.prepareNativeOrcaCoordinator(ctx, run, execution); err != nil {
			return false, s.nativeOrcaUncertain(run, "Original Orca Run needs reconciliation: "+err.Error())
		}
	}
	if execution.PendingTurnID != "" {
		turnID := execution.PendingTurnID
		op := s.operationByKey("worker-turn:" + execution.ExecutionID + ":" + turnID)
		if op == nil {
			return false, s.nativeOrcaUncertain(run, "Original Orca turn has no saved input intent")
		}
		var input struct {
			Prompt string `json:"prompt"`
		}
		if json.Unmarshal(op.Request, &input) != nil || input.Prompt == "" {
			return false, s.nativeOrcaUncertain(run, "Original Orca turn input cannot be restored")
		}
		if err := s.startNativeOrcaTurn(ctx, run, execution, turnID, input.Prompt); err != nil {
			return false, s.nativeOrcaUncertain(run, "Original Orca Task/Dispatch needs reconciliation: "+err.Error())
		}
		turn := nativeOrcaCurrentTurn(execution)
		if turn != nil && turn.DispatchID != "" && op.Status != "accepted" && op.Status != "confirmed" {
			if err := s.changeWorkerOperation(run, op.OperationID, "accepted", turn.DispatchRequestID, mustJSON(*turn), ""); err != nil {
				return false, err
			}
		}
		return false, s.pollNativeOrcaWorker(ctx, run)
	}
	if nativeOrcaCurrentTurn(execution) == nil && workflow.Phase == "queued" {
		return false, s.startInitialWorkerFlowTurn(ctx, *task, run)
	}
	return false, s.pollNativeOrcaWorker(ctx, run)
}

func (s *Service) resumeSupervisedOrcaWorker(ctx context.Context, task *TaskRecord, run *Run) (bool, error) {
	return s.resumeRemoteOrcaWorker(ctx, task, run)
}

func (s *Service) resumeCodexWorkspace(ctx context.Context, task *TaskRecord, run *Run) (bool, error) {
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return false, err
	}
	workflow, execution, c := fixed.workflow, fixed.execution, fixed.config
	if execution.RecoveryRequired && execution.Process == nil {
		execution.RecoveryState = "unverified"
		return true, s.waitWorkerFlow(run, "原 worker 尚无可核实的进程或 terminal；重开服务不会自动准备工作区或发起执行")
	}
	if execution.Process != nil && (execution.PendingTurnID != "" || execution.CurrentTurnID != execution.LastStoppedTurnID) {
		switch inspectProcess(execution.Process) {
		case "matching process still exists":
			return false, nil // A live original process cannot be restarted.
		case "process absent":
			return false, s.finishWorkerFlowFor(run, nil)
		default:
			execution.Status = "blocked"
			return false, s.waitWorkerFlow(run, "Original worker process identity cannot be verified; its session and files are retained")
		}
	}
	if workflow.Phase != "queued" {
		return true, nil
	}
	if execution.RecoveryRequired {
		return true, s.waitWorkerFlow(run, "原 worker 已停，等待手动继续")
	}
	var preparation *OperationRecord
	for i := range s.domain.Operations {
		if s.domain.Operations[i].IdempotencyKey == "worker-workspace:"+execution.ExecutionID {
			preparation = &s.domain.Operations[i]
			break
		}
	}
	if preparation == nil {
		return false, s.prepareWorkerFlow(ctx, *task, run)
	}
	if run.WorkspaceIntent == nil {
		return false, s.waitWorkerFlow(run, "Saved workspace preparation has no fixed worktree identity")
	}
	if _, err := os.Lstat(run.WorkspaceIntent.Path); os.IsNotExist(err) && preparation.Status != "confirmed" {
		if err := s.checkCurrentExecutionRequirements(ctx, fixed); err != nil {
			return false, err
		}
		if err := prepareWorktreeOnly(ctx, c, run); err != nil {
			return false, s.waitWorkerFlow(run, "Original workspace preparation cannot be reconciled: "+err.Error())
		}
	} else if err != nil {
		return false, s.waitWorkerFlow(run, "Original workspace cannot be inspected: "+err.Error())
	}
	s.refreshRun(run)
	workspace, err := s.inspectWorkflowWorkspace(ctx, run)
	if err != nil {
		return false, s.waitWorkerFlow(run, "Original workspace identity is unresolved: "+err.Error())
	}
	applyWorkflowWorkspaceEvidence(run, execution, workspace)
	if preparation.Status != "confirmed" {
		if err := s.changeWorkerOperation(run, preparation.OperationID, "confirmed", "", mustJSON(workspace), ""); err != nil {
			return false, err
		}
	}
	return false, s.startInitialWorkerFlowTurn(ctx, *task, run)
}
