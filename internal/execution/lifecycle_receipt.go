package execution

import (
	"errors"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

func (s *Service) newLifecycleOperation(run *reconstructiontypes.RunView, kind, key string, value any) (*executionmodel.OperationRecord, error) {
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
func (s *Service) originalLifecycleOperation(run *reconstructiontypes.RunView, kind, key string, value any) (*executionmodel.OperationRecord, bool, error) {
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

func checkedLifecycleRequest(snapshot executionbackend.LifecycleSnapshot, operation *executionmodel.OperationRecord, action executionbackend.LifecycleAction, input executionbackend.LifecycleInput) (executionbackend.LifecycleRequest, error) {
	if err := validateSavedLifecycleOperation(operation, action, input); err != nil {
		return executionbackend.LifecycleRequest{}, err
	}
	request := lifecycleRequest(snapshot, *operation, action, input)
	return request, request.Validate()
}

func (s *Service) checkedWorkerTurnReceipt(run *reconstructiontypes.RunView, request executionbackend.LifecycleRequest, receipt executionbackend.LifecycleReceipt, action executionbackend.LifecycleAction) (workerRecords, error) {
	if err := receipt.ValidateFor(request); err != nil {
		return workerRecords{}, err
	}
	if err := s.validateLifecycleResultBinding(run, request); err != nil {
		return workerRecords{}, err
	}
	fixed, err := s.workerRecords(run)
	if err != nil {
		return fixed, err
	}
	if request.Operation.Action != string(action) || request.Snapshot.Session.ExecutionID != fixed.execution.ExecutionID ||
		request.Snapshot.Session.AttemptID != fixed.execution.CurrentAttemptID || request.Snapshot.Session.Generation != fixed.execution.SessionGeneration ||
		request.Snapshot.ResourceGeneration != fixed.execution.Generation || request.Snapshot.ConfigurationDigest != fixed.execution.ConfigurationDigest ||
		request.Operation.PayloadDigest == "" || request.Input.Turn == nil || fixed.execution.PendingCommandID != request.Input.Turn.CommandID {
		return fixed, executionmodel.ErrTurnStale
	}
	return fixed, validateSavedLifecycleOperation(snapshotOperation(&s.domain, request.Operation.ID), action, request.Input)
}
