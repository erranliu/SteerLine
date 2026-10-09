package execution

import (
	"context"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
)

type workerStartBinding struct {
	execution, attempt, generation, configDigest string
}

type workerStartCheckState struct {
	binding  workerStartBinding
	verified bool
}

type workerStartCheckResult struct {
	runID   string
	binding workerStartBinding
	err     error
}

func startCheckBinding(run *reconstructiontypes.RunView, execution *executionmodel.ExecutionRecord) workerStartBinding {
	return workerStartBinding{execution: run.ExecutionID, attempt: run.AttemptID, generation: execution.Generation, configDigest: execution.ConfigurationDigest}
}

func checkWorkerStart(ctx context.Context, config Config, lifecycle executionbackend.LifecyclePort, setup executionbackend.LifecycleSetup) error {
	config = copyWorkerFlowConfig(config)
	if err := config.BindPublicationTarget(ctx, setup.ProcessHost); err != nil {
		return err
	}
	report, err := lifecycle.PreflightLifecycle(ctx, setup)
	if err != nil {
		return err
	}
	return validateLifecycleStartCapability(setup.Snapshot.Ref, report)
}

// Resource/turn preparation must not repeat a synchronous external probe on
// the actor after claim admission. A verified check belongs to one generation.
func (s *Service) scheduleWorkerStartCheck(ctx context.Context, run *reconstructiontypes.RunView, config Config, lifecycle executionbackend.LifecyclePort, setup executionbackend.LifecycleSetup) error {
	binding := startCheckBinding(run, s.executionRecord(run))
	if state, found := s.workerStartChecks[run.RunID]; found && state.binding == binding {
		if state.verified {
			return s.prepareWorkerFlowResources(ctx, run)
		}
		return nil
	}
	if s.workerStartChecks == nil {
		s.workerStartChecks = make(map[string]workerStartCheckState)
	}
	s.workerStartChecks[run.RunID] = workerStartCheckState{binding: binding}
	runID, queue := run.RunID, s.signalQueue
	s.forwarders.Add(1)
	go func() {
		defer s.forwarders.Done()
		result := workerStartCheckResult{runID: runID, binding: binding, err: checkWorkerStart(ctx, config, lifecycle, setup)}
		select {
		case queue <- Signal{workerStartCheck: &result}:
		case <-ctx.Done():
		}
	}()
	return nil
}

func (s *Service) applyWorkerStartCheck(result workerStartCheckResult) error {
	state, found := s.workerStartChecks[result.runID]
	if !found || state.binding != result.binding {
		return executionmodel.ErrTurnStale
	}
	run := s.find(result.runID)
	var execution *executionmodel.ExecutionRecord
	if run != nil {
		execution = s.executionRecord(run)
	}
	if execution == nil || s.candidateBusinessClosed(run) || startCheckBinding(run, execution) != result.binding || execution.RecoveryRequired {
		delete(s.workerStartChecks, result.runID)
		return executionmodel.ErrTurnStale
	}
	ctx := s.runContext
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		delete(s.workerStartChecks, result.runID)
		return nil
	}
	if result.err != nil {
		delete(s.workerStartChecks, result.runID)
		execution.Status = "failed"
		s.refreshRun(run)
		return s.waitWorkerFlow(run, "Worker start preflight failed: "+result.err.Error())
	}
	state.verified = true
	s.workerStartChecks[result.runID] = state
	return s.prepareWorkerFlowResources(ctx, run)
}
