package steerline

import (
	"context"
	"encoding/json"
	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	"os"
	"path/filepath"

	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
	"github.com/erranliu/SteerLine/internal/steerline/workerflow"
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
	return s.resumeWorkspaceWorker(ctx, task, run, false)
}

func (s *Service) resumeNativeOrcaWorker(ctx context.Context, task *TaskRecord, run *Run) (bool, error) {
	workflow, execution := s.workflowRecords(run)
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
		return false, s.nativeOrcaUncertain(run, "Original Orca agent session is unavailable: "+err.Error())
	}
	if execution.Target.OrchestrationRun == "" {
		if err := s.prepareNativeOrcaCoordinator(ctx, run, execution); err != nil {
			return false, s.nativeOrcaUncertain(run, "Original Orca Run needs reconciliation: "+err.Error())
		}
	}
	if execution.PendingTurnID != "" {
		turnID := execution.PendingTurnID
		op := s.nativeOrcaOperation("worker-turn:" + execution.ExecutionID + ":" + turnID)
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
			if err := s.changeOperation(op.OperationID, "accepted", turn.DispatchRequestID, mustJSON(*turn), ""); err != nil {
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
	return s.resumeWorkspaceWorker(ctx, task, run, true)
}

func (s *Service) resumeWorkspaceWorker(ctx context.Context, task *TaskRecord, run *Run, remote bool) (bool, error) {
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return false, err
	}
	workflow, execution, c := fixed.workflow, fixed.execution, fixed.config
	if execution.Process != nil && execution.Process.Kind == "orca" && (execution.PendingTurnID != "" || execution.CurrentTurnID != execution.LastStoppedTurnID) {
		turnID := execution.PendingTurnID
		if turnID == "" {
			turnID = execution.CurrentTurnID
		}
		events := filepath.Join(orcaTurnPath(c, run.RunID, turnID), "events.jsonl")
		if exited, err := orcaworker.ReadTurnExit(events, turnID); err != nil {
			return false, s.waitWorkerFlow(run, "Original Orca turn exit evidence is invalid: "+err.Error())
		} else if exited != nil {
			return false, s.finishWorkerFlowFor(run, nil)
		}
		term, err := showWorkerOrcaTerminal(ctx, c, execution.Process.Terminal)
		if err != nil {
			return false, s.waitWorkerFlow(run, "Original Orca terminal is unavailable: "+err.Error())
		}
		if err := verifyWorkerOrcaTerminal(term, run, execution.Process); err != nil {
			return false, s.waitWorkerFlow(run, err.Error())
		}
		worker, err := resumeWorkerOrcaTurn(c, run, turnID)
		if err != nil {
			return false, s.waitWorkerFlow(run, "Original Orca turn journal cannot be reattached: "+err.Error())
		}
		s.registerWorker(run, worker)
		return false, nil
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
	if execution.PendingTurnID != "" {
		events := filepath.Join(c.RunDir, "runs", run.RunID, "turns", execution.PendingTurnID, "events.jsonl")
		if outcome, err := codexworker.ReadExecOutcome(events); err == nil && outcome.Completed && outcome.SessionID != "" {
			return false, s.finishWorkerFlowFor(run, nil)
		}
		if workflow.Phase != "waiting_decision" {
			return false, s.waitWorkerFlow(run, "Worker turn start has no confirmed process receipt; original request remains unverified")
		}
		return false, nil
	}
	if workflow.Phase != "queued" {
		return true, nil
	}
	var preparation *OperationRecord
	for i := range s.State.Operations {
		if s.State.Operations[i].IdempotencyKey == "worker-workspace:"+execution.ExecutionID {
			preparation = &s.State.Operations[i]
			break
		}
	}
	if preparation == nil {
		return false, s.prepareWorkerFlow(ctx, *task, run)
	}
	if run.WorkspaceIntent == nil {
		return false, s.waitWorkerFlow(run, "Saved workspace preparation has no fixed worktree identity")
	}
	if remote {
		if execution.Workspace.Worktree == "" {
			if err := s.recoverRemoteWorkerWorkspace(ctx, run, execution, preparation); err != nil {
				return false, s.waitWorkerFlow(run, "Original remote worktree cannot be reconciled: "+err.Error())
			}
		} else {
			s.refreshRun(run)
			if preparation.Status != "confirmed" {
				if err := s.changeOperation(preparation.OperationID, "confirmed", preparation.PlatformRequestID, mustJSON(execution.Workspace), ""); err != nil {
					return false, err
				}
			}
		}
	} else {
		if _, err := os.Lstat(run.WorkspaceIntent.Path); os.IsNotExist(err) && preparation.Status != "confirmed" {
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
			if err := s.changeOperation(preparation.OperationID, "confirmed", "", mustJSON(workspace), ""); err != nil {
				return false, err
			}
		}
	}
	if remote {
		if err := s.prepareWorkerOrcaTerminal(ctx, run); err != nil {
			return false, s.waitWorkerFlow(run, "Original Orca terminal cannot be reconciled: "+err.Error())
		}
	}
	return false, s.startInitialWorkerFlowTurn(ctx, *task, run)
}

func (s *Service) resumeWorkerFlowByMode(ctx context.Context, task *TaskRecord, run *Run, mode workerflow.Mode) (bool, error) {
	switch mode {
	case workerflow.LocalCodex:
		return s.resumeCodexWorker(ctx, task, run)
	case workerflow.NativeOrca:
		_, err := s.resumeNativeOrcaWorker(ctx, task, run)
		return true, err // Poll every active Orca Run, not only the first one.
	case workerflow.SupervisedOrca:
		return s.resumeSupervisedOrcaWorker(ctx, task, run)
	default:
		return false, nativeErr("unsupported_schema", "unknown worker mode")
	}
}
