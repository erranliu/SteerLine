package steerline

import (
	"context"
	"errors"
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
