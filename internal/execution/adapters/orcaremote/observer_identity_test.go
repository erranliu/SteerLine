package orca_remote_worker

import (
	"context"
	"errors"
	"testing"
	"time"

	orcaworker "github.com/erranliu/SteerLine/internal/execution/adapters/orca"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func TestRemoteEmptyInboxDetectsLostDispatchWithoutRelaunch(t *testing.T) {
	for _, timedOut := range []bool{false, true} {
		for _, changed := range []string{"attachment", "after_healthy_poll", "terminal", "incarnation", "branch", "runtime"} {
			t.Run(changed+map[bool]string{false: "/empty", true: "/timeout"}[timedOut], func(t *testing.T) {
				snapshot, state := remoteSnapshotForTest(t)
				state.Turns = []orcaworker.NativeTurnState{{TurnID: "turn", TaskID: "task", DispatchID: "dispatch"}}
				envelope, err := orcaworker.NewExecutionStateEnvelope(stateIdentity(snapshot), state)
				if err != nil {
					t.Fatal(err)
				}
				snapshot.Implementation = []executionbackend.ImplementationDataEnvelope{envelope}
				shows := 0
				config := lifecycleRemoteTestConfig(func(_ context.Context, _ time.Duration, args ...string) ([]byte, error) {
					if len(args) < 2 {
						t.Errorf("unexpected command: %v", args)
						return nil, errors.New("unexpected command")
					}
					switch args[1] {
					case "run-show":
						return remoteEnvelope("center-runtime", map[string]any{"run": map[string]any{"id": "run", "coordinator_handle": "coordinator"}}), nil
					case "check":
						return remoteEnvelope("center-runtime", map[string]any{"runId": "run", "messages": []any{}, "timedOut": timedOut}), nil
					case "worker-show":
						shows++
						terminal := map[string]any{"handle": "agent-terminal", "incarnationId": "incarnation", "worktreeId": "repo::worktree", "worktreePath": "/remote/worktree", "branch": "branch"}
						remoteRuntime := "remote-epoch"
						switch changed {
						case "terminal":
							terminal["handle"] = "second-session"
						case "incarnation":
							terminal["incarnationId"] = "replacement"
						case "branch":
							terminal["branch"] = "another-branch"
						case "runtime":
							remoteRuntime = "replacement-runtime"
						}
						exact := changed != "attachment" && (changed != "after_healthy_poll" || shows == 1)
						return remoteEnvelope("center-runtime", map[string]any{
							"dispatch":   map[string]string{"id": "dispatch", "runId": "run", "taskId": "task", "status": "dispatched"},
							"worker":     map[string]string{"dispatchId": "dispatch", "worktreeId": "repo::worktree", "agentTerminalHandle": "agent-terminal"},
							"projection": map[string]any{"host": map[string]string{"id": "environment"}, "liveness": map[string]string{"verdict": "live"}},
							"server":     map[string]string{"environmentId": "environment"}, "remoteRuntimeEpoch": remoteRuntime,
							"terminal": terminal, "observation": map[string]any{"exactWorker": exact},
						}), nil
					default:
						t.Errorf("identity loss issued a mutation or replacement start: %v", args)
						return nil, errors.New("unexpected mutation")
					}
				})
				lifecycle := &ExecutionLifecycle{setup: executionbackend.LifecycleSetup{Snapshot: snapshot}, config: config, client: New(config)}
				observer := newLifecycleObserver(context.Background(), lifecycle, snapshot)
				defer observer.Close(context.Background())
				select {
				case err := <-observer.Done():
					var failure *executionbackend.BackendError
					if !errors.As(err, &failure) || failure.Code != executionbackend.BackendIdentityMismatch {
						t.Fatalf("lost Dispatch was not surfaced: %v", err)
					}
				case <-time.After(2 * time.Second):
					t.Fatal("empty inbox left a detached Dispatch waiting forever")
				}
				wantShows := 1
				if changed == "after_healthy_poll" {
					wantShows = 2
				}
				if shows != wantShows {
					t.Fatalf("expected %d inspections without a replacement start, got %d", wantShows, shows)
				}
				for event := range observer.Events() {
					t.Fatalf("identity loss fabricated an execution fact: %+v", event.Fact)
				}
			})
		}
	}
}
