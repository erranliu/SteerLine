package orca_remote_worker

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	orcaworker "github.com/erranliu/SteerLine/internal/execution/adapters/orca"
	b "github.com/erranliu/SteerLine/internal/execution/backend"
)

const replyBody = "完整结果🙂\n\"quoted\" \\ $() \x60literal\x60"

func TestSupervisedResultPromptIsUnchanged(t *testing.T) {
	snapshot, state := remoteSnapshotForTest(t)
	state.Turns = nil
	input := b.TurnInput{CommandID: "00000000000000000000000000000004", InputVersion: 1, Prompt: "Current step: implement.report.\nFixed business facts"}
	snapshot.PendingTurn = &input
	port := &ExecutionLifecycle{setup: b.LifecycleSetup{Snapshot: snapshot}, config: lifecycleRemoteTestConfig(nil)}
	var err error
	snapshot.Implementation, err = port.stateEnvelopes(snapshot, state)
	if err != nil {
		t.Fatal(err)
	}
	next, err := port.Next(context.Background(), snapshot)
	if err != nil || next.Input.Resource == nil || next.Input.Resource.Objective != input.Prompt {
		t.Fatalf("adapter decorated the original prompt: %+v %v", next, err)
	}
}

func resultFixture(t *testing.T) (b.LifecycleSnapshot, orcaworker.NativeTurnState, Message) {
	t.Helper()
	snapshot, _ := remoteSnapshotForTest(t)
	snapshot.Session.CurrentTurnID, snapshot.Session.CurrentCommandID = "00000000000000000000000000000004", "00000000000000000000000000000004"
	turn := orcaworker.NativeTurnState{TurnID: snapshot.Session.CurrentTurnID, InputVersion: 3, TaskID: "task", DispatchID: "dispatch", ResultProcess: &snapshot.Ref.External}
	raw, _ := json.Marshal(map[string]any{"taskId": "task", "dispatchId": "dispatch", "outcome": "succeeded"})
	return snapshot, turn, Message{ID: "done", RunID: "run", ToHandle: "run:run", Type: "worker_done", FromHandle: "agent-terminal", Body: "Summary only.", Payload: raw}
}
func transcriptText(id, role, text string) map[string]any {
	return map[string]any{"id": id, "role": role, "blocks": []map[string]string{{"type": "text", "text": text}}}
}
func replyOutput(body string) map[string]any {
	return map[string]any{"dispatchId": "dispatch", "source": "transcript", "sourceIdentity": "original-transcript", "sourceExact": true, "contentComplete": false, "clipping": []string{"transcript_payload"}, "remoteRuntimeEpoch": "remote-epoch", "transcript": map[string]any{"messages": []any{
		transcriptText("old", "assistant", "old result must not be used"),
		transcriptText("anchor", "user", "You are working inside Orca, a multi-agent IDE. You are a dispatched worker.\nYour task ID is: task\n"),
		map[string]any{"id": "tool", "role": "tool", "blocks": []map[string]string{{"type": "tool-result", "output": "fake business JSON"}}},
		transcriptText("reply", "assistant", body),
	}}}
}
func replyClient(output map[string]any) Client {
	return New(lifecycleRemoteTestConfig(func(_ context.Context, _ time.Duration, args ...string) ([]byte, error) {
		if args[1] != "worker-read" || flag(args, "--dispatch") != "dispatch" || flag(args, "--source") != "transcript" {
			return nil, errors.New("unexpected result read")
		}
		return remoteEnvelope("center-runtime", output), nil
	}))
}
func TestSupervisedResultContract(t *testing.T) {
	snapshot, turn, message := resultFixture(t)
	captureResult(&turn, message, snapshot.Ref.External, "succeeded")
	if turn.FinalVerified || turn.FinalText != "" || turn.FinalReadError != "" {
		t.Fatal("completion summary became business result")
	}
	replyClient(replyOutput(replyBody)).captureReply(context.Background(), &turn)
	turn.Settled = true
	evidence, err := settledResult(snapshot, &turn)
	if err != nil || !evidence.FinalVerified || evidence.LastMessage != replyBody || evidence.FinalItemID != "reply" {
		t.Fatalf("bound reply lost: %+v %v", evidence, err)
	}
	captureResult(&turn, message, snapshot.Ref.External, "succeeded")
	if !turn.FinalVerified {
		t.Fatal("replay changed frozen reply")
	}
	message.Body = "conflicting"
	captureResult(&turn, message, snapshot.Ref.External, "succeeded")
	evidence, err = settledResult(snapshot, &turn)
	if err == nil || evidence.FinalVerified || !strings.Contains(err.Error(), "conflicting") {
		t.Fatal("completion conflict ignored")
	}
}

func TestSupervisedResultCompletionRejectsUnbound(t *testing.T) {
	for _, bad := range []string{"sender", "recipient", "task", "duplicate_payload", "process"} {
		t.Run(bad, func(t *testing.T) {
			snapshot, turn, message := resultFixture(t)
			switch bad {
			case "sender":
				message.FromHandle = "replacement-worker"
			case "recipient":
				message.ToHandle = "run:another"
			case "task":
				message.Payload = json.RawMessage(`{"taskId":"other","dispatchId":"dispatch","outcome":"succeeded"}`)
			case "duplicate_payload":
				message.Payload = json.RawMessage(`{"taskId":"other","taskId":"task","dispatchId":"dispatch","outcome":"succeeded"}`)
			case "process":
				changed := *turn.ResultProcess
				changed.Incarnation = "replacement"
				turn.ResultProcess = &changed
			}
			captureResult(&turn, message, snapshot.Ref.External, "succeeded")
			turn.Settled = true
			if _, err := settledResult(snapshot, &turn); err == nil {
				t.Fatal("invalid completion supplied a stopped execution")
			}
		})
	}
}
func TestSupervisedResultRejectsUnboundOrIncomplete(t *testing.T) {
	for _, bad := range []string{"dispatch", "source", "exact", "runtime", "anchor", "next_task", "new_user_input", "clipped", "pending_tool", "source_changed"} {
		t.Run(bad, func(t *testing.T) {
			snapshot, turn, message := resultFixture(t)
			captureResult(&turn, message, snapshot.Ref.External, "succeeded")
			output := replyOutput(replyBody)
			transcript := output["transcript"].(map[string]any)
			messages := transcript["messages"].([]any)
			switch bad {
			case "dispatch":
				output["dispatchId"] = "other"
			case "source":
				output["source"] = "terminal"
			case "exact":
				output["sourceExact"] = false
			case "runtime":
				output["remoteRuntimeEpoch"] = "replacement"
			case "anchor":
				messages[1] = transcriptText("anchor", "user", "no native input")
			case "next_task":
				messages = append(messages, transcriptText("next", "user", "You are working inside Orca, a multi-agent IDE. You are a dispatched worker.\nYour task ID is: another\n"))
			case "new_user_input":
				messages = append(messages, transcriptText("next-user", "user", "unrelated manual work"), transcriptText("next-reply", "assistant", "a different result"))
			case "clipped":
				messages[3] = transcriptText("reply", "assistant", "reply\n… (truncated)")
			case "pending_tool":
				messages = messages[:3]
			case "source_changed":
				turn.OutputSourceIdentity = "old-source"
			}
			transcript["messages"] = messages
			replyClient(output).captureReply(context.Background(), &turn)
			turn.Settled = true
			evidence, err := settledResult(snapshot, &turn)
			if err != nil || evidence.FinalVerified || evidence.FinalDiagnostic == "" {
				t.Fatalf("invalid reply verified: %+v %v", evidence, err)
			}
		})
	}
}
func TestSupervisedResultPaginationAndDelayedReply(t *testing.T) {
	_, turn, _ := resultFixture(t)
	calls := 0
	client := New(lifecycleRemoteTestConfig(func(_ context.Context, _ time.Duration, args ...string) ([]byte, error) {
		calls++
		output := replyOutput(replyBody)
		transcript := output["transcript"].(map[string]any)
		messages := transcript["messages"].([]any)
		if flag(args, "--cursor") == "" {
			transcript["messages"], transcript["nextCursor"] = messages[:3], "page-2"
		} else {
			transcript["messages"] = messages[3:]
		}
		return remoteEnvelope("center-runtime", output), nil
	}))
	client.captureReply(context.Background(), &turn)
	if calls != 2 || turn.FinalText != replyBody {
		t.Fatalf("pagination lost reply: %+v", turn)
	}
	_, turn, _ = resultFixture(t)
	output := replyOutput(replyBody)
	transcript := output["transcript"].(map[string]any)
	messages := transcript["messages"].([]any)
	transcript["messages"] = messages[:3]
	replyClient(output).captureReply(context.Background(), &turn)
	if turn.FinalReadError == "" || turn.InputAnchorID != "anchor" {
		t.Fatal("missing reply boundary lost")
	}
	transcript["messages"] = messages
	replyClient(output).captureReply(context.Background(), &turn)
	if turn.FinalReadError != "" || turn.FinalText != replyBody {
		t.Fatal("delayed reply not collected")
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
				case "worker-read":
					output := replyOutput(replyBody)
					output["remoteRuntimeEpoch"] = snapshot.Ref.External.RuntimeID
					return remoteEnvelope(runtime, output), nil
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
