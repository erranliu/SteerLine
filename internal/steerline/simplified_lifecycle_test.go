package steerline

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

type simplifiedSlowPort struct {
	executionapi.LifecyclePort
	entered, release chan struct{}
	calls            atomic.Int32
}

func (p *simplifiedSlowPort) Execute(ctx context.Context, request executionapi.LifecycleRequest) (executionapi.LifecycleReceipt, executionapi.LifecycleObserver, error) {
	p.calls.Add(1)
	close(p.entered)
	select {
	case <-p.release:
	case <-ctx.Done():
		return executionapi.LifecycleReceipt{Operation: request.Operation, Disposition: executionapi.OperationUnknown}, nil, ctx.Err()
	}
	return executionapi.LifecycleReceipt{Operation: request.Operation, Disposition: executionapi.OperationConfirmed}, nil, nil
}
func (p *simplifiedSlowPort) Reconcile(_ context.Context, request executionapi.LifecycleRequest) (executionapi.LifecycleReceipt, executionapi.LifecycleObserver, error) {
	return executionapi.LifecycleReceipt{Operation: request.Operation, Disposition: executionapi.OperationUnknown}, nil, nil
}

func TestSimplifiedLifecycleActorDoesNotWaitForEffect(t *testing.T) {
	for _, stale := range []bool{false, true} {
		t.Run(map[bool]string{false: "original_receipt", true: "stale_attempt"}[stale], func(t *testing.T) {
			s, run := neutralAuditService(t)
			input := executionapi.LifecycleInput{Resource: &executionapi.ResourceIntent{Purpose: "fixture"}}
			_, err := s.startWorkerOperation(run, string(executionapi.LifecycleCreateResource), "slow-effect", input.Resource)
			require(t, err)
			fixed, err := s.workerFlowContext(run)
			require(t, err)
			snapshot, err := s.lifecycleSnapshot(run, fixed.workflow, fixed.execution, fixed.attempt, nil)
			require(t, err)
			request := lifecycleRequest(snapshot, *s.operationByKey("slow-effect"), executionapi.LifecycleCreateResource, input)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			s.runContext = ctx
			s.workerMessages = make(chan workerMessage, 8)
			port := &simplifiedSlowPort{entered: make(chan struct{}), release: make(chan struct{})}
			applied := 0
			err = s.dispatchLifecycleEffect(ctx, run, port, request, false, func(run *Run, result lifecycleEffectResult) error {
				if run != s.find(run.RunID) {
					return errors.New("effect captured an obsolete projection")
				}
				if err := result.receipt.ValidateFor(request); err != nil {
					return err
				}
				applied++
				return nil
			})
			if !errors.Is(err, errLifecycleEffectPending) {
				t.Fatalf("effect did not return acceptance: %v", err)
			}
			select {
			case <-port.entered:
			case <-time.After(time.Second):
				t.Fatal("effect was not scheduled")
			}
			if applied != 0 {
				t.Fatal("receipt applied before native work finished")
			}
			if err := s.dispatchLifecycleEffect(ctx, run, port, request, false, nil); !errors.Is(err, errLifecycleEffectPending) {
				t.Fatalf("duplicate effect: %v", err)
			}
			if stale {
				run.AttemptID = newID()
			} else {
				require(t, s.refreshNativeSnapshot(context.Background()))
			}
			close(port.release)
			select {
			case message := <-s.workerMessages:
				err = s.handleWorkerMessage(message)
				if stale {
					if !errors.Is(err, ErrWorkerTurnStale) || applied != 0 {
						t.Fatalf("stale receipt applied: %v / %d", err, applied)
					}
				} else {
					require(t, err)
					if applied != 1 {
						t.Fatal("original receipt not applied once")
					}
				}
			case <-time.After(time.Second):
				t.Fatal("result not delivered to actor")
			}
			s.forwarders.Wait()
			if port.calls.Load() != 1 {
				t.Fatal("duplicate native mutation")
			}
		})
	}
}
