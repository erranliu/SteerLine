package steerline

import (
	"context"
	"errors"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

var errLifecycleEffectPending = errors.New("lifecycle action accepted and awaiting its original receipt")

// Stops bind the original owned handle without exposing it to actor state in
// the background job. Their results use the same immutable receipt handoff.
type lifecycleStopEffectPort struct {
	executionapi.LifecyclePort
	handle    executionapi.StopHandle
	stopper   executionapi.ExecutionStopper
	operation executionapi.Operation
}

func (p lifecycleStopEffectPort) Execute(ctx context.Context, request executionapi.LifecycleRequest) (executionapi.LifecycleReceipt, executionapi.LifecycleObserver, error) {
	if p.stopper == nil {
		receipt, err := p.LifecyclePort.StopLifecycle(ctx, request, p.handle)
		return receipt, nil, err
	}
	receipt, err := p.stopper.StopExecution(ctx, *p.operation.Ref, p.operation, p.handle)
	return executionapi.LifecycleReceipt{Operation: request.Operation, Disposition: executionapi.OperationStatus(receipt.Disposition), Diagnostic: receipt.Diagnostic, RequestID: request.Operation.ID}, nil, err
}

func (p lifecycleStopEffectPort) Reconcile(ctx context.Context, request executionapi.LifecycleRequest) (executionapi.LifecycleReceipt, executionapi.LifecycleObserver, error) {
	if p.stopper == nil {
		return p.LifecyclePort.Reconcile(ctx, request)
	}
	// The original execution-stop owner verifies process absence on the actor's
	// next reconciliation pass. An unknown stop is never repeated.
	return executionapi.LifecycleReceipt{Operation: request.Operation, Disposition: executionapi.OperationUnknown}, nil, nil
}

type lifecycleEffectResult struct {
	request   executionapi.LifecycleRequest
	receipt   executionapi.LifecycleReceipt
	observer  executionapi.LifecycleObserver
	err       error
	uncertain bool
}
type pendingLifecycleEffect struct {
	runID   string
	apply   func(*Run, lifecycleEffectResult) error
	release func()
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
func invokeLifecycleEffect(ctx context.Context, port executionapi.LifecyclePort, request executionapi.LifecycleRequest, reconcile bool) lifecycleEffectResult {
	result := lifecycleEffectResult{request: request.Clone()}
	if reconcile {
		result.receipt, result.observer, result.err = port.Reconcile(ctx, request.Clone())
	} else {
		result.receipt, result.observer, result.err = port.Execute(ctx, request.Clone())
	}
	result.uncertain = !reconcile && (result.receipt.Disposition == executionapi.OperationUnknown || result.err != nil && result.receipt.Disposition != executionapi.OperationRejected)
	if result.uncertain {
		if result.observer != nil {
			_ = result.observer.Close(context.Background())
			result.observer = nil
		}
		receipt, observer, err := port.Reconcile(ctx, request.Clone())
		if err == nil && receipt.ValidateFor(request) == nil && receipt.Disposition != executionapi.OperationUnknown {
			result.receipt, result.observer, result.err = receipt, observer, nil
		} else {
			if observer != nil {
				_ = observer.Close(context.Background())
			}
			result.err = errors.Join(result.err, err)
		}
	}
	result.receipt = result.receipt.Clone()
	return result
}

func (s *Service) dispatchLifecycleEffect(ctx context.Context, run *Run, port executionapi.LifecyclePort, request executionapi.LifecycleRequest, reconcile bool, apply func(*Run, lifecycleEffectResult) error) error {
	if err := request.Validate(); err != nil {
		return err
	}
	if s.lifecycleEffectPending(run.RunID) {
		return errLifecycleEffectPending
	}
	request = request.Clone()
	// Isolated initialization/tests await the identical background-result path;
	// the running actor always returns immediately and consumes workerMessages.
	if s.workerMessages == nil {
		result := make(chan lifecycleEffectResult, 1)
		go func() { result <- invokeLifecycleEffect(ctx, port, request, reconcile) }()
		received := <-result
		if s.candidateBusinessClosed(run) && !retiredResourceAction(executionapi.LifecycleAction(request.Operation.Action), request.Input) {
			return s.applyRetiredLifecycleReceipt(run, received)
		}
		return apply(run, received)
	}
	if s.lifecycleEffects == nil {
		s.lifecycleEffects = make(map[string]pendingLifecycleEffect)
	}
	pending := pendingLifecycleEffect{runID: run.RunID, apply: apply}
	if owner, ok := port.(executionapi.LifecyclePreparationOwner); ok {
		pending.release = owner.ReleasePreparation
	}
	s.lifecycleEffects[request.Operation.ID] = pending
	messages, runID := s.workerMessages, run.RunID
	jobCtx := s.runContext
	if jobCtx == nil {
		jobCtx = ctx
	}
	deliveryCtx := jobCtx
	deadline, bounded := ctx.Deadline()
	s.forwarders.Add(1)
	go func() {
		defer s.forwarders.Done()
		if bounded {
			var cancel context.CancelFunc
			jobCtx, cancel = context.WithDeadline(jobCtx, deadline)
			defer cancel()
		}
		result := invokeLifecycleEffect(jobCtx, port, request, reconcile)
		select {
		case messages <- workerMessage{runID: runID, effect: &result}:
		case <-deliveryCtx.Done():
			if result.observer != nil {
				_ = result.observer.Close(context.Background())
			}
			if owner, ok := port.(executionapi.LifecyclePreparationOwner); ok {
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
		return ErrWorkerTurnStale
	}
	if !ok || pending.runID != runID {
		return discard()
	}
	run := s.find(runID)
	if run == nil || s.workflowRecord(run) == nil || run.AttemptID != result.request.Snapshot.Session.AttemptID || run.ExecutionID != result.request.Snapshot.Session.ExecutionID {
		return discard()
	}
	action := executionapi.LifecycleAction(result.request.Operation.Action)
	if s.candidateBusinessClosed(run) && action != executionapi.LifecycleCleanup && action != executionapi.LifecycleStopExecution && action != executionapi.LifecycleStopObservation && action != executionapi.LifecycleAcknowledge && !(action == executionapi.LifecycleHostAction && result.request.Input.Host != nil && (result.request.Input.Host.Kind == executionapi.HostCleanupProbeFiles || result.request.Input.Host.Kind == executionapi.HostRemoveFeedback)) {
		return s.applyRetiredLifecycleReceipt(run, result)
	}
	err := pending.apply(run, result)
	if errors.Is(err, errLifecycleEffectPending) || errors.Is(err, errLifecycleNeedsReconciliation) || errors.Is(err, errLifecycleResourcePending) {
		return nil
	}
	return err
}
