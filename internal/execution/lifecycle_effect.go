package execution

import (
	"context"
	"errors"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

var errLifecycleEffectPending = errors.New("lifecycle action accepted and awaiting its original receipt")

// Stops bind the original owned handle without exposing it to actor state in
// the background job. Their results use the same immutable receipt handoff.
type lifecycleStopEffectPort struct {
	executionbackend.LifecyclePort
	handle    executionbackend.StopHandle
	stopper   executionbackend.ExecutionStopper
	operation executionbackend.Operation
}

func (p lifecycleStopEffectPort) Execute(ctx context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, executionbackend.LifecycleObserver, error) {
	if p.stopper == nil {
		receipt, err := p.LifecyclePort.StopLifecycle(ctx, request, p.handle)
		return receipt, nil, err
	}
	receipt, err := p.stopper.StopExecution(ctx, *p.operation.Ref, p.operation, p.handle)
	return executionbackend.LifecycleReceipt{Operation: request.Operation, Disposition: executionbackend.OperationStatus(receipt.Disposition), Diagnostic: receipt.Diagnostic, RequestID: request.Operation.ID}, nil, err
}

func (p lifecycleStopEffectPort) Reconcile(ctx context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, executionbackend.LifecycleObserver, error) {
	if p.stopper == nil {
		return p.LifecyclePort.Reconcile(ctx, request)
	}
	// The original execution-stop owner verifies process absence on the actor's
	// next reconciliation pass. An unknown stop is never repeated.
	return executionbackend.LifecycleReceipt{Operation: request.Operation, Disposition: executionbackend.OperationUnknown}, nil, nil
}

type lifecycleEffectResult struct {
	request    executionbackend.LifecycleRequest
	receipt    executionbackend.LifecycleReceipt
	observer   executionbackend.LifecycleObserver
	err        error
	uncertain  bool
	reconciled bool
}

type pendingLifecycleEffect struct {
	runID              string
	apply              func(*reconstructiontypes.RunView, lifecycleEffectResult) error
	release            func()
	port               executionbackend.LifecyclePort
	resourceManagement bool
}

func (s *Service) lifecycleEffectPending(runID string) bool {
	for _, effect := range s.lifecycleEffects {
		if effect.runID == runID {
			return true
		}
	}
	return false
}

// invokeLifecycleEffect has no business state or write port. Unknown mutations
// are only reconciled against this same immutable request, never executed again.
func invokeLifecycleEffect(ctx context.Context, port executionbackend.LifecyclePort, request executionbackend.LifecycleRequest, reconcile bool) lifecycleEffectResult {
	result := lifecycleEffectResult{request: request.Clone(), reconciled: reconcile}
	if reconcile {
		result.receipt, result.observer, result.err = port.Reconcile(ctx, request.Clone())
	} else {
		result.receipt, result.observer, result.err = port.Execute(ctx, request.Clone())
	}
	result.uncertain = !reconcile && (result.receipt.Disposition == executionbackend.OperationUnknown || result.err != nil && result.receipt.Disposition != executionbackend.OperationRejected)
	result.receipt = result.receipt.Clone()
	return result
}

// A failed reconciliation must not replace already observed facts with an
// empty or unknown receipt. Merge before the actor chooses business or facts
// processing, so a candidate stopped during reconciliation keeps them too.
func reconcileLifecycleResult(result lifecycleEffectResult, original *lifecycleEffectResult) lifecycleEffectResult {
	if original == nil {
		return result
	}
	result.uncertain = true
	validationErr := result.receipt.ValidateFor(result.request)
	if result.err == nil && validationErr == nil && result.receipt.Disposition != executionbackend.OperationUnknown {
		return result
	}
	if result.observer != nil {
		_ = result.observer.Close(context.Background())
	}
	fallback := *original
	fallback.observer = nil // The initial observer was closed before reconciliation.
	fallback.err = errors.Join(original.err, result.err, validationErr)
	fallback.reconciled = true
	return fallback
}

func (s *Service) dispatchLifecycleEffect(ctx context.Context, run *reconstructiontypes.RunView, port executionbackend.LifecyclePort, request executionbackend.LifecycleRequest, reconcile bool, apply func(*reconstructiontypes.RunView, lifecycleEffectResult) error) error {
	return s.dispatchLifecycleEffectAuthorized(ctx, run, port, request, reconcile, apply, false, nil)
}

// Only explicit resource management and verified delivery cleanup use this
// entry. Candidate stop and normal business progression never grant it.
func (s *Service) dispatchResourceEffect(ctx context.Context, run *reconstructiontypes.RunView, port executionbackend.LifecyclePort, request executionbackend.LifecycleRequest, reconcile bool, apply func(*reconstructiontypes.RunView, lifecycleEffectResult) error) error {
	action := executionbackend.LifecycleAction(request.Operation.Action)
	if action != executionbackend.LifecycleCleanup && action != executionbackend.LifecycleStopExecution && action != executionbackend.LifecycleStopObservation {
		return errors.New("resource operation authorization cannot grant business actions")
	}
	return s.dispatchLifecycleEffectAuthorized(ctx, run, port, request, reconcile, apply, true, nil)
}

func (s *Service) dispatchLifecycleEffectAuthorized(ctx context.Context, run *reconstructiontypes.RunView, port executionbackend.LifecyclePort, request executionbackend.LifecycleRequest, reconcile bool, apply func(*reconstructiontypes.RunView, lifecycleEffectResult) error, resourceManagement bool, original *lifecycleEffectResult) error {
	if !resourceManagement && s.candidateBusinessClosed(run) {
		return nativeErr("candidate_inactive", "candidate no longer participates")
	}
	if err := request.Validate(); err != nil {
		return err
	}
	if s.lifecycleEffectPending(run.RunID) {
		return errLifecycleEffectPending
	}
	request = request.Clone()
	// Isolated initialization/tests await the identical background-result path;
	// the running actor always returns immediately and consumes signalQueue.
	messages, jobCtx := s.signalTransport()
	if messages == nil {
		result := make(chan lifecycleEffectResult, 1)
		go func() {
			result <- reconcileLifecycleResult(invokeLifecycleEffect(ctx, port, request, reconcile), original)
		}()
		received := <-result
		return s.consumeLifecycleEffect(ctx, run, port, received, apply, resourceManagement)
	}
	if s.lifecycleEffects == nil {
		s.lifecycleEffects = make(map[string]pendingLifecycleEffect)
	}
	pending := pendingLifecycleEffect{runID: run.RunID, apply: apply, port: port, resourceManagement: resourceManagement}
	if owner, ok := port.(executionbackend.LifecyclePreparationOwner); ok {
		pending.release = owner.ReleasePreparation
	}
	s.lifecycleEffects[request.Operation.ID] = pending
	runID := run.RunID
	if jobCtx == nil {
		jobCtx = ctx
	}
	deliveryCtx := jobCtx
	acknowledgement := executionbackend.LifecycleAction(request.Operation.Action) == executionbackend.LifecycleAcknowledge
	deadline, bounded := ctx.Deadline()
	s.forwarders.Add(1)
	go func() {
		defer s.forwarders.Done()
		if acknowledgement {
			// Its message already passed the persistence barrier. Service cancellation
			// must not strand an unsent acknowledgement behind a mutation intent.
			var cancel context.CancelFunc
			jobCtx, cancel = context.WithTimeout(context.Background(), s.runtimeSettings.ShutdownGrace)
			defer cancel()
		}
		if bounded {
			var cancel context.CancelFunc
			jobCtx, cancel = context.WithDeadline(jobCtx, deadline)
			defer cancel()
		}
		result := reconcileLifecycleResult(invokeLifecycleEffect(jobCtx, port, request, reconcile), original)
		select {
		case messages <- Signal{runID: runID, effect: &result}:
		case <-deliveryCtx.Done():
			if acknowledgement {
				// The stopping actor drains these original immutable receipts too.
				s.forwardMu.Lock()
				s.forwardedDone = append(s.forwardedDone, Signal{runID: runID, effect: &result})
				s.forwardMu.Unlock()
				return
			}
			if result.observer != nil {
				_ = result.observer.Close(context.Background())
			}
			if owner, ok := port.(executionbackend.LifecyclePreparationOwner); ok {
				owner.ReleasePreparation()
			}
		}
	}()
	return errLifecycleEffectPending
}

func (s *Service) applyLifecycleEffect(runID string, result lifecycleEffectResult) error {
	pending, ok := s.lifecycleEffects[result.request.Operation.ID]
	delete(s.lifecycleEffects, result.request.Operation.ID)
	discard := func() error {
		if result.observer != nil {
			_ = result.observer.Close(context.Background())
		}
		if pending.release != nil {
			pending.release()
		}
		return executionmodel.ErrTurnStale
	}
	if !ok || pending.runID != runID {
		return discard()
	}
	run := s.find(runID)
	if run == nil || s.workflowRecord(run) == nil || run.AttemptID != result.request.Snapshot.Session.AttemptID || run.ExecutionID != result.request.Snapshot.Session.ExecutionID {
		return discard()
	}
	_, ctx := s.signalTransport()
	if ctx == nil {
		ctx = context.Background()
	}
	err := s.consumeLifecycleEffect(ctx, run, pending.port, result, pending.apply, pending.resourceManagement)
	if errors.Is(err, errLifecycleEffectPending) || errors.Is(err, errLifecycleNeedsReconciliation) || errors.Is(err, errLifecycleResourcePending) {
		return nil
	}
	return err
}

// All original receipts cross this actor-owned boundary. Only participating
// candidates may reconcile an unknown mutation or run the business callback.
func (s *Service) consumeLifecycleEffect(ctx context.Context, run *reconstructiontypes.RunView, port executionbackend.LifecyclePort, result lifecycleEffectResult, apply func(*reconstructiontypes.RunView, lifecycleEffectResult) error, resourceManagement bool) error {
	if s.candidateBusinessClosed(run) && !resourceManagement {
		return s.recordLifecycleReceipt(run, result)
	}
	if result.uncertain && !result.reconciled {
		if result.observer != nil {
			_ = result.observer.Close(context.Background())
		}
		request := result.request.Clone()
		if request.Operation.Action == string(executionbackend.LifecyclePrepareWorkspace) && len(result.receipt.Operation.Receipt) > 0 && result.receipt.ValidateFor(request) == nil {
			request.Operation.Receipt = mustJSON(result.receipt)
		}
		return s.dispatchLifecycleEffectAuthorized(ctx, run, port, request, true, apply, resourceManagement, &result)
	}
	return apply(run, result)
}

// Record only proven facts of an original in-flight request, using the same
// binding validation, receipt merge, operation journal and worker commit as
// normal lifecycle processing. No planner, adapter or recovery is invoked.
func (s *Service) recordLifecycleReceipt(run *reconstructiontypes.RunView, result lifecycleEffectResult) (retErr error) {
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
	w, e := s.workflowRecords(run)
	if w == nil || e == nil {
		return executionmodel.ErrTurnStale
	}
	frozen := w.Status == "completed"
	var candidate executionmodel.ExecutionRecord
	if err := decode(mustJSON(e), &candidate); err != nil {
		return err
	}
	if err := mergeLifecycleExecutionReceipt(&candidate, receipt); err != nil {
		return err
	}
	if !frozen {
		if receipt.State.InputAccepted && receipt.State.TurnID != "" {
			candidate.PendingTurnID, candidate.Status = receipt.State.TurnID, "pending"
		}
		if evidence := receipt.TurnEvidence; evidence != nil {
			if evidence.SessionID != candidate.SessionID || evidence.TurnID != candidate.CurrentTurnID && evidence.TurnID != candidate.PendingTurnID {
				return executionmodel.ErrTurnStale
			}
			candidate.Summary = ptr(evidence.LastMessage)
			if evidence.Completed {
				if candidate.LastStoppedTurnID != evidence.TurnID || candidate.TurnEndedAt == nil {
					candidate.TurnEndedAt = ptr(now())
				}
				candidate.Status, candidate.LastStoppedTurnID = "idle", evidence.TurnID
				if evidence.Succeeded {
					candidate.LastTurnOutcome = "succeeded"
				} else {
					candidate.LastTurnOutcome = "failed"
				}
			}
		}
		*e = candidate
	}
	status, diagnostic := lifecycleOutcome(receipt, result.err)
	if err := s.setOperationState(request.Operation.ID, status, receipt.RequestID, mustJSON(receipt), diagnostic); err != nil {
		return err
	}
	if owned, ok := receipt.Handle.(executionbackend.OwnedProcessHandle); ok && owned.OwnedProcess() {
		if s.ownedProcesses == nil {
			s.ownedProcesses = map[string]executionbackend.StopHandle{}
		}
		s.ownedProcesses[run.RunID] = owned
	}
	s.refreshRun(run)
	s.note(run, "steer", "worker", "late_receipt", "original receipt retained without business progression", request.Operation.ID, nil)
	if err := s.commitWorker(run, request.Operation.ID); err != nil {
		return err
	}
	if result.observer != nil {
		worker := workerFromLifecycle(run.RunID, run.AttemptID, result.observer)
		if worker == nil || worker.proc == nil || e.Process == nil || worker.proc.Identity() != *e.Process {
			return executionmodel.ErrTurnStale
		}
		s.registerWorker(run, worker)
	}
	return nil
}
