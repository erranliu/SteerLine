package execution

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	contracts "github.com/erranliu/SteerLine/internal/steerline/contracts"
)

type simplifiedSlowPort struct {
	executionbackend.LifecyclePort
	entered, release chan struct{}
	calls            atomic.Int32
}

type simplifiedCleanupPort struct{ executionbackend.LifecyclePort }

func (p simplifiedCleanupPort) Execute(_ context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, executionbackend.LifecycleObserver, error) {
	target := request.Snapshot.Ref.Target
	target.CoordinatorTerminal = "cleanup-private-progress"
	return executionbackend.LifecycleReceipt{Operation: request.Operation, Disposition: executionbackend.OperationConfirmed, Target: &target}, nil, nil
}

func TestSimplifiedCleanupRetainsFrozenDeliveryEvidence(t *testing.T) {
	s, run := neutralAuditService(t)
	lifecycle, fixed, err := s.lifecycleForRun(run)
	require(t, err)
	original := fixed.execution.Target
	// A discarded task exercises the same settled evidence guard as delivery.
	task := nativeTaskRecord(s.domain.Tasks, run.TaskID)
	_, err = (newTestCommandService(s.commandStore, s.Config)).DiscardNativeTask(context.Background(), s.Config, run.TaskID, newID(), task.Version, "fixture settlement", "operator")
	require(t, err)
	require(t, s.refreshNativeSnapshot(context.Background()))
	run.TaskStatus = "discarded"
	candidate := executionbackend.LifecycleCandidate{Action: executionbackend.LifecycleCleanup, Input: executionbackend.LifecycleInput{Cleanup: &executionbackend.CleanupInput{Resources: []executionbackend.ProcessIdentity{{Kind: "fixture", Terminal: "original"}}}}}
	require(t, s.executeLifecycleStep(context.Background(), run, simplifiedCleanupPort{lifecycle}, candidate))
	stored, err := s.domainStore.Query(context.Background(), s.Config.Identity())
	require(t, err)
	if stored.Executions[0].Target != original {
		t.Fatal("cleanup receipt rewrote delivered execution evidence")
	}
	confirmed := false
	for _, operation := range stored.Operations {
		if operation.Kind == string(executionbackend.LifecycleCleanup) && operation.Status == "confirmed" {
			confirmed = true
		}
	}
	if !confirmed {
		t.Fatal("original cleanup receipt was not committed")
	}
}

func (p *simplifiedSlowPort) Execute(ctx context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, executionbackend.LifecycleObserver, error) {
	p.calls.Add(1)
	close(p.entered)
	select {
	case <-p.release:
	case <-ctx.Done():
		return executionbackend.LifecycleReceipt{Operation: request.Operation, Disposition: executionbackend.OperationUnknown}, nil, ctx.Err()
	}
	return executionbackend.LifecycleReceipt{Operation: request.Operation, Disposition: executionbackend.OperationConfirmed}, nil, nil
}

func (p *simplifiedSlowPort) Reconcile(_ context.Context, request executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, executionbackend.LifecycleObserver, error) {
	return executionbackend.LifecycleReceipt{Operation: request.Operation, Disposition: executionbackend.OperationUnknown}, nil, nil
}

func TestSimplifiedLifecycleActorDoesNotWaitForEffect(t *testing.T) {
	for _, stale := range []bool{false, true} {
		t.Run(map[bool]string{false: "original_receipt", true: "stale_attempt"}[stale], func(t *testing.T) {
			s, run := neutralAuditService(t)
			input := executionbackend.LifecycleInput{Resource: &executionbackend.ResourceIntent{Purpose: "fixture"}}
			_, err := s.startWorkerOperation(run, string(executionbackend.LifecycleCreateResource), "slow-effect", input.Resource)
			require(t, err)
			fixed, err := s.workerFlowContext(run)
			require(t, err)
			snapshot, err := s.lifecycleSnapshot(run, fixed.workflow, fixed.execution, fixed.attempt, nil)
			require(t, err)
			request := lifecycleRequest(snapshot, *s.operationByKey("slow-effect"), executionbackend.LifecycleCreateResource, input)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			s.runContext = ctx
			s.signalQueue = make(chan Signal, 8)
			port := &simplifiedSlowPort{entered: make(chan struct{}), release: make(chan struct{})}
			applied := 0
			err = s.dispatchLifecycleEffect(ctx, run, port, request, false, func(run *contracts.Run, result lifecycleEffectResult) error {
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
			case message := <-s.signalQueue:
				err = s.handleWorkerMessage(message)
				if stale {
					if !errors.Is(err, executionmodel.ErrTurnStale) || applied != 0 {
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
