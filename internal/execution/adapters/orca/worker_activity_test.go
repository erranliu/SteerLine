package orcaworker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func activityTestResult(intent time.Time) map[string]any {
	return map[string]any{
		"dispatch":    map[string]any{"id": "dispatch", "taskId": "task", "runId": "run"},
		"worker":      map[string]any{"dispatchId": "dispatch", "agentTerminalHandle": "terminal", "stage": "turn_start_unobserved"},
		"terminal":    map[string]any{"handle": "terminal", "incarnationId": "inc", "worktreeId": "workspace", "connected": true, "agentWait": nil},
		"observation": map[string]any{"exactWorker": true},
		"projection": map[string]any{
			"dispatchId": "dispatch", "stage": map[string]any{"activity": "working"},
			"liveness": map[string]any{"verdict": "live", "source": "agent_status", "observedAt": intent.Add(time.Second).UnixMilli()},
			"evidence": map[string]any{"liveStatus": "fresh"},
		},
	}
}

func TestOrcaDelayedStartRequiresFreshOriginalActivity(t *testing.T) {
	intent := time.Now().Add(-time.Second)
	for _, scenario := range []string{"working", "idle", "missing_projection", "stale", "before_input", "no_intent", "non_agent_source", "unknown", "not_exact", "disconnected", "orphaned", "exited", "wrong_runtime", "wrong_dispatch", "wrong_task", "wrong_run", "wrong_worker", "wrong_recipient", "wrong_handle", "wrong_incarnation", "wrong_workspace", "wrong_projection", "unreachable"} {
		t.Run(scenario, func(t *testing.T) {
			payload := activityTestResult(intent)
			terminal := payload["terminal"].(map[string]any)
			projection := payload["projection"].(map[string]any)
			liveness := projection["liveness"].(map[string]any)
			identity := DispatchIdentity{DispatchID: "dispatch", TaskID: "task", RunID: "run", RuntimeID: "runtime", Terminal: "terminal", Incarnation: "inc", WorktreeID: "workspace"}
			at := intent
			runtime := "runtime"
			wantErr := false
			switch scenario {
			case "idle":
				projection["stage"].(map[string]any)["activity"] = "idle"
			case "missing_projection":
				delete(payload, "projection")
			case "stale":
				projection["evidence"].(map[string]any)["liveStatus"] = "stale"
			case "before_input":
				liveness["observedAt"] = intent.Add(-time.Second).UnixMilli()
			case "no_intent":
				at = time.Time{}
			case "non_agent_source":
				liveness["source"] = "terminal_output"
			case "unknown":
				liveness["verdict"] = "unverifiable"
			case "not_exact":
				payload["observation"].(map[string]any)["exactWorker"] = false
			case "disconnected":
				terminal["connected"] = false
			case "orphaned":
				terminal["orphaned"] = true
			case "exited":
				terminal["status"] = "exited"
			case "wrong_runtime":
				runtime, wantErr = "foreign", true
			case "wrong_dispatch", "wrong_task", "wrong_run":
				key := map[string]string{"wrong_dispatch": "id", "wrong_task": "taskId", "wrong_run": "runId"}[scenario]
				payload["dispatch"].(map[string]any)[key], wantErr = "foreign", true
			case "wrong_worker", "wrong_recipient":
				key := map[string]string{"wrong_worker": "dispatchId", "wrong_recipient": "agentTerminalHandle"}[scenario]
				payload["worker"].(map[string]any)[key], wantErr = "foreign", true
			case "wrong_handle", "wrong_incarnation", "wrong_workspace":
				key := map[string]string{"wrong_handle": "handle", "wrong_incarnation": "incarnationId", "wrong_workspace": "worktreeId"}[scenario]
				terminal[key], wantErr = "foreign", true
			case "wrong_projection":
				projection["dispatchId"] = "foreign"
			case "unreachable":
				wantErr = true
			}
			started, err := ObserveWorkerStarted(context.Background(), func(_ context.Context, args ...string) (Result, error) {
				if flagValue(args, "--dispatch") != identity.DispatchID || args[1] != "worker-show" {
					t.Fatalf("start observation mutated or changed the original input: %v", args)
				}
				if scenario == "unreachable" {
					return Result{}, errors.New("contact lost")
				}
				raw, err := json.Marshal(payload)
				return Result{RuntimeID: runtime, Payload: raw}, err
			}, identity, at)
			if started != (scenario == "working") || (err != nil) != wantErr {
				t.Fatalf("start proof: started=%v err=%v", started, err)
			}
		})
	}
}

func TestOrcaObserverRecordsDelayedStartBeforeWorkerDone(t *testing.T) {
	snapshot, state := nativeTestSnapshot(t, "agent")
	turn := NativeTurnState{TurnID: stableIDValue(7), TaskID: "task", DispatchID: "dispatch", Purpose: "business", IntentAt: time.Now().Add(-time.Second)}
	state.Turns = []NativeTurnState{turn}
	process := executionbackend.ProcessIdentity{Kind: "orca", RuntimeID: "runtime", Terminal: "terminal", Incarnation: "inc", WorktreeID: "workspace", Task: "task", Dispatch: "dispatch", OrchestrationRun: "run"}
	port := &ExecutionLifecycle{}
	snapshot.Session.PendingCommandID, snapshot.Session.PendingTurnID = turn.TurnID, turn.TurnID
	snapshot = port.boundSnapshot(snapshot, snapshot.Ref.Target, process, state)
	reads := 0
	hostChecks := make(chan struct{}, 8)
	port.client = New(Config{Output: func(_ context.Context, _ time.Duration, args ...string) ([]byte, error) {
		switch args[1] {
		case "check":
			// No worker_done or status message: activity alone must advance the turn.
			return nativeTestEnvelope(t, map[string]any{"runId": "run", "messages": []any{}}), nil
		case "worker-show":
			reads++
			payload := activityTestResult(turn.IntentAt)
			if reads == 1 {
				payload["projection"].(map[string]any)["stage"].(map[string]any)["activity"] = "idle"
			}
			return nativeTestEnvelope(t, payload), nil
		case "show":
			hostChecks <- struct{}{}
			return nativeTestEnvelope(t, map[string]any{"terminal": activityTestResult(turn.IntentAt)["terminal"]}), nil
		default:
			return nil, errors.New("unexpected mutation: " + args[1])
		}
	}})
	observer := newNativeObserver(context.Background(), port, snapshot)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := observer.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	select {
	case event := <-observer.Events():
		if reads != 2 || event.Fact.Kind != executionbackend.EventStarted || event.Fact.StartedTurnID != turn.TurnID || event.Fact.Completed {
			t.Fatalf("delayed turn was not started before completion: %+v", event)
		}
		saved := snapshot.Clone()
		saved.Implementation = event.Implementation
		restored, found, err := port.readState(saved)
		if err != nil || !found || !restored.Turns[0].Started {
			t.Fatalf("start evidence was not persisted for recovery: %+v %v", restored, err)
		}
		event.Persisted <- nil
		event.Processed <- nil
	case err := <-observer.Done():
		t.Fatalf("observer stopped before start: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("late activity never advanced the pending turn")
	}
	// Wait through another observation cycle; the accepted start must be unique.
	for i := 0; i < 3; i++ {
		select {
		case <-hostChecks:
		case <-time.After(time.Second):
			t.Fatal("observer did not continue after the delayed start")
		}
	}
	select {
	case event := <-observer.Events():
		t.Fatalf("start repeated or fabricated completion: %+v", event)
	default:
	}
}
