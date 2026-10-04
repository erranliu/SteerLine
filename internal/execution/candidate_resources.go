package execution

import (
	"context"
	"errors"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	contracts "github.com/erranliu/SteerLine/internal/steerline/contracts"
)

// Receipt handling after experiment exit retains physical facts and the
// original Operation. It never asks a planner for another business action.
func (s *Service) applyRetiredLifecycleReceipt(run *contracts.Run, result lifecycleEffectResult) (retErr error) {
	defer func() {
		if retErr != nil && result.observer != nil {
			_ = result.observer.Close(context.Background())
		}
	}()
	request, receipt := result.request, result.receipt
	if err := receipt.ValidateFor(request); err != nil {
		return err
	}
	if err := s.validateLifecycleResultBinding(run, request); err != nil {
		return err
	}
	fixed, err := s.workerFlowContext(run)
	if err != nil {
		return err
	}
	status, diagnostic := string(receipt.Disposition), receipt.Diagnostic
	if result.err != nil && status != "rejected" {
		status, diagnostic = "outcome_unknown", result.err.Error()
	}
	if err := s.setOperationState(request.Operation.ID, status, receipt.RequestID, mustJSON(receipt), diagnostic); err != nil {
		return err
	}
	// The selected Delivery freezes its execution identity. In-flight resource
	// receipts for that winner still remain in the original Operation and event.
	if fixed.workflow.Phase != "completed" {
		if err := mergeLifecycleExecutionReceipt(fixed.execution, receipt); err != nil {
			return err
		}
		if receipt.State.InputAccepted && receipt.State.TurnID != "" {
			fixed.execution.PendingTurnID = receipt.State.TurnID
			fixed.execution.Status = "pending"
			fixed.attempt.Status = "dispatched"
		}
	}
	if fixed.workflow.Phase == "completed" && receipt.TurnEvidence != nil {
		evidence := receipt.TurnEvidence
		if evidence.SessionID != fixed.execution.SessionID || evidence.TurnID != fixed.execution.CurrentTurnID && evidence.TurnID != fixed.execution.PendingTurnID {
			return executionmodel.ErrTurnStale
		}
		fixed.execution.Summary = ptr(evidence.LastMessage)
	}
	if receipt.Handle != nil {
		if owned, ok := receipt.Handle.(executionbackend.OwnedProcessHandle); ok && owned.OwnedProcess() {
			if s.ownedProcesses == nil {
				s.ownedProcesses = map[string]executionbackend.StopHandle{}
			}
			s.ownedProcesses[run.RunID] = owned
		}
	}
	s.refreshRun(run)
	s.note(run, "steer", "worker", "late_receipt", "retired candidate retained the original resource/input receipt", request.Operation.ID, nil)
	if err := s.commitWorker(run, request.Operation.ID); err != nil {
		return err
	}
	if result.observer != nil {
		worker := workerFromLifecycle(run.RunID, run.AttemptID, result.observer)
		if worker == nil {
			return errors.New("retired receipt observer has no original execution identity")
		}
		s.registerWorker(run, worker)
	}
	return nil
}

func (s *Service) recordRetiredWorkerEvent(run *contracts.Run, ev WorkerEvent) error {
	w, e := s.workflowRecords(run)
	if e == nil || w == nil {
		return executionmodel.ErrTurnStale
	}
	if ev.Fact.SessionID != "" && e.SessionID != "" && ev.Fact.SessionID != e.SessionID {
		return executionmodel.ErrTurnStale
	}
	if ev.Activity.Kind != "" {
		if err := s.recordConversationEvent(run, ev); err != nil {
			return err
		}
		s.recordActivity(run, ev.Activity)
	}
	var operations []string
	if w.Phase != "completed" {
		for _, envelope := range ev.Implementation {
			if err := envelope.ValidateResourceIdentity(e.ProjectIdentity, e.ExecutionID, e.CurrentAttemptID, e.Generation, e.ConfigurationDigest); err != nil {
				return err
			}
			if err := putImplementationEnvelope(e, envelope); err != nil {
				return err
			}
		}
		if ev.Fact.Kind == executionbackend.EventStarted && ev.Fact.StartedTurnID != "" {
			phase, updated := w.Phase, w.UpdatedAt
			commandID := e.PendingCommandID
			if commandID == "" {
				commandID = e.CurrentCommandID
			}
			op := s.operationByKey("worker-turn:" + e.ExecutionID + ":" + commandID)
			if op == nil {
				return executionmodel.ErrTurnStale
			}
			if err := BeginWorkerTurn(w, e, ev.Fact.SessionID, ev.Fact.StartedTurnID); err != nil {
				return err
			}
			e.CurrentCommandID, e.PendingCommandID = commandID, ""
			if err := s.setOperationState(op.OperationID, "confirmed", ev.Fact.StartedTurnID, mustJSON(ev.Fact), ""); err != nil {
				return err
			}
			w.Phase, w.UpdatedAt = phase, updated
			operations = append(operations, op.OperationID)
		}
		if ev.DeliveryFinal && ev.DeliveryID != "" {
			e.PendingDeliveryID = ev.DeliveryID
		}
		if ev.Summary != "" {
			e.LastEvent = ptr(ev.Summary)
		}
	}
	s.refreshRun(run)
	if err := s.commitWorker(run, operations...); err != nil {
		return err
	}
	if w.Phase != "completed" && ev.Candidate != nil {
		candidate := ev.Candidate.Clone()
		if candidate.Action != executionbackend.LifecycleReportCompletion || candidate.Input.Completion == nil || candidate.Input.Completion.TurnID != e.CurrentTurnID && candidate.Input.Completion.TurnID != e.PendingTurnID {
			return executionmodel.ErrTurnStale
		}
		lifecycle, _, err := s.lifecycleForRun(run)
		if err != nil {
			return err
		}
		if err := s.executeLifecycleStep(context.Background(), run, lifecycle, candidate); err != nil && !errors.Is(err, errLifecycleEffectPending) {
			return err
		}
	}
	if w.Phase != "completed" && ev.DeliveryFinal && ev.DeliveryID != "" {
		return s.acknowledgeLifecycleDelivery(run, ev.DeliveryID)
	}
	return nil
}

func (s *Service) finishRetiredCandidate(run *contracts.Run, lifecycle executionbackend.LifecyclePort, fixed workerFlowContext) error {
	snapshot, err := s.lifecycleSnapshot(run, fixed.workflow, fixed.execution, fixed.attempt, nil)
	if err != nil {
		return err
	}
	receipt, observer, err := lifecycle.ObserveLifecycle(context.Background(), snapshot)
	if err != nil {
		return err
	}
	if observer != nil {
		if fixed.execution.Process == nil || observer.StopHandle() == nil || observer.StopHandle().Identity() != *fixed.execution.Process {
			_ = observer.Close(context.Background())
			return executionmodel.ErrTurnStale
		}
		s.registerWorker(run, workerFromLifecycle(run.RunID, run.AttemptID, observer))
		return nil
	}
	if fixed.workflow.Phase == "completed" && receipt.TurnEvidence != nil {
		evidence := receipt.TurnEvidence
		if evidence.SessionID != fixed.execution.SessionID || evidence.TurnID != fixed.execution.CurrentTurnID && evidence.TurnID != fixed.execution.PendingTurnID {
			return executionmodel.ErrTurnStale
		}
		fixed.execution.Summary = ptr(evidence.LastMessage)
	}
	if fixed.workflow.Phase != "completed" {
		if err := mergeLifecycleExecutionReceipt(fixed.execution, receipt); err != nil {
			return err
		}
		if evidence := receipt.TurnEvidence; evidence != nil && evidence.Completed {
			e := fixed.execution
			if e.SessionID != evidence.SessionID || e.CurrentTurnID != evidence.TurnID && e.PendingTurnID != evidence.TurnID {
				return executionmodel.ErrTurnStale
			}
			e.Status, e.LastStoppedTurnID, e.TurnEndedAt = "idle", evidence.TurnID, ptr(now())
			e.Summary = ptr(evidence.LastMessage)
			if len(evidence.PRCandidates) == 1 {
				fixed.workflow.PRCandidateURL = evidence.PRCandidates[0]
			}
			if evidence.Succeeded {
				e.LastTurnOutcome = "succeeded"
			} else {
				e.LastTurnOutcome = "failed"
			}
		}
	}
	s.refreshRun(run)
	s.note(run, "steer", "worker", "retired_output", "retired candidate original turn observation retained", receipt.Diagnostic, map[string]any{"observation": receipt})
	return s.commitWorker(run)
}

func (s *Service) resumeRetiredCandidate(ctx context.Context, run *contracts.Run) error {
	if s.lifecycleEffectPending(run.RunID) {
		return nil
	}
	lifecycle, fixed, err := s.lifecycleForRun(run)
	if err != nil {
		return err
	}
	if fixed.workflow.Phase == "completed" {
		return nil
	}
	snapshot, err := s.lifecycleSnapshot(run, fixed.workflow, fixed.execution, fixed.attempt, nil)
	if err != nil {
		return err
	}
	// Reconcile only already-dispatched original operations, never pending
	// intents which had not crossed the dispatch boundary before exit.
	for _, op := range snapshot.Operations {
		if op.Status != executionbackend.OperationPending && op.Status != executionbackend.OperationConfirmed && op.Status != executionbackend.OperationRejected {
			one := snapshot.Clone()
			one.Operations = []executionbackend.Operation{op}
			candidate, found, err := unresolvedLifecycleCandidate(one)
			if err != nil {
				return err
			}
			if found {
				err := s.executeLifecycleStep(ctx, run, lifecycle, candidate)
				if errors.Is(err, errLifecycleEffectPending) || errors.Is(err, errLifecycleNeedsReconciliation) {
					return nil
				}
				return err
			}
		}
	}
	if fixed.execution.PendingDeliveryID != "" {
		return s.acknowledgeLifecycleDelivery(run, fixed.execution.PendingDeliveryID)
	}
	worker := s.workers[run.RunID]
	if worker == nil && s.ownedProcesses[run.RunID] != nil {
		worker = &Worker{proc: s.ownedProcesses[run.RunID]}
	}
	if worker != nil && fixed.workflow.Abandonment != nil {
		op, err := s.stopRunExecution(ctx, run, worker)
		if errors.Is(err, errLifecycleEffectPending) {
			return nil
		}
		if err != nil {
			message := "candidate exited; original worker stop is unsupported or unverified: " + err.Error()
			if deref(fixed.execution.Error) != message {
				fixed.execution.Error = ptr(message)
				s.refreshRun(run)
				return s.commitWorker(run)
			}
			return nil
		}
		if op.Status == executionbackend.OperationConfirmed {
			fixed.execution.Status = "stopped"
			s.refreshRun(run)
			return s.commitWorker(run)
		}
		if op.Status != executionbackend.OperationRejected {
			return s.reconcileRunStop(ctx, run, op)
		}
		return nil
	}
	if fixed.execution.Process != nil && fixed.execution.CurrentTurnID != fixed.execution.LastStoppedTurnID && s.workers[run.RunID] == nil {
		// Attach observation to the exact original resource. This does not start
		// a replacement process, prepare resources, or send another input.
		receipt, observer, err := lifecycle.ObserveLifecycle(ctx, snapshot)
		if err != nil {
			return nil
		}
		if observer != nil {
			if fixed.execution.Process == nil || observer.StopHandle() == nil || observer.StopHandle().Identity() != *fixed.execution.Process {
				_ = observer.Close(context.Background())
				return executionmodel.ErrTurnStale
			}
			s.registerWorker(run, workerFromLifecycle(run.RunID, run.AttemptID, observer))
			return nil
		}
		if receipt.TurnEvidence != nil && receipt.TurnEvidence.Completed {
			return s.finishRetiredCandidate(run, lifecycle, fixed)
		}
	}
	return nil
}

func retiredResourceAction(action executionbackend.LifecycleAction, input executionbackend.LifecycleInput) bool {
	return action == executionbackend.LifecycleCleanup || action == executionbackend.LifecycleStopExecution || action == executionbackend.LifecycleStopObservation || action == executionbackend.LifecycleAcknowledge || action == executionbackend.LifecycleReportCompletion || action == executionbackend.LifecycleHostAction && input.Host != nil && (input.Host.Kind == executionbackend.HostCleanupProbeFiles || input.Host.Kind == executionbackend.HostRemoveFeedback)
}
