package steerline

import (
	"context"
	"errors"
	"os"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

func prepareOrcaFeedback(s *Service, ctx context.Context, run *Run, execution *ExecutionRecord, answer DecisionAnswer, commandID string) (bool, error) {
	if run.Decision.Request.HostTarget != nil {
		return true, s.applyEnvironmentAction(ctx, run, answer, commandID)
	}
	if execution.Environment != nil {
		if err := s.observeWorkerEnvironment(ctx, run); err != nil {
			return true, err
		}
		if execution.Environment.Blocker != nil {
			return true, nil
		}
		if !execution.Environment.Ready {
			return true, s.advanceOrcaPreparation(ctx, run)
		}
	}

	return false, nil
}
func recoverOrcaFeedback(s *Service, ctx context.Context, run *Run, execution *ExecutionRecord, answer DecisionAnswer, commandID string) (bool, error) {
	if execution.RecoveryState == "relay_pending" {
		if answer.OptionID != "forward_result" {
			return true, errors.New("Orca result relay requires explicit forwarding approval")
		}
		s.setDecisionStatus(run, "applied")
		s.setRunDecision(run, nil)
		execution.RecoveryRequired = false
		execution.RecoveryState = "running"
		if err := s.commitWorker(run); err != nil {
			return true, err
		}
		turn := nativeOrcaCurrentTurn(execution)
		if turn == nil || turn.DispatchID == "" {
			return true, errors.New("original Orca Dispatch disappeared before relay")
		}
		if err := s.relayNativeOrcaDone(ctx, run, execution, turn); err != nil {
			return true, s.nativeOrcaUncertain(run, "Manual Orca result relay needs reconciliation: "+err.Error())
		}
		return true, nil // The original inbox will prove delivery on a later poll.
	}

	return false, nil
}

func reconcileNativeResultRelay(s *Service, ctx context.Context, run *Run, execution *ExecutionRecord, turn *OrcaNativeTurn) (bool, error) {
	workflow := s.workflowRecord(run)
	if turn.DoneMessageID == "" {
		if execution.RecoveryRequired {
			path, pathErr := nativeOrcaRelayPath(execution.Workspace.Worktree, run.RunID, turn.TurnID)
			if pathErr != nil {
				return true, s.nativeOrcaUncertain(run, "Orca result relay path cannot be verified: "+pathErr.Error())
			}
			if _, statErr := os.Stat(path); statErr == nil {
				request, readErr := readNativeOrcaDoneRequest(path, execution.Workspace.Worktree, run, turn)
				if readErr != nil || request == nil {
					return true, s.nativeOrcaUncertain(run, "Orca result relay request cannot be verified")
				}
				execution.RecoveryState = "relay_pending"
				if run.Decision == nil || len(run.Decision.Request.Options) != 1 || run.Decision.Request.Options[0].ID != "forward_result" {
					oldDecisionID := ""
					contextText := "原 worker 已写入结果请求。请核对原 Dispatch、结果与当前工作区后决定是否转发。"
					if run.Decision != nil && (run.Decision.Status == "pending" || run.Decision.Status == "answer_saved") {
						oldDecisionID = run.Decision.ID
						contextText += "\n\n原待答输入：" + run.Decision.Request.Question + "\n" + run.Decision.Request.Context
						s.setDecisionStatus(run, "superseded")
						if err := s.commitWorker(run); err != nil {
							return true, err
						}
						s.setRunDecision(run, nil)
					}
					decision, err := WriteDecisionRequest(run, DecisionRequest{Question: "Forward the original Orca worker result?", Context: clip(contextText, 7000), Options: []executionapi.DecisionOption{{ID: "forward_result", Label: "转发原结果"}}, AllowFreeText: true})
					if err != nil {
						return true, err
					}
					s.setRunDecision(run, decision)
					if record := s.decisionRecord(run); record != nil {
						record.RecoveryOfDecision = oldDecisionID
					}
				}
				workflow.Phase, workflow.UpdatedAt = "waiting_decision", now()
				s.refreshRun(run)
				return true, s.commitWorker(run)
			} else if !os.IsNotExist(statErr) {
				return true, s.nativeOrcaUncertain(run, "Orca result relay file cannot be inspected: "+statErr.Error())
			}
			return true, nil // Restart observation never sends a new worker_done.
		}
		if err := s.relayNativeOrcaDone(ctx, run, execution, turn); err != nil {
			if ctx.Err() != nil {
				return true, nil // Service shutdown is not a worker result failure.
			}
			return true, s.nativeOrcaUncertain(run, "Orca worker result relay needs reconciliation: "+err.Error())
		}
	}

	return false, nil
}
func clearNativeResultRelay(e *ExecutionRecord, run *Run, t *OrcaNativeTurn) {
	removeNativeOrcaRelayRequest(e.Workspace.Worktree, run.RunID, t.TurnID)
}
