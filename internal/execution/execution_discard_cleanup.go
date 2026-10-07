package execution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

func (s *Service) reconcileDiscardedExecutionStops(ctx context.Context) error {
	for i := range s.domain.Tasks {
		task := s.domain.Tasks[i]
		if task.Status != "discarded" || len(task.StopPending) == 0 {
			continue
		}
		for _, item := range append([]string(nil), task.StopPending...) {
			if !strings.HasPrefix(item, "execution_stop_manual:") {
				continue
			}
			executionID := strings.TrimPrefix(item, "execution_stop_manual:")
			run := s.findExecution(executionID)
			var worker *Worker
			if run != nil {
				worker = s.workers[run.RunID]
				if worker == nil && s.ownedProcesses[run.RunID] != nil {
					worker = &Worker{proc: s.ownedProcesses[run.RunID]}
				}
			}
			if worker != nil {
				config, configErr := s.executionConfigForRun(run)
				if configErr != nil {
					return configErr
				}
				if _, err := executionStopperFor(config, s.runtime.Directory); err != nil {
					s.executionRecord(run).Error = ptr("discarded task worker remains active; original stop adapter is unavailable: " + clip(err.Error(), 1000))
					s.refreshRun(run)
					if commitErr := s.commitWorker(run); commitErr != nil {
						return commitErr
					}
					continue
				}
				op, stopErr := s.stopRunExecution(ctx, run, worker)
				if errors.Is(stopErr, errLifecycleEffectPending) {
					continue
				}
				if stopErr != nil {
					s.executionRecord(run).Error = ptr("discarded task worker stop requires reconciliation: " + clip(stopErr.Error(), 1000))
					s.refreshRun(run)
					if commitErr := s.commitWorker(run); commitErr != nil {
						return commitErr
					}
					continue
				}
				if op.Status != executionbackend.OperationConfirmed {
					backend := BackendFor(config, s.runtime.Directory)
					observation, observeErr := backend.Observe(ctx, *op.Ref)
					if observeErr != nil || observation.Liveness != "exited" {
						s.executionRecord(run).Error = ptr("discarded task worker stop is awaiting confirmed process absence")
						s.refreshRun(run)
						if commitErr := s.commitWorker(run); commitErr != nil {
							return commitErr
						}
						continue
					}
					confirmed, confirmErr := s.confirmExecutionStop(op)
					if !confirmed {
						s.executionRecord(run).Error = ptr("discarded task worker stop is not confirmed: " + clip(fmt.Sprint(confirmErr), 1000))
						s.refreshRun(run)
						if commitErr := s.commitWorker(run); commitErr != nil {
							return commitErr
						}
						continue
					}
				}
				if err := s.recordNativeStopReceipt(ctx, task.TaskID, item, op); err != nil {
					s.executionRecord(run).Error = ptr("discarded task stop receipt could not be committed: " + clip(err.Error(), 1000))
					s.refreshRun(run)
					if commitErr := s.commitWorker(run); commitErr != nil {
						return commitErr
					}
					continue
				}
				if err := s.refreshNativeSnapshot(ctx); err != nil {
					return err
				}
				// Keep the original observer until its output/ack stream drains.
				// A confirmed physical stop is not an active dispatch slot.
				if s.workers[run.RunID] == nil && s.ownedProcesses[run.RunID] == worker.proc {
					worker.proc.Close()
					delete(s.ownedProcesses, run.RunID)
				}
				if s.State.CurrentTask != nil && *s.State.CurrentTask == run.TaskID {
					s.State.CurrentTask = nil
				}
				s.refreshSlotOccupancy()
				if err := s.publishCheckpoint(); err != nil {
					return err
				}
				return nil
			}
			// After a restart there is no local Job/process-group handle to stop.
			// Observe the exact saved identity and settle only if it is already absent.
			if err := s.reconcileExitedNativeExecution(ctx, task.TaskID, item, executionID); err != nil {
				continue
			}
			return nil
		}
	}
	return nil
}

func (s *Service) recordNativeStopReceipt(ctx context.Context, taskID, stopItem string, op executionbackend.Operation) error {
	if op.Ref == nil {
		return &executionbackend.BackendError{Code: executionbackend.BackendIdentityMismatch, Diagnostic: "native stop operation has no fixed execution reference"}
	}
	confirmed, err := s.confirmExecutionStop(op)
	if err != nil || !confirmed {
		return errors.Join(err, &executionbackend.BackendError{Code: executionbackend.BackendOutcomeUnknown, Diagnostic: "native stop operation is not confirmed"})
	}
	records := s.records
	persisted, err := records.loadOperation(op.RunID, op.AttemptID, op.ID)
	if err != nil || persisted.Status != executionbackend.OperationConfirmed {
		return errors.Join(err, nativeErr("outcome_unknown", "confirmed stop was not durably recorded"))
	}
	config, configErr := s.executionConfigForWorkflowID(op.RunID)
	if configErr != nil {
		return configErr
	}
	backend := BackendFor(config, s.runtime.Directory)
	observation, err := backend.Observe(ctx, *op.Ref)
	if err != nil || observation.Liveness != "exited" {
		return errors.Join(err, nativeErr("outcome_unknown", "fixed execution is not confirmed absent"))
	}
	evidence := NativeExecutionStopReceipt{WorkflowRunID: op.RunID, ExecutionID: nativeExecutionIDForAttempt(s.domain.Attempts, op.AttemptID), AttemptID: op.AttemptID, Operation: persisted, Observation: observation}
	if evidence.ExecutionID == "" {
		return nativeErr("contract_mismatch", "stop attempt is not bound to a native execution")
	}
	task := nativeTaskRecord(s.domain.Tasks, taskID)
	if task == nil {
		return nativeErr("task_not_found", "discarded native task disappeared during stop reconciliation")
	}
	data, _ := json.Marshal(evidence)
	requestID := digestValue(struct{ TaskID, Item, OperationID string }{taskID, stopItem, op.ID})[:32]
	_, err = (s.commandService(s.Config.identity)).RecordNativeStopReceipt(ctx, taskID, requestID, task.Version, stopItem, s.taskWork.DefaultRequestedBy(), data)
	return err
}

func (s *Service) reconcileExitedNativeExecution(ctx context.Context, taskID, stopItem, executionID string) error {
	var execution *executionmodel.ExecutionRecord
	for i := range s.domain.Executions {
		if s.domain.Executions[i].ExecutionID == executionID {
			execution = &s.domain.Executions[i]
			break
		}
	}
	if execution == nil || execution.Process == nil {
		return nativeErr("outcome_unknown", "discarded task has no fixed resource identity")
	}
	workflowRunID := execution.WorkflowRunID
	config, configErr := s.executionConfigForWorkflowID(workflowRunID)
	if configErr != nil {
		return configErr
	}
	var attempt *executionmodel.StepAttemptRecord
	for i := range s.domain.Attempts {
		candidate := &s.domain.Attempts[i]
		if candidate.ExecutionID == executionID && candidate.AttemptID == execution.CurrentAttemptID {
			attempt = candidate
			break
		}
	}
	if attempt == nil {
		return nativeErr("outcome_unknown", "discarded task has no original step attempt")
	}
	ref, err := s.records.loadBinding(workflowRunID, attempt.AttemptID)
	if err != nil {
		return err
	}
	backend := BackendFor(config, s.runtime.Directory)
	observation, err := backend.Observe(ctx, ref)
	if err != nil || observation.Liveness != "exited" {
		return errors.Join(err, nativeErr("outcome_unknown", "fixed resource is live or its state is unknown; it cannot be stopped without the original handle"))
	}
	opID := "stop-" + attempt.AttemptID
	payload := struct {
		Policy string
		Grace  string
	}{"fixed_local_execution_stop", s.runtimeSettings.ShutdownGrace.String()}
	op := executionbackend.Operation{Version: executionbackend.ExecutionRecordVersion, ID: opID, RunID: workflowRunID, AttemptID: attempt.AttemptID, Backend: ref.Backend, Protocol: ref.Protocol, Target: ref.Target, Action: "stop", PayloadDigest: digestValue(payload), Status: executionbackend.OperationConfirmed, StartedAt: now(), UpdatedAt: now(), Diagnostic: observation.Diagnostic, Ref: &ref}
	records := s.records
	expectedOperation := ""
	if old, loadErr := records.loadOperation(workflowRunID, attempt.AttemptID, opID); loadErr == nil {
		expectedOperation = recordFingerprint(old)
		if old.PayloadDigest != op.PayloadDigest || old.Ref == nil || digestValue(*old.Ref) != digestValue(ref) {
			return nativeErr("authority_mismatch", "saved local stop operation differs from the fixed execution identity")
		}
		if old.Status != executionbackend.OperationConfirmed {
			old.Status, old.UpdatedAt, old.Diagnostic = executionbackend.OperationConfirmed, now(), observation.Diagnostic
			op = old
		}
	} else if !os.IsNotExist(loadErr) {
		return loadErr
	}
	if err := s.commitExecutionOperation(s.find(workflowRunID), op, expectedOperation); err != nil {
		return err
	}
	return s.recordNativeStopReceipt(ctx, taskID, stopItem, op)
}

func nativeExecutionIDForAttempt(attempts []executionmodel.StepAttemptRecord, attemptID string) string {
	for _, attempt := range attempts {
		if attempt.AttemptID == attemptID {
			return attempt.ExecutionID
		}
	}
	return ""
}
