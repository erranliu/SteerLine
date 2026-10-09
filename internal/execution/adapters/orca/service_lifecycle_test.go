package orcaworker

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func observationFixture(t *testing.T, output func(context.Context, time.Duration, ...string) ([]byte, error)) (*ExecutionLifecycle, executionbackend.LifecycleSnapshot) {
	t.Helper()
	profile := ProfileConfig{Executable: "owned-orca"}
	identity := executionbackend.ProcessIdentity{Kind: "orca", Terminal: "agent", RuntimeID: "runtime", Incarnation: "inc", WorktreeID: "worktree"}
	target := executionbackend.ExecutionTarget{ID: NativeTargetID(profile), OrchestrationRun: "native-run", CoordinatorTerminal: "coordinator"}
	snapshot := executionbackend.LifecycleSnapshot{Ref: executionbackend.ExecutionRef{Backend: executionbackend.BackendOrca, Protocol: executionbackend.ProtocolOrcaTerminalV2, Agent: executionbackend.AgentSpec{Name: "codex"}, External: identity, Target: target},
		Session:             executionbackend.SessionContext{ProjectID: "project", TaskID: "task", WorkflowRunID: stableIDValue(1), ExecutionID: stableIDValue(2), AttemptID: stableIDValue(3), CurrentTurnID: stableIDValue(4), Target: target, Generation: "inc"},
		ConfigurationDigest: "config", ResourceGeneration: "resources", Workspace: executionbackend.WorkspaceIntent{Path: t.TempDir()}}
	state := NativeExecutionState{Coordinator: executionbackend.ProcessIdentity{Terminal: "coordinator", RuntimeID: "runtime"},
		Turns: []NativeTurnState{{TurnID: snapshot.Session.CurrentTurnID, TaskID: "task", DispatchID: "dispatch", Purpose: "business"}}}
	l := &ExecutionLifecycle{orca: profile, client: New(Config{Output: output})}
	envelopes, err := l.targetEnvelope(snapshot, target, identity, state)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Implementation = envelopes
	return l, snapshot
}

func TestServiceStopWorkerFacts(t *testing.T) {
	t.Run("service_stop_does_not_call_platform", func(t *testing.T) {
		calls := 0
		l, snapshot := observationFixture(t, func(context.Context, time.Duration, ...string) ([]byte, error) {
			calls++
			return nil, errors.New("service observation stop must not mutate the platform")
		})
		facts, err := l.ShutdownLifecycle(context.Background(), snapshot, nil)
		if err != nil || calls != 0 || facts.Stopped || facts.WorkerAbsent || l.PolicyCapabilities().ServiceStopBehavior != executionbackend.ContinueExecutionOnServiceStop {
			t.Fatalf("continuing worker stop changed platform execution: %+v/%v, calls=%d", facts, err, calls)
		}
	})
	t.Run("exited_terminal_recovers_offline_done_from_original_inbox", func(t *testing.T) {
		mutations, inboxReads := 0, 0
		l, snapshot := observationFixture(t, func(_ context.Context, _ time.Duration, args ...string) ([]byte, error) {
			command := strings.Join(args, " ")
			result := ""
			switch {
			case strings.HasPrefix(command, "terminal show"):
				result = `{"terminal":{"handle":"agent","status":"exited","connected":false,"incarnationId":"inc","worktreeId":"worktree"}}`
			case strings.HasPrefix(command, "orchestration check"):
				inboxReads++
				result = `{"runId":"native-run","deliveryId":"delivery","messages":[{"id":"done","run_id":"native-run","from_handle":"agent","to_handle":"run:native-run","type":"worker_done","payload":{"taskId":"task","dispatchId":"dispatch","outcome":"succeeded"},"subject":"completed offline","body":"trusted original output","outcome":"succeeded"}]}`
			case strings.HasPrefix(command, "orchestration worker-show"):
				result = `{"dispatch":{"id":"dispatch","taskId":"task","runId":"native-run","status":"completed"},"worker":{"dispatchId":"dispatch","agentTerminalHandle":"agent"},"terminal":{"handle":"agent","incarnationId":"inc","worktreeId":"worktree"}}`
			default:
				mutations++
				return nil, errors.New("unexpected platform mutation: " + command)
			}
			return []byte(`{"ok":true,"result":` + result + `,"_meta":{"runtimeId":"runtime"}}`), nil
		})
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		receipt, observer, err := l.ObserveLifecycle(ctx, snapshot)
		if err != nil || observer == nil || receipt.State.WorkerLive {
			t.Fatalf("exited original terminal observation: %+v/%v", receipt, err)
		}
		defer observer.Close(context.Background())
		seenDone, seenSettlement := false, false
		for !seenSettlement {
			select {
			case event, ok := <-observer.Events():
				if !ok {
					t.Fatal("original observer closed before settlement")
				}
				if event.Fact.Kind == executionbackend.EventCompleted {
					seenDone = true
					if event.Fact.SourceEventID != "done" || event.DeliveryID != "delivery" || !event.Fact.Matched {
						t.Fatal("offline completion lost original message/Delivery identity")
					}
				}
				if event.Status == "settled" {
					seenSettlement = true
				}
				if event.Persisted != nil {
					event.Persisted <- nil
				}
				if event.Processed != nil {
					event.Processed <- nil
				}
			case <-ctx.Done():
				t.Fatal("offline inbox completion was not reconciled")
			}
		}
		if !seenDone || inboxReads != 1 || mutations != 0 {
			t.Fatalf("offline done replayed or fabricated: done=%v reads=%d mutations=%d", seenDone, inboxReads, mutations)
		}
	})
}
