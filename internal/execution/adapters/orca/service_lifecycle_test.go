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
	l := &ExecutionLifecycle{setup: executionbackend.LifecycleSetup{Snapshot: snapshot.Clone()}, orca: profile, client: New(Config{Output: output})}
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

func TestServiceRecoveryNativeStartUsesOnlyOriginalReceipt(t *testing.T) {
	for _, stateName := range []string{"absent", "pending", "completed"} {
		t.Run(stateName, func(t *testing.T) {
			snapshot, state := nativeTestSnapshot(t, "agent")
			native := ProfileConfig{Executable: "orca", Shell: "cmd.exe", WaitTimeout: "1m", Model: "opaque-model", ReasoningEffort: "medium"}
			calls := []string{}
			port := ExecutionLifecycle{orca: native}
			requestID := nativeRequestID("original-operation")
			port.client = New(Config{Output: func(_ context.Context, _ time.Duration, args ...string) ([]byte, error) {
				calls = append(calls, args[1])
				switch args[1] {
				case "request-show":
					if flagValue(args, "--request") != requestID {
						t.Fatal("changed original request")
					}
					receipt := map[string]any{"runId": "run", "taskId": "task", "dispatchId": "dispatch", "state": "ready", "stage": "input_accepted", "turnStart": "observed", "mode": map[string]string{"mode": "terminal"}, "launch": map[string]any{"effective": map[string]string{"agent": "agent", "model": "opaque-model", "effort": "medium"}}}
					return nativeTestEnvelope(t, map[string]any{"requestId": requestID, "state": stateName, "method": "orchestration.workerStart", "receipt": receipt}), nil
				case "worker-show":
					terminal := Terminal{Handle: "terminal", IncarnationID: "inc", WorktreeID: "workspace", WorktreePath: snapshot.Workspace.Path, Branch: "branch", AgentIdentity: "agent", Connected: true, Status: "running"}
					return nativeTestEnvelope(t, map[string]any{"dispatch": map[string]string{"id": "dispatch", "runId": "run", "taskId": "task"}, "worker": map[string]string{"dispatchId": "dispatch", "agentTerminalHandle": "terminal"}, "terminal": terminal}), nil
				default:
					t.Fatalf("recovery created input or resources: %v", args)
					return nil, errors.New("unexpected mutation")
				}
			}})
			_, started, process, err := port.reconcileNativeWorkerStart(context.Background(), snapshot, state, NativeTurnState{TaskID: "task"}, "original-operation")
			if stateName != "completed" {
				if err == nil || started.DispatchID != "" || process.Terminal != "" || len(calls) != 1 {
					t.Fatal("unknown request became an execution")
				}
			} else if err != nil || started.DispatchID != "dispatch" || process.Terminal != "terminal" || len(calls) != 2 {
				t.Fatalf("original receipt was not bound: %+v/%v", process, err)
			}
		})
	}
	// An intent journaled locally but never received by Orca stays unknown.
	snapshot, state := nativeTestSnapshot(t, "agent")
	turn := NativeTurnState{TurnID: stableIDValue(4), TaskID: "task", Purpose: "business"}
	state.Turns = []NativeTurnState{turn}
	port := ExecutionLifecycle{orca: ProfileConfig{Executable: "orca", Shell: "cmd.exe", WaitTimeout: "1m", Model: "opaque-model", ReasoningEffort: "medium"}}
	envelopes, err := port.targetEnvelope(snapshot, snapshot.Ref.Target, snapshot.Ref.External, state)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Implementation = envelopes
	port.client = New(Config{Output: func(_ context.Context, _ time.Duration, args ...string) ([]byte, error) {
		if args[1] != "request-show" {
			t.Fatalf("unsent original input was dispatched during recovery: %v", args)
		}
		return nativeTestEnvelope(t, map[string]string{"requestId": nativeRequestID("original-operation"), "state": "absent"}), nil
	}})
	request := executionbackend.LifecycleRequest{Snapshot: snapshot, Operation: executionbackend.Operation{ID: "original-operation"}, Input: executionbackend.LifecycleInput{Turn: &executionbackend.TurnInput{CommandID: turn.TurnID, Prompt: "never sent"}}}
	receipt, observer, err := port.reconcileStart(context.Background(), request)
	if err == nil || observer != nil || receipt.Disposition != executionbackend.OperationUnknown || receipt.State.InputAccepted || receipt.State.TurnStarted {
		t.Fatal("locally persisted unsent intent became a worker")
	}
}
