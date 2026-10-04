package steerline

import (
	"errors"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

// newLifecycleOperation publishes the intent before returning its original row.
func (s *Service) newLifecycleOperation(run *Run, kind, key string, value any) (*OperationRecord, error) {
	id, err := s.startWorkerOperation(run, kind, key, value)
	if err != nil {
		return nil, err
	}
	operation := s.operationByKey(key)
	if operation == nil || operation.OperationID != id || operation.Kind != kind {
		return nil, errors.New("lifecycle intent lost its original identity")
	}
	return operation, nil
}

// originalLifecycleOperation never offers an existing external intent to Execute.
func (s *Service) originalLifecycleOperation(run *Run, kind, key string, value any) (*OperationRecord, bool, error) {
	prior := s.operationByKey(key)
	if prior != nil && prior.Status != "pending" {
		if prior.Kind != kind {
			return nil, true, errors.New("lifecycle intent kind differs from its original request")
		}
		return prior, true, nil
	}
	operation, err := s.newLifecycleOperation(run, kind, key, value)
	return operation, prior != nil, err
}

func checkedLifecycleRequest(snapshot executionapi.LifecycleSnapshot, operation *OperationRecord, action executionapi.LifecycleAction, input executionapi.LifecycleInput) (executionapi.LifecycleRequest, error) {
	if err := validateSavedLifecycleOperation(operation, action, input); err != nil {
		return executionapi.LifecycleRequest{}, err
	}
	request := lifecycleRequest(snapshot, *operation, action, input)
	return request, request.Validate()
}

// lifecycleOutcome only normalizes the external result. Each action still owns
// what counts as accepted/confirmed and which business transition may follow.
func lifecycleOutcome(receipt executionapi.LifecycleReceipt, err error) (string, string) {
	status, diagnostic := string(receipt.Disposition), receipt.Diagnostic
	if err != nil {
		diagnostic = err.Error()
		if receipt.Disposition != executionapi.OperationRejected {
			status = "outcome_unknown"
		}
	}
	return status, diagnostic
}

func (s *Service) checkedWorkerTurnReceipt(run *Run, request executionapi.LifecycleRequest, receipt executionapi.LifecycleReceipt, action executionapi.LifecycleAction) (workerFlowContext, error) {
	if err := receipt.ValidateFor(request); err != nil {
		return workerFlowContext{}, err
	}
	if err := s.validateLifecycleResultBinding(run, request); err != nil {
		return workerFlowContext{}, err
	}
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return fixed, err
	}
	if request.Operation.Action != string(action) || request.Snapshot.Session.ExecutionID != fixed.execution.ExecutionID ||
		request.Snapshot.Session.AttemptID != fixed.execution.CurrentAttemptID || request.Snapshot.Session.Generation != fixed.execution.SessionGeneration ||
		request.Snapshot.ResourceGeneration != fixed.execution.Generation || request.Snapshot.ConfigurationDigest != fixed.execution.ConfigurationDigest ||
		request.Operation.PayloadDigest == "" || request.Input.Turn == nil || fixed.execution.PendingCommandID != request.Input.Turn.CommandID {
		return fixed, ErrWorkerTurnStale
	}
	return fixed, validateSavedLifecycleOperation(snapshotOperation(&s.domain, request.Operation.ID), action, request.Input)
}
