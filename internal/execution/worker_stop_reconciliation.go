package execution

import (
	"context"
	"errors"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
)

// Only the worker interprets the original stop request. The application stores
// its standard receipt and never re-executes an uncertain stop.
func (s *Service) reconcileWorkerStops(ctx context.Context, run *reconstructiontypes.RunView, lifecycle executionbackend.LifecyclePort) (bool, error) {
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return false, err
	}
	snapshot, err := s.lifecycleSnapshot(run, fixed.workflow, fixed.execution, fixed.attempt, nil)
	if err != nil {
		return false, err
	}
	for _, operation := range snapshot.Operations {
		if operation.Action != string(executionbackend.LifecycleStopObservation) || operation.Status != executionbackend.OperationInFlight && operation.Status != executionbackend.OperationUnknown && operation.Status != executionbackend.OperationAccepted {
			continue
		}
		candidate, err := executionbackend.CandidateForOperation(operation)
		if err != nil {
			return true, err
		}
		request := executionbackend.LifecycleRequest{Snapshot: snapshot, Operation: operation, Input: candidate.Input}
		err = s.dispatchLifecycleEffect(ctx, run, lifecycle, request, true, func(run *reconstructiontypes.RunView, result lifecycleEffectResult) error {
			if err := result.receipt.ValidateFor(request); err != nil {
				return err
			}
			if result.err != nil || result.receipt.Disposition != executionbackend.OperationConfirmed {
				s.executionRecord(run).RecoveryState = "unverified"
				return s.waitWorkerFlow(run, "Original stop operation "+operation.ID+" remains unverified; no recovery input was sent")
			}
			if err := s.setOperationState(operation.ID, "confirmed", result.receipt.RequestID, mustJSON(result.receipt), result.receipt.Diagnostic); err != nil {
				return err
			}
			return s.commitWorker(run, operation.ID)
		})
		if errors.Is(err, errLifecycleEffectPending) {
			return true, nil
		}
		return true, err
	}
	return false, nil
}
