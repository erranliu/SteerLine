package integration

import (
	"context"
	"testing"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
)

func TestWorkerServiceStopRecoveryControlled(t *testing.T) {
	t.Run("local_stops_and_requires_original_session_consent", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
		defer cancel()
		f := newLifecycleProjectBase(t, "local", true)
		t.Setenv("STEERLINE_INTEGRATION_HOLD_FIRST_TURN", "1")
		createIntegrationTask(t, ctx, f, 901)
		service := startIntegrationService(t, ctx, f.config, f.config.ExecutionDirectory)
		defer func() { service.stop(t) }()
		claimIntegrationTask(t, ctx, f, 901, service.instanceID)
		active := waitIntegrationRun(t, ctx, service, f.config, func(run *reconstructiontypes.RunView, e *executionmodel.ExecutionRecord) bool {
			return e.CurrentTurnID != "" && e.CurrentTurnID != e.LastStoppedTurnID && e.TurnNumber == 1
		})
		oldProcess, oldAttempt, oldSession := *active.execution.Process, active.run.AttemptID, active.execution.SessionID
		if active.runtime.WorkflowRuns[0].ExecutionProfile.ServiceStopBehavior != executionbackend.StopExecutionOnServiceStop {
			t.Fatal("local policy was not frozen")
		}
		service.stop(t)
		waitFor(t, 5*time.Second, func() bool { return nativeLiveProcessState(&oldProcess) == "process absent" })
		service = startIntegrationService(t, ctx, f.config, f.config.ExecutionDirectory)
		recovered := waitIntegrationRun(t, ctx, service, f.config, func(run *reconstructiontypes.RunView, e *executionmodel.ExecutionRecord) bool {
			return e.RecoveryState == "interrupted" && run.Decision != nil && run.Decision.Status == "pending"
		})
		if recovered.execution.TurnNumber != 1 || recovered.run.AttemptID != oldAttempt || recovered.execution.SessionID != oldSession || len(recovered.runtime.Attempts) != 1 {
			t.Fatal("reopen dispatched before fresh manual consent")
		}
		for _, op := range recovered.runtime.Operations {
			if op.Kind == "stop_worker_observation" {
				t.Fatal("normal stop persisted an interrupt RPC")
			}
		}
		submitIntegrationAnswerOption(t, ctx, f.config, recovered.run, "continue")
		continued := waitIntegrationRun(t, ctx, service, f.config, func(run *reconstructiontypes.RunView, e *executionmodel.ExecutionRecord) bool {
			return run.AttemptID != oldAttempt && e.TurnNumber >= 2 && e.CurrentTurnID == e.LastStoppedTurnID
		})
		if len(continued.runtime.Attempts) != 2 || continued.execution.SessionID != oldSession || *continued.execution.Process == oldProcess || continued.execution.Workspace.Worktree != active.execution.Workspace.Worktree {
			t.Fatal("manual continuation replaced the original session/workspace or repeated an Attempt")
		}
		assertNoTaskDelivery(t, continued.runtime, continued.run)
	})
}
