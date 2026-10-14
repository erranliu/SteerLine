package execution

import (
	"context"
	"errors"
	"testing"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

type failedStartResolver struct{ calls int }

func (r *failedStartResolver) ResolveLifecycle(setup executionbackend.LifecycleSetup) (executionbackend.LifecyclePort, error) {
	return failedStartPort{LifecyclePort: &neutralLifecycle{Setup: setup}, owner: r}, nil
}

type failedStartPort struct {
	executionbackend.LifecyclePort
	owner *failedStartResolver
}

func (p failedStartPort) PolicyCapabilities() executionbackend.LifecyclePolicyCapabilities {
	capabilities := p.LifecyclePort.PolicyCapabilities()
	capabilities.SeparateTurnStart, capabilities.ResumeActiveObservation = true, true
	return capabilities
}

func (p failedStartPort) Execute(_ context.Context, r executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, executionbackend.LifecycleObserver, error) {
	p.owner.calls++
	return executionbackend.LifecycleReceipt{Operation: r.Operation, Disposition: executionbackend.OperationUnknown}, nil, errors.New("unexpected duplicate start")
}

func (p failedStartPort) Reconcile(ctx context.Context, r executionbackend.LifecycleRequest) (executionbackend.LifecycleReceipt, executionbackend.LifecycleObserver, error) {
	return p.Execute(ctx, r)
}

func (p failedStartPort) VerifyManualRecovery(context.Context, executionbackend.LifecycleSnapshot) (executionbackend.LifecycleRecoveryEvidence, error) {
	return executionbackend.LifecycleRecoveryEvidence{ManagedProcess: true, ProcessAbsent: true}, nil
}

func TestReleasedWorkerStartConvergesAcrossRestart(t *testing.T) {
	s, run := neutralAuditService(t)
	resolver := &failedStartResolver{}
	s.lifecycleResolver = resolver
	fixed, err := s.workerRecords(run)
	require(t, err)
	input := executionbackend.TurnInput{CommandID: newID(), Prompt: "original business input", Purpose: "business"}
	fixed.execution.PendingCommandID = input.CommandID
	require(t, s.commitWorker(run))
	key := "worker-turn:" + fixed.execution.ExecutionID + ":" + input.CommandID
	_, err = s.startWorkerOperation(run, "start_worker_turn", key, input)
	require(t, err)
	snapshot, err := s.lifecycleSnapshot(run, fixed.workflow, fixed.execution, fixed.attempt, &input)
	require(t, err)
	request := lifecycleRequest(snapshot, *s.operationByKey(key), executionbackend.LifecycleStartTurn, executionbackend.LifecycleInput{Turn: &input})
	receipt := executionbackend.LifecycleReceipt{Operation: request.Operation, Disposition: executionbackend.OperationRejected,
		Diagnostic: "Original worker start failed; exact resource released",
		State:      executionbackend.LifecycleStateFacts{Stopped: true, WorkerAbsent: true, WritersAbsent: true}}
	err = s.applyLifecycleStepReceipt(run, request, lifecycleEffectResult{receipt: receipt, uncertain: true})
	if err != nil {
		t.Fatalf("failed start did not settle its candidate: %v", err)
	}
	assertFailedStart := func(s *Service) {
		t.Helper()
		run = s.find(run.RunID)
		e := s.executionRecord(run)
		if e.PendingCommandID != "" || e.PendingTurnID != "" || s.operationByKey(key).Status != "rejected" || run.WorkflowStatus != "abandoned" || run.WorkflowStage != "start.failed" || run.Decision != nil {
			t.Fatalf("released start remained pending or offered automatic continuation: %+v / %+v", e, run.Decision)
		}
		for i := 0; i < 3; i++ {
			err := s.resumeWorkerFlow(context.Background())
			require(t, err)
			if resolver.calls != 0 {
				t.Fatal("released start was retried by later scheduling")
			}
		}
		if len(s.domain.Attempts) != 1 || len(s.domain.Executions) != 1 {
			t.Fatal("failed start created another execution")
		}
	}
	assertFailedStart(s)
	config := s.Config
	require(t, s.Close())
	reopened, err := newNeutralService(config)
	require(t, err)
	defer reopened.Close()
	reopened.lifecycleResolver = resolver
	require(t, reopened.markServiceRecoveryGate())
	assertFailedStart(reopened)
}
