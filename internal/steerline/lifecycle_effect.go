package steerline

import (
	"context"
	"errors"

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
	request   executionbackend.LifecycleRequest
	receipt   executionbackend.LifecycleReceipt
	observer  executionbackend.LifecycleObserver
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
func invokeLifecycleEffect(ctx context.Context, port executionbackend.LifecyclePort, request executionbackend.LifecycleRequest, reconcile bool) lifecycleEffectResult {
	result := lifecycleEffectResult{request: request.Clone()}
	if reconcile {
		result.receipt, result.observer, result.err = port.Reconcile(ctx, request.Clone())
	} else {
		result.receipt, result.observer, result.err = port.Execute(ctx, request.Clone())
	}
	result.uncertain = !reconcile && (result.receipt.Disposition == executionbackend.OperationUnknown || result.err != nil && result.receipt.Disposition != executionbackend.OperationRejected)
	if result.uncertain {
		if result.observer != nil {
			_ = result.observer.Close(context.Background())
			result.observer = nil
		}
		receipt, observer, err := port.Reconcile(ctx, request.Clone())
		if err == nil && receipt.ValidateFor(request) == nil && receipt.Disposition != executionbackend.OperationUnknown {
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

func (s *Service) dispatchLifecycleEffect(ctx context.Context, run *Run, port executionbackend.LifecyclePort, request executionbackend.LifecycleRequest, reconcile bool, apply func(*Run, lifecycleEffectResult) error) error {
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
		if s.candidateBusinessClosed(run) && !retiredResourceAction(executionbackend.LifecycleAction(request.Operation.Action), request.Input) {
			return s.applyRetiredLifecycleReceipt(run, received)
		}
		return apply(run, received)
	}
	if s.lifecycleEffects == nil {
		s.lifecycleEffects = make(map[string]pendingLifecycleEffect)
	}
	pending := pendingLifecycleEffect{runID: run.RunID, apply: apply}
	if owner, ok := port.(executionbackend.LifecyclePreparationOwner); ok {
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
	action := executionbackend.LifecycleAction(result.request.Operation.Action)
	if s.candidateBusinessClosed(run) && action != executionbackend.LifecycleCleanup && action != executionbackend.LifecycleStopExecution && action != executionbackend.LifecycleStopObservation && action != executionbackend.LifecycleAcknowledge && !(action == executionbackend.LifecycleHostAction && result.request.Input.Host != nil && (result.request.Input.Host.Kind == executionbackend.HostCleanupProbeFiles || result.request.Input.Host.Kind == executionbackend.HostRemoveFeedback)) {
		return s.applyRetiredLifecycleReceipt(run, result)
	}
	err := pending.apply(run, result)
	if errors.Is(err, errLifecycleEffectPending) || errors.Is(err, errLifecycleNeedsReconciliation) || errors.Is(err, errLifecycleResourcePending) {
		return nil
	}
	return err
}
