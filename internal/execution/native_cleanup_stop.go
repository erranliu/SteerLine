package execution

import (
	"context"
	"errors"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
	"github.com/erranliu/SteerLine/internal/execution/reliable"
	"os"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

type stopReconciliationPort struct {
	executionbackend.LifecyclePort
	backend executionbackend.ExecutionBackend
	ref     executionbackend.ExecutionRef
}

func (p stopReconciliationPort) Reconcile(ctx context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, executionbackend.LifecycleObserver, error) {
	observation, err := p.backend.Observe(ctx, p.ref)
	status := executionbackend.OperationUnknown
	if err == nil && observation.Liveness == "exited" {
		status = executionbackend.OperationConfirmed
	}
	return executionbackend.LifecycleReceipt{Operation: request.Operation, Disposition: status, Diagnostic: observation.Diagnostic}, nil, err
}

// Observe the original stop request through the shared background channel;
// reconciliation cannot repeat Kill or occupy a project control checkpoint.
func (s *Service) reconcileRunStop(ctx context.Context, run *reconstructiontypes.RunView, op executionbackend.Operation) error {
	if op.Ref == nil || s.lifecycleEffectPending(run.RunID) {
		return nil
	}
	lifecycle, fixed, err := s.lifecycleForRun(run)
	if err != nil {
		return err
	}
	snapshot, err := s.lifecycleSnapshot(run, fixed.workflow, fixed.execution, fixed.attempt, nil)
	if err != nil {
		return err
	}
	requestOp := op
	requestOp.Action = string(executionbackend.LifecycleStopExecution)
	requestOp.LifecycleIdentity = &executionbackend.LifecycleOperationIdentity{ProjectID: snapshot.Session.ProjectID, ExecutionID: snapshot.Session.ExecutionID, ConfigurationDigest: snapshot.ConfigurationDigest, ResourceGeneration: snapshot.ResourceGeneration, InputBindingDigest: executionbackend.LifecycleBindingDigest(snapshot), Ref: *op.Ref}
	request := executionbackend.LifecycleRequest{Snapshot: snapshot, Operation: requestOp, Input: executionbackend.LifecycleInput{Stop: &executionbackend.StopInput{TurnID: snapshot.Session.CurrentTurnID}}}
	records := s.records
	entry, err := registrationFor(fixed.config, s.runtime.Directory)
	if err != nil {
		return err
	}
	var reconciliation executionbackend.LifecyclePort = stopReconciliationPort{LifecyclePort: lifecycle, backend: BackendFor(fixed.config, s.runtime.Directory), ref: *op.Ref}
	if entry.HostManagedPermissions {
		reconciliation = lifecycle
	}
	err = s.dispatchResourceEffect(ctx, run, reconciliation, request, true, func(run *reconstructiontypes.RunView, result lifecycleEffectResult) error {
		if err := result.receipt.ValidateFor(request); err != nil {
			return err
		}
		saved, err := records.loadOperation(op.RunID, op.AttemptID, op.ID)
		if err != nil {
			return err
		}
		expected := recordFingerprint(saved)
		if saved.Ref == nil || digestValue(*saved.Ref) != digestValue(*op.Ref) || saved.PayloadDigest != op.PayloadDigest {
			return executionmodel.ErrTurnStale
		}
		saved.Status, saved.Diagnostic, saved.UpdatedAt = result.receipt.Disposition, result.receipt.Diagnostic, now()
		saved.Receipt = append([]byte(nil), result.receipt.Operation.Receipt...)
		if result.err != nil {
			saved.Status, saved.Diagnostic = executionbackend.OperationUnknown, result.err.Error()
		}
		if saved.Status == executionbackend.OperationConfirmed {
			e := s.executionRecord(run)
			e.ResourceStopOperationID, e.ResourceStoppedAt = saved.ID, ptr(now())
			if s.workflowRecord(run).Status != "completed" {
				e.Status = "stopped"
			}
			s.refreshRun(run)
		}
		return s.commitExecutionOperation(run, saved, expected)
	})
	if errors.Is(err, errLifecycleEffectPending) {
		return nil
	}
	return err
}

func (s *Service) stopRunExecution(ctx context.Context, r *reconstructiontypes.RunView, worker *Worker) (executionbackend.Operation, error) {
	if r == nil {
		return executionbackend.Operation{}, errors.New("there is no active execution to stop")
	}
	config, configErr := s.executionConfigForRun(r)
	if configErr != nil {
		return executionbackend.Operation{}, configErr
	}
	records := s.records
	ref, err := records.loadBinding(r.RunID, r.AttemptID)
	if err != nil {
		return executionbackend.Operation{}, &executionbackend.BackendError{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "active execution has no valid fixed binding", Cause: err}
	}
	if ref.RunID != r.RunID || ref.AttemptID != r.AttemptID || r.Process == nil || ref.External != *r.Process {
		return executionbackend.Operation{}, &executionbackend.BackendError{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "active execution binding does not match runtime identity"}
	}
	entry, registryErr := registrationFor(config, s.runtime.Directory)
	if registryErr != nil {
		return executionbackend.Operation{}, registryErr
	}
	if (worker == nil || worker.proc == nil) && !entry.HostManagedPermissions {
		return executionbackend.Operation{}, errors.New("there is no original local handle to stop")
	}
	if entry.Key.Backend != ref.Backend || entry.Key.Protocol != ref.Protocol || entry.Key.Agent != ref.Agent.Name {
		return executionbackend.Operation{}, &executionbackend.BackendError{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "configured backend/protocol differs from fixed execution"}
	}
	stopper, stopperErr := executionStopperFor(config, s.runtime.Directory)
	if stopperErr != nil {
		return executionbackend.Operation{}, stopperErr
	}
	opID := "stop-" + r.AttemptID
	payload := struct {
		Policy string
		Grace  string
	}{"fixed_local_execution_stop", s.runtimeSettings.ShutdownGrace.String()}
	op := executionbackend.Operation{Version: executionbackend.ExecutionRecordVersion, ID: opID, RunID: r.RunID, AttemptID: r.AttemptID, Backend: ref.Backend, Protocol: ref.Protocol, Target: ref.Target, Action: "stop", PayloadDigest: digestValue(payload), Status: executionbackend.OperationPending, StartedAt: now(), UpdatedAt: now(), Ref: &ref}
	if old, loadErr := records.loadOperation(r.RunID, r.AttemptID, opID); loadErr == nil {
		if old.Action != op.Action || old.PayloadDigest != op.PayloadDigest || old.Target != op.Target {
			return executionbackend.Operation{}, errors.New("stop operation id conflicts with its fixed target/policy")
		}
		return old, nil // Never resend a persisted stop operation.
	} else if !os.IsNotExist(loadErr) {
		return executionbackend.Operation{}, loadErr
	}
	if err = s.commitExecutionOperation(r, op, ""); err != nil {
		return executionbackend.Operation{}, err
	}
	expected := recordFingerprint(op)
	op.Status, op.UpdatedAt = executionbackend.OperationInFlight, now()
	if err = s.commitExecutionOperation(r, op, expected); err != nil {
		return executionbackend.Operation{}, err
	}
	lifecycle, fixed, err := s.lifecycleForRun(r)
	if err != nil {
		return op, err
	}
	snapshot, err := s.lifecycleSnapshot(r, fixed.workflow, fixed.execution, fixed.attempt, nil)
	if err != nil {
		return op, err
	}
	requestOperation := op
	requestOperation.Action = string(executionbackend.LifecycleStopExecution)
	requestOperation.LifecycleIdentity = &executionbackend.LifecycleOperationIdentity{ProjectID: snapshot.Session.ProjectID, ExecutionID: snapshot.Session.ExecutionID, ConfigurationDigest: snapshot.ConfigurationDigest, ResourceGeneration: snapshot.ResourceGeneration, InputBindingDigest: executionbackend.LifecycleBindingDigest(snapshot), Ref: *op.Ref}
	request := executionbackend.LifecycleRequest{Snapshot: snapshot, Operation: requestOperation, Input: executionbackend.LifecycleInput{Stop: &executionbackend.StopInput{TurnID: snapshot.Session.CurrentTurnID}}}
	if entry.HostManagedPermissions {
		stopper = nil
	}
	var handle executionbackend.StopHandle
	if worker != nil {
		handle = worker.proc
	}
	port := lifecycleStopEffectPort{LifecyclePort: lifecycle, handle: handle, stopper: stopper, operation: op}
	err = s.dispatchResourceEffect(ctx, r, port, request, false, func(run *reconstructiontypes.RunView, result lifecycleEffectResult) error {
		if err := result.receipt.ValidateFor(request); err != nil {
			return err
		}
		saved, err := records.loadOperation(op.RunID, op.AttemptID, op.ID)
		if err != nil {
			return err
		}
		expected := recordFingerprint(saved)
		status, diagnostic := reliable.Outcome(result.receipt, result.err)
		saved.Status, saved.Diagnostic, saved.UpdatedAt = executionbackend.OperationStatus(status), diagnostic, now()
		saved.Receipt = append([]byte(nil), result.receipt.Operation.Receipt...)
		if result.err != nil || saved.Status != executionbackend.OperationConfirmed && saved.Status != executionbackend.OperationAccepted && saved.Status != executionbackend.OperationRejected {
			saved.Status = executionbackend.OperationUnknown
		}
		return s.commitExecutionOperation(run, saved, expected)
	})
	if err != nil {
		return op, err
	}
	return records.loadOperation(op.RunID, op.AttemptID, op.ID)
}

func (s *Service) confirmExecutionStop(op executionbackend.Operation) (bool, error) {
	config, configErr := s.executionConfigForWorkflowID(op.RunID)
	if configErr != nil {
		return false, configErr
	}
	if op.ID == "" {
		return false, errors.New("stop operation is missing")
	}
	records := s.records
	run := s.find(op.RunID)
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
	if op.Status == executionbackend.OperationConfirmed {
		persisted, err := records.loadOperation(op.RunID, op.AttemptID, op.ID)
		if err != nil {
			return false, err
		}
		if persisted.Status != executionbackend.OperationConfirmed || persisted.PayloadDigest != op.PayloadDigest || persisted.Target != op.Target || persisted.Ref == nil || op.Ref == nil || digestValue(*persisted.Ref) != digestValue(*op.Ref) {
			return false, &executionbackend.BackendError{Code: executionbackend.BackendOutcomeUnknown, Diagnostic: "exact stop confirmation was not durably saved"}
		}
		return true, nil
	}
	ref := op.Ref
	if ref == nil {
		loaded, err := s.records.loadBinding(op.RunID, op.AttemptID)
		if err != nil {
			return false, err
		}
		ref = &loaded
	}
	entry, resolveErr := registrationFor(config, s.runtime.Directory)
	if resolveErr != nil {
		return false, resolveErr
	}
	if entry.HostManagedPermissions {
		lifecycle, fixed, err := s.lifecycleForRun(run)
		if err != nil {
			return false, err
		}
		snapshot, err := s.lifecycleSnapshot(run, fixed.workflow, fixed.execution, fixed.attempt, nil)
		if err != nil {
			return false, err
		}
		requestOp := op
		requestOp.Action = string(executionbackend.LifecycleStopExecution)
		requestOp.LifecycleIdentity = &executionbackend.LifecycleOperationIdentity{ProjectID: snapshot.Session.ProjectID, ExecutionID: snapshot.Session.ExecutionID, ConfigurationDigest: snapshot.ConfigurationDigest, ResourceGeneration: snapshot.ResourceGeneration, InputBindingDigest: executionbackend.LifecycleBindingDigest(snapshot), Ref: *ref}
		request := executionbackend.LifecycleRequest{Snapshot: snapshot, Operation: requestOp, Input: executionbackend.LifecycleInput{Stop: &executionbackend.StopInput{TurnID: snapshot.Session.CurrentTurnID}}}
		receipt, _, err := lifecycle.Reconcile(context.Background(), request)
		if err == nil {
			err = receipt.ValidateFor(request)
		}
		op.Status, op.Diagnostic, op.UpdatedAt = receipt.Disposition, receipt.Diagnostic, now()
		op.Receipt = append([]byte(nil), receipt.Operation.Receipt...)
		if err != nil || op.Status != executionbackend.OperationConfirmed {
			op.Status = executionbackend.OperationUnknown
			if err != nil {
				op.Diagnostic = err.Error()
			}
		}
		saveErr := s.commitExecutionOperation(run, op, expected)
		return op.Status == executionbackend.OperationConfirmed && saveErr == nil, errors.Join(err, saveErr)
	}
	backend := BackendFor(config, s.runtime.Directory)
	obs, err := backend.Observe(context.Background(), *ref)
	if err != nil {
		op.Status, op.Diagnostic, op.UpdatedAt = executionbackend.OperationUnknown, err.Error(), now()
		saveErr := s.commitExecutionOperation(run, op, expected)
		return false, errors.Join(err, saveErr)
	}
	if obs.Liveness != "exited" {
		op.Status, op.Diagnostic, op.UpdatedAt = executionbackend.OperationUnknown, "original execution has not produced stopped evidence: "+obs.Diagnostic, now()
		saveErr := s.commitExecutionOperation(run, op, expected)
		return false, errors.Join(&executionbackend.BackendError{Code: executionbackend.BackendOutcomeUnknown, Diagnostic: op.Diagnostic}, saveErr)
	}
	op.Status, op.Diagnostic, op.UpdatedAt = executionbackend.OperationConfirmed, obs.Diagnostic, now()
	if err = s.commitExecutionOperation(run, op, expected); err != nil {
		return false, err
	}
	return true, nil
}
