package steerline

import (
	"context"
	"errors"
	"os"
)

func (s *Service) stopRunExecution(ctx context.Context, r *Run, worker *Worker) (Operation, error) {
	if r == nil || worker == nil {
		return Operation{}, errors.New("there is no active execution to stop")
	}
	config, configErr := s.executionConfigForRun(r)
	if configErr != nil {
		return Operation{}, configErr
	}
	records := executionRecords{root: config.RunDir}
	ref, err := records.loadBinding(r.RunID, r.AttemptID)
	if err != nil {
		return Operation{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "active execution has no valid fixed binding", Cause: err}
	}
	if ref.RunID != r.RunID || ref.AttemptID != r.AttemptID || r.Process == nil || ref.External != *r.Process {
		return Operation{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "active execution binding does not match runtime identity"}
	}
	backend := backendFor(config)
	kind, protocol := backend.Identity()
	if kind != ref.Backend || protocol != ref.Protocol {
		return Operation{}, &BackendError{Code: BackendIdentityMismatch, Diagnostic: "configured backend/protocol differs from fixed execution"}
	}
	opID := "stop-" + r.AttemptID
	payload := struct {
		Policy string
		Grace  string
	}{"fixed_local_execution_stop", config.ShutdownGrace}
	op := Operation{Version: ExecutionRecordVersion, ID: opID, RunID: r.RunID, AttemptID: r.AttemptID, Backend: ref.Backend, Protocol: ref.Protocol, Target: ref.Target, Action: "stop", PayloadDigest: digestValue(payload), Status: OperationPending, StartedAt: now(), UpdatedAt: now(), Ref: &ref}
	if old, loadErr := records.loadOperation(r.RunID, r.AttemptID, opID); loadErr == nil {
		if old.Action != op.Action || old.PayloadDigest != op.PayloadDigest || old.Target != op.Target {
			return Operation{}, errors.New("stop operation id conflicts with its fixed target/policy")
		}
		return old, nil // Never resend a persisted stop operation.
	} else if !os.IsNotExist(loadErr) {
		return Operation{}, loadErr
	}
	if err = records.saveOperation(op); err != nil {
		return Operation{}, err
	}
	op.Status, op.UpdatedAt = OperationInFlight, now()
	if err = records.saveOperation(op); err != nil {
		return Operation{}, err
	}
	receipt, stopErr := backend.StopExecution(ctx, ref, op, worker)
	if stopErr != nil {
		var be *BackendError
		if errors.As(stopErr, &be) && (be.Code == BackendOutcomeUnknown || be.Code == BackendUnreachable || be.Code == BackendIdentityMismatch) {
			op.Status = OperationUnknown
		} else {
			op.Status = OperationRejected
		}
		op.Diagnostic, op.UpdatedAt = stopErr.Error(), now()
		if saveErr := records.saveOperation(op); saveErr != nil {
			return op, errors.Join(stopErr, saveErr)
		}
		return op, stopErr
	}
	switch receipt.Disposition {
	case "confirmed":
		op.Status = OperationConfirmed
	case "accepted":
		op.Status = OperationAccepted
	case "rejected":
		op.Status = OperationRejected
	default:
		op.Status = OperationUnknown
	}
	op.Diagnostic, op.UpdatedAt = receipt.Diagnostic, now()
	if err = records.saveOperation(op); err != nil {
		return op, err
	}
	if op.Status == OperationUnknown || op.Status == OperationRejected {
		return op, &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "stop request did not confirm an accepted stop"}
	}
	return op, nil
}

func (s *Service) confirmExecutionStop(op Operation) (bool, error) {
	config, configErr := s.executionConfigForWorkflowID(op.RunID)
	if configErr != nil {
		return false, configErr
	}
	if op.ID == "" {
		return false, errors.New("stop operation is missing")
	}
	records := executionRecords{root: config.RunDir}
	// The adapter already persisted an exact stop receipt. Orca's terminal show
	// may later report an orphaned/disconnected tab even after ptyKilled=true;
	// that weaker observation must not erase the durable close evidence.
	if op.Status == OperationConfirmed {
		persisted, err := records.loadOperation(op.RunID, op.AttemptID, op.ID)
		if err != nil {
			return false, err
		}
		if persisted.Status != OperationConfirmed || persisted.PayloadDigest != op.PayloadDigest || persisted.Target != op.Target || persisted.Ref == nil || op.Ref == nil || digestValue(*persisted.Ref) != digestValue(*op.Ref) {
			return false, &BackendError{Code: BackendOutcomeUnknown, Diagnostic: "exact stop confirmation was not durably saved"}
		}
		return true, nil
	}
	ref := op.Ref
	if ref == nil {
		loaded, err := (executionRecords{root: config.RunDir}).loadBinding(op.RunID, op.AttemptID)
		if err != nil {
			return false, err
		}
		ref = &loaded
	}
	backend := backendFor(config)
	obs, err := backend.Observe(context.Background(), *ref)
	if err != nil {
		op.Status, op.Diagnostic, op.UpdatedAt = OperationUnknown, err.Error(), now()
		saveErr := records.saveOperation(op)
		return false, errors.Join(err, saveErr)
	}
	if obs.Liveness != "exited" {
		op.Status, op.Diagnostic, op.UpdatedAt = OperationUnknown, "original execution has not produced stopped evidence: "+obs.Diagnostic, now()
		saveErr := records.saveOperation(op)
		return false, errors.Join(&BackendError{Code: BackendOutcomeUnknown, Diagnostic: op.Diagnostic}, saveErr)
	}
	op.Status, op.Diagnostic, op.UpdatedAt = OperationConfirmed, obs.Diagnostic, now()
	if err = records.saveOperation(op); err != nil {
		return false, err
	}
	return true, nil
}
