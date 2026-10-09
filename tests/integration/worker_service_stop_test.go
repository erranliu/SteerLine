package integration

import (
	"context"
	"testing"
	"time"

	steerlineapp "github.com/erranliu/SteerLine/internal/app"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"
	testprotocol "github.com/erranliu/SteerLine/internal/testkit/protocol"
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
	t.Run("orca_continues_and_recovers_completion_after_terminal_exit", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		f := newLifecycleProjectBase(t, "orca", true)
		require(t, testprotocol.WriteNativeOrcaState(f.nativeStateDir, testprotocol.NativeOrcaState{EnvironmentMode: true, ReadinessMode: "success", BusinessActive: true}))
		createIntegrationTask(t, ctx, f, 901)
		service := startIntegrationService(t, ctx, f.config, f.config.ExecutionDirectory)
		defer func() { service.stop(t) }()
		claimIntegrationTask(t, ctx, f, 901, service.instanceID)
		active := waitIntegrationRun(t, ctx, service, f.config, func(run *reconstructiontypes.RunView, e *executionmodel.ExecutionRecord) bool {
			return e.TurnNumber == 1 && e.CurrentTurnID != "" && e.CurrentTurnID != e.LastStoppedTurnID
		})
		oldAttempt, oldSession, oldTurn, oldProcess := active.run.AttemptID, active.execution.SessionID, active.execution.CurrentTurnID, *active.execution.Process
		if active.runtime.WorkflowRuns[0].ExecutionProfile.ServiceStopBehavior != executionbackend.ContinueExecutionOnServiceStop {
			t.Fatal("continuing policy was not frozen")
		}
		original, err := testprotocol.ReadNativeOrcaState(f.nativeStateDir)
		require(t, err)
		service.stop(t)
		afterStop, err := testprotocol.ReadNativeOrcaState(f.nativeStateDir)
		require(t, err)
		if len(afterStop.CloseRequests) != len(original.CloseRequests) || afterStop.ClosedAgent || afterStop.ClosedCoordinator {
			t.Fatal("service stop closed platform-owned execution")
		}
		service = startIntegrationService(t, ctx, f.config, f.config.ExecutionDirectory)
		reopened := waitIntegrationRun(t, ctx, service, f.config, func(run *reconstructiontypes.RunView, e *executionmodel.ExecutionRecord) bool {
			return e.RecoveryState == "running" || e.RecoveryState == "live"
		})
		if reopened.run.AttemptID != oldAttempt || reopened.execution.SessionID != oldSession || reopened.execution.CurrentTurnID != oldTurn || *reopened.execution.Process != oldProcess {
			t.Fatal("continuing worker was replaced on restart")
		}
		service.stop(t)
		// The platform produces an original completion while the center is offline.
		state, err := testprotocol.ReadNativeOrcaState(f.nativeStateDir)
		require(t, err)
		state.BusinessActive, state.AgentExited = false, true
		require(t, testprotocol.WriteNativeOrcaState(f.nativeStateDir, state))
		service = startIntegrationService(t, ctx, f.config, f.config.ExecutionDirectory)
		settled := waitIntegrationRun(t, ctx, service, f.config, func(run *reconstructiontypes.RunView, e *executionmodel.ExecutionRecord) bool {
			return e.LastStoppedTurnID == oldTurn && e.RecoveryState == "completed"
		})
		state, err = testprotocol.ReadNativeOrcaState(f.nativeStateDir)
		require(t, err)
		if settled.run.AttemptID != oldAttempt || settled.execution.SessionID != oldSession || state.Dispatches != original.Dispatches || state.Replies != original.Replies || state.Sends != original.Sends {
			t.Fatal("offline completion replaced or resent the original Dispatch")
		}
		assertNoTaskDelivery(t, settled.runtime, settled.run)
		originalDispatches, originalReplies, originalSends, originalAcks := state.Dispatches, state.Replies, state.Sends, len(state.Acked)
		service.stop(t)
		service = startIntegrationService(t, ctx, f.config, f.config.ExecutionDirectory)
		repeated := waitIntegrationRun(t, ctx, service, f.config, func(run *reconstructiontypes.RunView, e *executionmodel.ExecutionRecord) bool {
			return e.LastStoppedTurnID == oldTurn && e.RecoveryState == "completed"
		})
		state, err = testprotocol.ReadNativeOrcaState(f.nativeStateDir)
		require(t, err)
		if repeated.run.AttemptID != oldAttempt || state.Dispatches != originalDispatches || state.Replies != originalReplies || state.Sends != originalSends || len(state.Acked) != originalAcks {
			t.Fatal("repeated completion restart replayed platform operations")
		}

		// Query from the same persisted authority cannot claim TaskDelivery from done.
		page, err := steerlineapp.LoadDatabaseRuntime(f.config)
		require(t, err)
		if len(page.Attempts) != len(active.runtime.Attempts) {
			t.Fatal("completion invented another Attempt")
		}
	})

	t.Run("completed_live_orca_session_accepts_fresh_next_round", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
		defer cancel()
		f := newLifecycleProjectBase(t, "orca", true)
		require(t, testprotocol.WriteNativeOrcaState(f.nativeStateDir, testprotocol.NativeOrcaState{EnvironmentMode: true, ReadinessMode: "success"}))
		createIntegrationTask(t, ctx, f, 901)
		service := startIntegrationService(t, ctx, f.config, f.config.ExecutionDirectory)
		defer func() { service.stop(t) }()
		claimIntegrationTask(t, ctx, f, 901, service.instanceID)
		complete := waitIntegrationRun(t, ctx, service, f.config, func(run *reconstructiontypes.RunView, e *executionmodel.ExecutionRecord) bool {
			return e.TurnNumber == 1 && e.CurrentTurnID == e.LastStoppedTurnID && run.Decision != nil && run.Decision.Status == "pending"
		})
		oldAttempt, oldSession, oldProcess := complete.run.AttemptID, complete.execution.SessionID, *complete.execution.Process
		state, err := testprotocol.ReadNativeOrcaState(f.nativeStateDir)
		require(t, err)
		oldDispatches := state.Dispatches
		service.stop(t)
		service = startIntegrationService(t, ctx, f.config, f.config.ExecutionDirectory)
		recovered := waitIntegrationRun(t, ctx, service, f.config, func(run *reconstructiontypes.RunView, e *executionmodel.ExecutionRecord) bool {
			return e.RecoveryState == "completed" && run.Decision != nil && run.Decision.Status == "pending" && len(run.Decision.Request.Options) > 0
		})
		submitIntegrationAnswerOption(t, ctx, f.config, recovered.run, "continue")
		next := waitIntegrationRun(t, ctx, service, f.config, func(run *reconstructiontypes.RunView, e *executionmodel.ExecutionRecord) bool {
			return e.TurnNumber == 2 && e.CurrentTurnID == e.LastStoppedTurnID
		})
		state, err = testprotocol.ReadNativeOrcaState(f.nativeStateDir)
		require(t, err)
		if next.run.AttemptID != oldAttempt || next.execution.SessionID != oldSession || (next.execution.Process.Terminal != oldProcess.Terminal || next.execution.Process.RuntimeID != oldProcess.RuntimeID || next.execution.Process.Incarnation != oldProcess.Incarnation) || len(next.runtime.Attempts) != len(complete.runtime.Attempts) || state.Dispatches != oldDispatches+1 {
			t.Fatal("fresh feedback rebuilt a live original worker or duplicated the next Dispatch")
		}
	})

	t.Run("orca_original_pending_question_and_answer_receipt", func(t *testing.T) { testIntegratedNativeSessionGateWithMutations(t, false, false) })

}
