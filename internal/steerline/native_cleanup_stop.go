package steerline

import (
	"context"
	"errors"
	"os"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

func (s *Service) stopRunExecution(ctx context.Context, r *Run, worker *Worker) (executionapi.Operation, error) {
	if r == nil || worker == nil || worker.proc == nil {
		return executionapi.Operation{}, errors.New("there is no active execution to stop")
	}
	config, configErr := s.executionConfigForRun(r)
	if configErr != nil {
		return executionapi.Operation{}, configErr
	}
	records := executionRecords{root: config.RunDir, persistence: config.Persistence}
	ref, err := records.loadBinding(r.RunID, r.AttemptID)
	if err != nil {
		return executionapi.Operation{}, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "active execution has no valid fixed binding", Cause: err}
	}
	if ref.RunID != r.RunID || ref.AttemptID != r.AttemptID || r.Process == nil || ref.External != *r.Process {
		return executionapi.Operation{}, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "active execution binding does not match runtime identity"}
	}
	entry, registryErr := registrationFor(config)
	if registryErr != nil {
		return executionapi.Operation{}, registryErr
	}
	if entry.Key.Backend != ref.Backend || entry.Key.Protocol != ref.Protocol || entry.Key.Agent != ref.Agent.Name {
		return executionapi.Operation{}, &executionapi.BackendError{Code: executionapi.BackendIdentityMismatch, Diagnostic: "configured backend/protocol differs from fixed execution"}
	}
	stopper, stopperErr := executionStopperFor(config)
	if stopperErr != nil {
		return executionapi.Operation{}, stopperErr
	}
	opID := "stop-" + r.AttemptID
	payload := struct {
		Policy string
		Grace  string
	}{"fixed_local_execution_stop", config.ShutdownGrace}
	op := executionapi.Operation{Version: executionapi.ExecutionRecordVersion, ID: opID, RunID: r.RunID, AttemptID: r.AttemptID, Backend: ref.Backend, Protocol: ref.Protocol, Target: ref.Target, Action: "stop", PayloadDigest: digestValue(payload), Status: executionapi.OperationPending, StartedAt: now(), UpdatedAt: now(), Ref: &ref}
	if old, loadErr := records.loadOperation(r.RunID, r.AttemptID, opID); loadErr == nil {
		if old.Action != op.Action || old.PayloadDigest != op.PayloadDigest || old.Target != op.Target {
			return executionapi.Operation{}, errors.New("stop operation id conflicts with its fixed target/policy")
		}
		return old, nil // Never resend a persisted stop operation.
	} else if !os.IsNotExist(loadErr) {
		return executionapi.Operation{}, loadErr
	}
	if err = s.commitExecutionOperation(r, op, ""); err != nil {
		return executionapi.Operation{}, err
	}
	expected := recordFingerprint(op)
	op.Status, op.UpdatedAt = executionapi.OperationInFlight, now()
	if err = s.commitExecutionOperation(r, op, expected); err != nil {
		return executionapi.Operation{}, err
	}
	receipt, stopErr := stopper.StopExecution(ctx, ref, op, worker.proc)
	expected = recordFingerprint(op)
	if stopErr != nil {
		var be *executionapi.BackendError
		if errors.As(stopErr, &be) && (be.Code == executionapi.BackendOutcomeUnknown || be.Code == executionapi.BackendUnreachable || be.Code == executionapi.BackendIdentityMismatch) {
			op.Status = executionapi.OperationUnknown
		} else {
			op.Status = executionapi.OperationRejected
		}
		op.Diagnostic, op.UpdatedAt = stopErr.Error(), now()
		if saveErr := s.commitExecutionOperation(r, op, expected); saveErr != nil {
			return op, errors.Join(stopErr, saveErr)
		}
		return op, stopErr
	}
	switch receipt.Disposition {
	case "confirmed":
		op.Status = executionapi.OperationConfirmed
	case "accepted":
		op.Status = executionapi.OperationAccepted
	case "rejected":
		op.Status = executionapi.OperationRejected
	default:
		op.Status = executionapi.OperationUnknown
	}
	op.Diagnostic, op.UpdatedAt = receipt.Diagnostic, now()
	if err = s.commitExecutionOperation(r, op, expected); err != nil {
		return op, err
	}
	if op.Status == executionapi.OperationUnknown || op.Status == executionapi.OperationRejected {
		return op, &executionapi.BackendError{Code: executionapi.BackendOutcomeUnknown, Diagnostic: "stop request did not confirm an accepted stop"}
	}
	return op, nil
}

func (s *Service) confirmExecutionStop(op executionapi.Operation) (bool, error) {
	config, configErr := s.executionConfigForWorkflowID(op.RunID)
	if configErr != nil {
		return false, configErr
	}
	if op.ID == "" {
		return false, errors.New("stop operation is missing")
	}
	records := executionRecords{root: config.RunDir, persistence: config.Persistence}
	run := s.localWorkerRun(op.RunID)
	if run == nil {
		return false, errors.New("execution stop has no fixed workflow")
	}
	saved, readErr := records.loadOperation(op.RunID, op.AttemptID, op.ID)
	if readErr != nil {
		return false, readErr
	}
	expected := recordFingerprint(saved)
	// The adapter already persisted an exact stop receipt. A weaker later
	// observation must not erase durable close evidence.
	if op.Status == executionapi.OperationConfirmed {
		persisted, err := records.loadOperation(op.RunID, op.AttemptID, op.ID)
		if err != nil {
			return false, err
		}
		if persisted.Status != executionapi.OperationConfirmed || persisted.PayloadDigest != op.PayloadDigest || persisted.Target != op.Target || persisted.Ref == nil || op.Ref == nil || digestValue(*persisted.Ref) != digestValue(*op.Ref) {
			return false, &executionapi.BackendError{Code: executionapi.BackendOutcomeUnknown, Diagnostic: "exact stop confirmation was not durably saved"}
		}
		return true, nil
	}
	ref := op.Ref
	if ref == nil {
		loaded, err := (executionRecords{root: config.RunDir, persistence: config.Persistence}).loadBinding(op.RunID, op.AttemptID)
		if err != nil {
			return false, err
		}
		ref = &loaded
	}
	backend := backendFor(config)
	obs, err := backend.Observe(context.Background(), *ref)
	if err != nil {
		op.Status, op.Diagnostic, op.UpdatedAt = executionapi.OperationUnknown, err.Error(), now()
		saveErr := s.commitExecutionOperation(run, op, expected)
		return false, errors.Join(err, saveErr)
	}
	if obs.Liveness != "exited" {
		op.Status, op.Diagnostic, op.UpdatedAt = executionapi.OperationUnknown, "original execution has not produced stopped evidence: "+obs.Diagnostic, now()
		saveErr := s.commitExecutionOperation(run, op, expected)
		return false, errors.Join(&executionapi.BackendError{Code: executionapi.BackendOutcomeUnknown, Diagnostic: op.Diagnostic}, saveErr)
	}
	op.Status, op.Diagnostic, op.UpdatedAt = executionapi.OperationConfirmed, obs.Diagnostic, now()
	if err = s.commitExecutionOperation(run, op, expected); err != nil {
		return false, err
	}
	return true, nil
}
