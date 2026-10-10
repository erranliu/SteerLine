package orca_remote_worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	orcaworker "github.com/erranliu/SteerLine/internal/execution/adapters/orca"
	b "github.com/erranliu/SteerLine/internal/execution/backend"
)

func resultFixture(t *testing.T) (b.LifecycleSnapshot, orcaworker.NativeTurnState, Message) {
	t.Helper()
	snapshot, _ := remoteSnapshotForTest(t)
	snapshot.Session.CurrentTurnID, snapshot.Session.CurrentCommandID = "00000000000000000000000000000004", "00000000000000000000000000000004"
	turn := orcaworker.NativeTurnState{TurnID: snapshot.Session.CurrentTurnID, InputVersion: 3, TaskID: "task", DispatchID: "dispatch", ResultProcess: &snapshot.Ref.External}
	raw, _ := json.Marshal(map[string]any{"taskId": "task", "dispatchId": "dispatch", "outcome": "succeeded", "steerline_result": declaredResult{Version: 1, CommandID: turn.TurnID, InputVersion: 3, Body: "完整结果🙂\n\"quoted\" \\ $() `literal`"}})
	return snapshot, turn, Message{ID: "done", RunID: "run", ToHandle: "run:run", Type: "worker_done", FromHandle: "agent-terminal", Body: "Summary only.", Payload: raw}
}

func TestSupervisedResultContract(t *testing.T) {
	snapshot, turn, message := resultFixture(t)
	captureResult(&turn, message, snapshot.Ref.External, "succeeded")
	if turn.FinalVerified || turn.FinalReadError != "" || !strings.Contains(turn.FinalText, "🙂") || turn.FinalText == message.Body {
		t.Fatalf("unsettled message or summary became final: %+v", turn)
	}
	turn.Settled = true
	evidence, err := settledResult(snapshot, &turn)
	if err != nil || !evidence.FinalVerified || evidence.FinalItemID != message.ID || evidence.FinalOrigin != "worker_declared" || evidence.FinalNativeTurnID != "dispatch" {
		t.Fatalf("original declared final was lost: %+v %v", evidence, err)
	}
	before := turn
	captureResult(&turn, message, snapshot.Ref.External, "succeeded")
	if turn.FinalText != before.FinalText || turn.ResultMessageDigest != before.ResultMessageDigest || !turn.FinalVerified {
		t.Fatal("same-message replay changed the frozen final")
	}
	message.Body = "changed original message"
	captureResult(&turn, message, snapshot.Ref.External, "succeeded")
	if evidence, err := settledResult(snapshot, &turn); err != nil || evidence.FinalVerified || !strings.Contains(evidence.FinalDiagnostic, "conflicting") || evidence.LastMessage != before.FinalText {
		t.Fatal("conflicting completion overwrote the original result")
	}
}

func TestSupervisedResultStringPayload(t *testing.T) {
	snapshot, turn, message := resultFixture(t)
	message.Payload, _ = json.Marshal(string(message.Payload))
	captureResult(&turn, message, snapshot.Ref.External, "succeeded")
	turn.Settled = true
	evidence, err := settledResult(snapshot, &turn)
	if err != nil || !evidence.FinalVerified || evidence.LastMessage != "完整结果🙂\n\"quoted\" \\ $() `literal`" {
		t.Fatalf("native string payload lost complete result: %+v %v", evidence, err)
	}
}

func TestSupervisedResultRejectsUnboundOrIncomplete(t *testing.T) {
	for _, bad := range []string{"missing", "unknown_field", "duplicate_field", "outer_duplicate", "outer_case", "outer_duplicate_identity", "version", "command", "input", "body", "sender", "run", "recipient", "task", "summary_size", "result_size", "process"} {
		t.Run(bad, func(t *testing.T) {
			snapshot, turn, message := resultFixture(t)
			var payload map[string]any
			_ = json.Unmarshal(message.Payload, &payload)
			result := payload["steerline_result"].(map[string]any)
			switch bad {
			case "missing":
				delete(payload, "steerline_result")
			case "unknown_field":
				result["unexpected"] = true
			case "duplicate_field":
				payload["steerline_result"] = json.RawMessage(`{"version":2,"version":1,"command_id":"` + turn.TurnID + `","input_version":3,"body":"x"}`)
			case "version":
				result["version"] = 2
			case "command":
				result["command_id"] = "other"
			case "input":
				result["input_version"] = 4
			case "body":
				result["body"] = " \n"
			case "sender":
				message.FromHandle = "other"
			case "run":
				message.RunID = "other"
			case "recipient":
				message.ToHandle = "run:other"
			case "task":
				payload["taskId"] = "old-task"
			case "summary_size":
				message.Body = strings.Repeat("x", MaxMessageBytes)
			case "result_size":
				result["body"] = strings.Repeat("中", MaxMessageBytes)
			case "process":
				changed := *turn.ResultProcess
				changed.Incarnation = "replacement"
				turn.ResultProcess = &changed
			}
			message.Payload, _ = json.Marshal(payload)
			resultJSON, _ := json.Marshal(payload["steerline_result"])
			switch bad {
			case "outer_duplicate":
				message.Payload = []byte(fmt.Sprintf(`{"taskId":"task","dispatchId":"dispatch","outcome":"succeeded","steerline_result":{},"steerline_result":%s}`, resultJSON))
			case "outer_case":
				message.Payload = []byte(fmt.Sprintf(`{"taskId":"task","dispatchId":"dispatch","outcome":"succeeded","STEERLINE_RESULT":%s}`, resultJSON))
			case "outer_duplicate_identity":
				message.Payload = []byte(fmt.Sprintf(`{"taskId":"other","taskId":"task","dispatchId":"dispatch","outcome":"succeeded","steerline_result":%s}`, resultJSON))
			}
			captureResult(&turn, message, snapshot.Ref.External, "succeeded")
			turn.Settled = true
			evidence, err := settledResult(snapshot, &turn)
			if err == nil && (evidence.FinalVerified || evidence.FinalDiagnostic == "") {
				t.Fatal("unbound or incomplete result was accepted")
			}
		})
	}
}

func TestSupervisedResultObserverPersistenceAndRestore(t *testing.T) {
	for _, target := range []string{"local", "environment"} {
		t.Run(target, func(t *testing.T) {
			snapshot, turn, message := resultFixture(t)
			message.FromHandle = "dispatch:dispatch"
			_, state := remoteSnapshotForTest(t)
			runtime := "center-runtime"
			if target == "local" {
				snapshot.Ref.Target.ID, snapshot.Ref.Target.ConnectionID, snapshot.Ref.Target.ExternalRuntime = (Config{Settings: Settings{Target: "local"}}).Target().ID, "", ""
				snapshot.Ref.Target.ExecutionHost = "local"
				snapshot.Ref.External.ExecutionHost = "local"
				snapshot.Ref.External.RuntimeID, snapshot.Ref.External.CoordinatorRuntimeID = "local-runtime", "local-runtime"
				state.RemoteRuntimeID, state.Coordinator.RuntimeID = "local-runtime", "local-runtime"
				runtime = "local-runtime"
			}
			snapshot.Session.Target = snapshot.Ref.Target
			snapshot.Session.SessionID = SessionID(snapshot.Ref.External)
			turn.ResultProcess = &snapshot.Ref.External
			state.Turns = []orcaworker.NativeTurnState{turn}
			worker := localWorker("task", "dispatch", "succeeded")
			worker["worker"].(map[string]string)["worktreeId"] = snapshot.Ref.External.WorktreeID
			worker["worker"].(map[string]string)["agentTerminalHandle"] = snapshot.Ref.External.Terminal
			worker["worker"].(map[string]string)["runtimeEpoch"] = snapshot.Ref.External.RuntimeID
			worker["terminal"] = map[string]string{"handle": snapshot.Ref.External.Terminal, "incarnationId": snapshot.Ref.External.Incarnation, "worktreeId": snapshot.Ref.External.WorktreeID, "worktreePath": snapshot.Ref.WorkspacePath, "branch": snapshot.Ref.WorkspaceBranch}
			worker["projection"].(map[string]any)["host"] = map[string]string{"id": snapshot.Ref.External.ExecutionHost}
			if target == "environment" {
				worker["server"] = map[string]string{"environmentId": "environment"}
				worker["remoteRuntimeEpoch"] = "remote-epoch"
			}
			config := lifecycleRemoteTestConfig(func(_ context.Context, _ time.Duration, args ...string) ([]byte, error) {
				switch args[1] {
				case "run-show":
					return remoteEnvelope(runtime, map[string]any{"run": map[string]string{"id": "run", "coordinator_handle": "coordinator"}}), nil
				case "terminal":
					return nil, errors.New("unexpected")
				case "show":
					return remoteEnvelope(runtime, map[string]any{"terminal": map[string]any{"handle": "coordinator", "incarnationId": state.Coordinator.Incarnation, "connected": true}}), nil
				case "check":
					return remoteEnvelope(runtime, map[string]any{"runId": "run", "deliveryId": "delivery", "messages": []Message{message}}), nil
				case "worker-show":
					return remoteEnvelope(runtime, worker), nil
				default:
					t.Fatalf("result recovery performed mutation: %v", args)
					return nil, nil
				}
			})
			if target == "local" {
				config.Settings = Settings{Target: "local", CoordinatorTerminal: "coordinator"}
			}
			port := &ExecutionLifecycle{setup: b.LifecycleSetup{Snapshot: snapshot}, config: config, client: New(config)}
			var err error
			snapshot.Implementation, err = port.stateEnvelopes(snapshot, state)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			observer := newLifecycleObserver(ctx, port, snapshot)
			defer observer.Close(context.Background())
			for {
				var event b.LifecycleEvent
				select {
				case value, ok := <-observer.Events():
					if !ok {
						t.Fatal("observer ended before persisting completion")
					}
					event = value
				case <-ctx.Done():
					t.Fatal("observer did not persist completion before deadline")
				}
				if event.Fact.Kind == b.EventCompleted {
					// Simulate a restart after durable storage but before processed/ack.
					wire, _ := json.Marshal(event.Implementation)
					_ = json.Unmarshal(wire, &snapshot.Implementation)
					event.Persisted <- nil
					if err := observer.Close(context.Background()); err != nil {
						t.Fatal(err)
					}
					break
				}
				event.Persisted <- nil
				event.Processed <- nil
			}
			terminal := worker["terminal"].(map[string]string)
			terminal["incarnationId"] = "replacement"
			if receipt, observer, err := port.ObserveLifecycle(context.Background(), snapshot); err == nil || observer != nil || receipt.TurnEvidence != nil {
				t.Fatal("replacement worker supplied a verified original result")
			}
			terminal["incarnationId"] = snapshot.Ref.External.Incarnation
			worker["worker"].(map[string]string)["agentTerminalHandle"] = "different-worker-terminal"
			if receipt, observer, err := port.ObserveLifecycle(context.Background(), snapshot); err == nil || observer != nil || receipt.TurnEvidence != nil {
				t.Fatal("cross-wired worker terminal supplied a verified original result")
			}
			worker["worker"].(map[string]string)["agentTerminalHandle"] = snapshot.Ref.External.Terminal
			worker["worker"].(map[string]string)["state"] = "running"
			if receipt, observer, err := port.ObserveLifecycle(context.Background(), snapshot); err != nil || observer != nil || receipt.TurnEvidence != nil || receipt.Diagnostic == "" {
				t.Fatal("unsettled native worker supplied a completed result")
			}
			worker["worker"].(map[string]string)["state"] = "succeeded"
			receipt, restored, err := port.ObserveLifecycle(context.Background(), snapshot)
			if err != nil || restored != nil || receipt.TurnEvidence == nil || !receipt.TurnEvidence.FinalVerified || receipt.TurnEvidence.LastMessage != "完整结果🙂\n\"quoted\" \\ $() `literal`" {
				t.Fatalf("restore: %+v %v", receipt, err)
			}
			snapshot.Implementation = receipt.Implementation
			// Once verified and saved, no process/history query is needed after release.
			port.client.config.Call = func(context.Context, time.Duration, ...string) ([]byte, error) {
				t.Fatal("frozen final contacted released worker")
				return nil, nil
			}
			cached, _, err := port.ObserveLifecycle(context.Background(), snapshot)
			if err != nil || !reflect.DeepEqual(cached.TurnEvidence, receipt.TurnEvidence) {
				t.Fatalf("frozen result changed: %v", err)
			}
		})
	}
}
