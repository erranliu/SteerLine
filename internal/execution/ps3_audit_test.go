package execution

import (
	"context"
	"fmt"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	resourceprocess "github.com/erranliu/SteerLine/internal/execution/process"
	"testing"
)

type ps3AuditStopBackend struct {
	executionbackend.ExecutionBackend
	calls       int
	exited      bool
	unsupported bool
}

func (b *ps3AuditStopBackend) Observe(_ context.Context, ref executionbackend.ExecutionRef) (executionbackend.Observation, error) {
	live := "live"
	if b.exited {
		live = "exited"
	}
	return executionbackend.Observation{Liveness: live, ObservedAt: now(), Source: "controlled original stop identity"}, nil
}
func (b *ps3AuditStopBackend) StopExecution(_ context.Context, ref executionbackend.ExecutionRef, _ executionbackend.Operation, handle executionbackend.StopHandle) (executionbackend.StopReceipt, error) {
	if handle == nil || handle.Identity() != ref.External {
		return executionbackend.StopReceipt{}, fmt.Errorf("original stop handle differs")
	}
	b.calls++
	if b.unsupported {
		return executionbackend.StopReceipt{}, executionbackend.UnsupportedAdapterOperation("StopExecution", "controlled remote cancellation unavailable")
	}
	b.exited = true
	return executionbackend.StopReceipt{Disposition: "confirmed"}, nil
}
func TestPS3AuditDiscardStopsSupportedHostsAndSettlesTerminalAttempt(t *testing.T) {
	for _, mode := range []string{"supported_nonlocal_host", "owned_handle_only", "terminal_attempt", "supported_non_os", "unsupported_cancel", "restart_absent_terminal"} {
		t.Run(mode, func(t *testing.T) {
			host := "local"
			if mode == "supported_nonlocal_host" || mode == "unsupported_cancel" {
				host = "orca"
			}
			s, run := neutralAuditService(t, host)
			db := testSQLite(s)
			ctx := context.Background()
			c := s.Config
			key := ExecutionRegistrationKey{Backend: executionbackend.BackendLocal, Protocol: neutralProtocol, Agent: "fixture"}
			entry := s.runtime.Directory.entries[key]
			backend := &ps3AuditStopBackend{ExecutionBackend: entry.Backend(c)}
			backend.unsupported = mode == "unsupported_cancel"
			backend.exited = mode == "restart_absent_terminal"
			entry.Backend = func(Config) executionbackend.ExecutionBackend { return backend }

			s.runtime.Directory.entries[key] = entry
			identity, err := resourceprocess.CurrentIdentity()
			require(t, err)
			if mode == "supported_non_os" {
				identity.Kind = "fixture"
				identity.RuntimeID = "controlled-original"
			}
			e := s.executionRecord(run)
			e.Process = identity
			e.Status = "active"
			e.Workspace.Worktree = e.WorkspaceIntent.Path
			e.Workspace.Branch = e.WorkspaceIntent.Branch
			e.Workspace.Base = e.WorkspaceIntent.BaseCommit
			e.PolicyResources = neutralPolicyReceipt(s.workflowRecord(run).ExecutionProfile.Digest, e.Workspace.Worktree, e)
			attempt := s.workflowAttempt(run, run.AttemptID)
			attempt.Status = "running"
			if mode == "terminal_attempt" || mode == "restart_absent_terminal" {
				attempt.Status = "succeeded"
			}
			s.refreshRun(run)
			require(t, s.commitWorker(run))
			ref := executionbackend.ExecutionRef{Version: executionbackend.ExecutionRecordVersion, Backend: executionbackend.BackendLocal, Protocol: neutralProtocol, Agent: AgentFor(c), Target: e.Target, RunID: run.RunID, AttemptID: run.AttemptID, WorkspacePath: e.Workspace.Worktree, WorkspaceBranch: e.Workspace.Branch, BaseCommit: e.Workspace.Base, External: *identity, Generation: e.SessionGeneration}
			require(t, s.commitWorkerBinding(run, ref))
			task := nativeTaskRecord(s.domain.Tasks, run.TaskID)
			result, err := s.commandService(c.Identity()).DiscardNativeTask(ctx, c, task.TaskID, newID(), task.Version, "audit retain resources", "operator")
			require(t, err)
			if result.Status != "discarded" {
				t.Fatalf("discard: %+v", result)
			}
			require(t, s.refreshNativeSnapshot(ctx))
			handle := &recordedStopProcess{identity: *identity, beforeKill: func() {}}
			if mode == "owned_handle_only" {
				s.ownedProcesses[run.RunID] = handle
			} else if mode != "restart_absent_terminal" {
				s.workers[run.RunID] = &Worker{proc: handle}
			}
			require(t, s.reconcileDiscardedExecutionStops(ctx))
			require(t, s.reconcileDiscardedExecutionStops(ctx))
			graph, err := db.TaskGraphSnapshot(ctx, c.Identity())
			require(t, err)
			if mode == "unsupported_cancel" {
				if backend.calls != 1 || len(graph.ByID[task.TaskID].StopPending) != 1 || !s.hasActiveWorkers() {
					t.Fatal("unsupported stop was retried or invented absence")
				}
				return
			}
			expectedCalls := 1
			if mode == "restart_absent_terminal" {
				expectedCalls = 0
			}
			if backend.calls != expectedCalls || len(graph.ByID[task.TaskID].StopPending) != 0 {
				t.Fatalf("original stop not settled exactly once: calls=%d pending=%v error=%s", backend.calls, graph.ByID[task.TaskID].StopPending, deref(s.executionRecord(run).Error))
			}
			if (mode == "terminal_attempt" || mode == "restart_absent_terminal") && s.workflowAttempt(run, run.AttemptID).Status != "succeeded" {
				t.Fatal("physical stop rewrote completed business attempt")
			}
			if s.hasActiveWorkers() {
				t.Fatal("confirmed stop retained an active dispatch slot")
			}
			if worker := s.workers[run.RunID]; worker != nil {
				ev := WorkerEvent{RunID: run.RunID, AttemptID: run.AttemptID, Summary: "late original output after physical stop"}
				require(t, s.handleWorkerMessage(Signal{runID: run.RunID, worker: worker, event: &ev}))
				if deref(s.executionRecord(run).LastEvent) != ev.Summary || s.executionRecord(run).Status != "stopped" {
					t.Fatal("late output lost or physical stop undone")
				}
			}
		})
	}
}
