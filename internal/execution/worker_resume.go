package execution

import (
	"context"
	"os"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func (s *Service) resumeWorkerWorkspace(ctx context.Context, task *taskmodel.TaskRecord, run *reconstructiontypes.RunView) (bool, error) {
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
		lifecycle, _, err := s.lifecycleForRun(run)
		if err != nil {
			return false, err
		}
		snapshot, err := s.lifecycleSnapshot(run, fixed.workflow, fixed.execution, fixed.attempt, nil)
		if err != nil {
			return false, err
		}
		evidence, err := lifecycle.VerifyManualRecovery(ctx, snapshot)
		if err != nil {
			execution.RecoveryState = "unverified"
			return true, s.waitWorkerFlow(run, "Original worker identity cannot be verified: "+err.Error())
		}
		if evidence.ManagedProcess && !evidence.ProcessAbsent {
			return false, nil
		}
		if evidence.ProcessAbsent && evidence.WritersAbsent {
			return false, nil
		} // the original recovery owner reconciles its result
		execution.RecoveryState = "unverified"
		return true, s.waitWorkerFlow(run, "Original worker writers cannot be verified; the original session and files are retained")
	}
	if workflow.Phase != "queued" {
		return true, nil
	}
	if execution.RecoveryRequired {
		return true, s.waitWorkerFlow(run, "原 worker 已停，等待手动继续")
	}
	var preparation *executionmodel.OperationRecord
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
		if err := prepareWorktreeOnly(ctx, c, s.runtime.Directory, s.runtime.ProcessHost, run); err != nil {
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
