package orcaworker

import (
	"context"
	"encoding/json"
	"errors"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	conversationtypes "github.com/erranliu/SteerLine/internal/execution/conversation"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func auditWorkerResource(dispatch, task, runtime, workspace, owner, state, release string) map[string]any {
	return map[string]any{"dispatch": map[string]string{"id": dispatch, "taskId": task, "runId": "run"}, "worker": map[string]string{"dispatchId": dispatch, "state": state}, "terminalResource": map[string]string{"terminalHandle": "terminal", "worktreeId": workspace, "ownerDispatchId": owner, "originDispatchId": dispatch, "releaseState": release}}
}
func TestPartialWorkerRecoveryChecksOwnershipAndPersistentStop(t *testing.T) {
	for _, scenario := range []string{"stopped", "late-stop", "wrong-runtime", "wrong-workspace", "transferred", "unreachable"} {
		t.Run(scenario, func(t *testing.T) {
			snapshot, state := nativeTestSnapshot(t, "agent")
			turn := stableIDValue(6)
			state.Turns = []NativeTurnState{{TurnID: turn, TaskID: "task"}}
			calls := []string{}
			reads := 0
			port := ExecutionLifecycle{client: New(Config{Output: func(_ context.Context, _ time.Duration, args ...string) ([]byte, error) {
				calls = append(calls, args[1])
				if flagValue(args, "--dispatch") != "dispatch" {
					t.Fatal("foreign cleanup")
				}
				if args[1] == "worker-show" {
					reads++
					runtime, workspace, owner, workerState := "runtime", "workspace", "dispatch", "active"
					switch scenario {
					case "stopped":
						workerState = "stopped"
					case "late-stop":
						if reads > 1 {
							workerState = "stopped"
						}
					case "wrong-runtime":
						runtime = "new-runtime"
					case "wrong-workspace":
						workspace = "foreign"
					case "transferred":
						owner = "new-owner"
					case "unreachable":
						return nil, errors.New("contact lost")
					}
					raw, _ := json.Marshal(map[string]any{"ok": true, "result": auditWorkerResource("dispatch", "task", runtime, workspace, owner, workerState, "retained"), "_meta": map[string]string{"runtimeId": runtime}})
					return raw, nil
				}
				if args[1] == "worker-stop" {
					return nativeTestEnvelope(t, map[string]string{"dispatchId": "dispatch", "state": "stop_unknown"}), nil
				}
				if args[1] == "worker-release" {
					return nativeTestEnvelope(t, map[string]string{"dispatchId": "dispatch", "state": "released"}), nil
				}
				t.Fatalf("unexpected mutation %v", args)
				return nil, nil
			}})}
			request := executionbackend.LifecycleRequest{Snapshot: snapshot, Operation: executionbackend.Operation{ID: "original-start"}, Input: executionbackend.LifecycleInput{Turn: &executionbackend.TurnInput{CommandID: turn}}}
			receipt, err := port.recoverFailedStart(context.Background(), request, state, workerStartReceipt{RunID: "run", TaskID: "task", DispatchID: "dispatch"}, Result{RuntimeID: "runtime", Payload: json.RawMessage(`{"input":"already accepted"}`)}, errors.New("start unknown"))
			if err == nil {
				t.Fatal("partial first-turn error erased")
			}
			switch scenario {
			case "stopped":
				if strings.Join(calls, ",") != "worker-show,worker-release" || !receipt.State.WritersAbsent {
					t.Fatalf("persistent stopped worker leaked: %v", calls)
				}
			case "late-stop":
				if strings.Join(calls, ",") != "worker-show,worker-stop,worker-show,worker-release" || !receipt.State.WorkerAbsent {
					t.Fatalf("late stop not released: %v", calls)
				}
			default:
				if len(calls) != 1 || receipt.State.WorkerAbsent || receipt.State.WritersAbsent {
					t.Fatalf("unverified original owner mutated: %v", calls)
				}
			}
		})
	}
}

func TestOutputEvidenceCoversOutcomeAndJSONExpansion(t *testing.T) {
	binding := outputTestBinding(t)
	path := filepath.Join(t.TempDir(), "source.json")
	material := OutputMaterial{WorkflowRunID: binding.WorkflowID, TurnID: binding.TurnID, TaskID: binding.Process.Task, DispatchID: binding.Process.Dispatch, Body: "complete", Outcome: "completed"}
	if err := freezeSettledOutput(path, binding, material); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path + ".settled")
	if err != nil {
		t.Fatal(err)
	}
	raw = []byte(strings.Replace(string(raw), `"outcome":"completed"`, `"outcome":"failed"`, 1))
	if err = os.WriteFile(path+".settled", raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = readSettledOutput(path, binding); err == nil {
		t.Fatal("outcome-only tampering accepted")
	}
	body := strings.Repeat("<&\x00", (executionmodel.MaxConversationBodyBytes / 6))
	material.Body = body
	largePath := filepath.Join(t.TempDir(), "large-source.json")
	if err = freezeSettledOutput(largePath, binding, material); err != nil {
		t.Fatal(err)
	}
	decoded, err := readSettledOutput(largePath, binding)
	if err != nil || decoded == nil || decoded.Body != body {
		t.Fatalf("valid escaped body unreadable: %v", err)
	}
	writeTestMaterial(t, binding, material)
	decoded, err = readOutputMaterial(context.Background(), func(context.Context, ...string) (Result, error) { return settledTestResult(binding, "completed"), nil }, binding)
	if err != nil || decoded == nil || decoded.Body != body {
		t.Fatalf("valid escaped input rejected: %v", err)
	}
	if settledOutputDigest(material, "completed") == settledOutputDigest(material, "failed") {
		t.Fatal("history cursor ignores outcome")
	}
}

func TestOutputDiscoveryIsolatesDamagedSource(t *testing.T) {
	binding := outputTestBinding(t)
	runDir := t.TempDir()
	dir := filepath.Join(runDir, "turns", binding.ExecutionID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(binding)
	if err := os.WriteFile(filepath.Join(dir, "orca-output."+binding.TurnID+".json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "orca-output."+stableIDValue(9)+".json"), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	unknown := binding
	unknown.AttemptID = stableIDValue(10)
	unknown.TurnID = stableIDValue(11)
	unknownRaw, _ := json.Marshal(unknown)
	if err := os.WriteFile(filepath.Join(dir, "orca-output."+unknown.TurnID+".json"), unknownRaw, 0600); err != nil {
		t.Fatal(err)
	}
	base := executionmodel.ConversationEvent{Version: executionmodel.ConversationVersion, ProjectID: binding.ProjectID, TaskID: "task", WorkflowID: binding.WorkflowID, ExecutionID: binding.ExecutionID, AttemptID: binding.AttemptID, Backend: executionbackend.BackendOrca, Protocol: executionbackend.ProtocolOrcaTerminalV2, Agent: "agent", TargetID: "target"}
	discovery := conversationtypes.ConversationDiscovery{ProjectID: binding.ProjectID, Runs: []conversationtypes.ConversationRunSource{{Base: base, RunDirectory: runDir, Workspace: binding.Workspace}}}
	got, err := NewConversationAdapter().DiscoverHistory(context.Background(), discovery)
	if err != nil || len(got.Sources) != 1 || len(got.Writes) != 2 || got.Writes[0].Event.Type != executionmodel.ConversationGap {
		t.Fatalf("damaged source stopped healthy discovery: %+v %v", got, err)
	}
	if err = got.Writes[0].Event.Validate(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = NewConversationAdapter().DiscoverHistory(ctx, discovery); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled discovery continued")
	}
}

func TestRecoveredDoneRequiresCompleteFrozenOutput(t *testing.T) {
	snapshot, state := nativeTestSnapshot(t, "agent")
	turn := NativeTurnState{TurnID: stableIDValue(6), TaskID: "task", DispatchID: "dispatch", DoneMessageID: "done", Outcome: "succeeded", Purpose: "business"}
	state.Turns = []NativeTurnState{turn}
	process := executionbackend.ProcessIdentity{RuntimeID: "runtime", Terminal: "terminal", Incarnation: "inc", WorktreeID: "workspace", Task: "task", Dispatch: "dispatch", OrchestrationRun: "run"}
	port := ExecutionLifecycle{orca: ProfileConfig{Executable: "orca", Shell: "cmd.exe", WaitTimeout: "1m", Model: "opaque-model", ReasoningEffort: "medium"}, setup: executionbackend.LifecycleSetup{RunDirectory: t.TempDir()}}
	snapshot = port.boundSnapshot(snapshot, snapshot.Ref.Target, process, state)
	port.setup.Snapshot = snapshot
	binding := outputBinding{Process: process}
	port.client = New(Config{Output: func(context.Context, time.Duration, ...string) ([]byte, error) {
		r := settledTestResult(binding, "completed")
		return nativeTestEnvelope(t, r.Payload), nil
	}})
	receipt, observer, err := port.ObserveLifecycle(context.Background(), snapshot)
	if err == nil || receipt.State.TurnCompleted || observer != nil || receipt.Diagnostic == "" {
		t.Fatal("recovered done bypassed output freeze")
	}
}

func TestNativeStartTransportBudgetCoversReadiness(t *testing.T) {
	snapshot, state := nativeTestSnapshot(t, "agent")
	port := ExecutionLifecycle{orca: ProfileConfig{WaitTimeout: "4m"}, client: New(Config{CommandTimeout: "10s", Output: func(_ context.Context, budget time.Duration, args ...string) ([]byte, error) {
		if budget < 4*time.Minute+30*time.Second {
			t.Fatalf("CLI killed before readiness deadline: %v", budget)
		}
		if flagValue(args, "--retry-request") != nativeRequestID("saved-start") {
			t.Fatal("start request changed")
		}
		return nil, errors.New("controlled transport loss")
	}})}
	_, _, _, err := port.startNativeWorker(context.Background(), snapshot, state, NativeTurnState{TaskID: "task"}, "saved-start")
	if err == nil {
		t.Fatal("transport loss became success")
	}
}

func TestSettledWorkerStopReleasesOriginalResource(t *testing.T) {
	for _, release := range []string{"released", "release_pending", "retained", "exited-terminal"} {
		t.Run(release, func(t *testing.T) {
			snapshot, _ := nativeTestSnapshot(t, "agent")
			snapshot.Ref.RunID, snapshot.Ref.AttemptID = snapshot.Session.WorkflowRunID, snapshot.Session.AttemptID
			p := executionbackend.ProcessIdentity{Kind: "orca", RuntimeID: "runtime", Terminal: "terminal", Incarnation: "inc", WorktreeID: "workspace", Task: "task", Dispatch: "dispatch", OrchestrationRun: "run"}
			snapshot.Ref.External = p
			ref := snapshot.Ref
			calls := []string{}
			port := ExecutionLifecycle{client: New(Config{Output: func(_ context.Context, _ time.Duration, args ...string) ([]byte, error) {
				calls = append(calls, args[1])
				switch args[1] {
				case "worker-show":
					return nativeTestEnvelope(t, auditWorkerResource("dispatch", "task", "runtime", "workspace", "dispatch", "failed", "retained")), nil
				case "show":
					return nativeTestEnvelope(t, map[string]any{"terminal": Terminal{Handle: "terminal", IncarnationID: "inc", WorktreeID: "workspace", WorktreePath: snapshot.Workspace.Path, Branch: "branch", AgentIdentity: "agent", Connected: true}}), nil
				case "worker-stop":
					return nativeTestEnvelope(t, map[string]any{"dispatchId": "dispatch", "state": "succeeded", "alreadySettled": true, "processAction": "none"}), nil
				case "worker-release":
					if flagValue(args, "--retry-request") != nativeRequestID("saved-stop:settled-release") {
						t.Fatal("release mutation changed")
					}
					state := release
					if release == "exited-terminal" {
						state = "released"
					}
					return nativeTestEnvelope(t, map[string]string{"dispatchId": "dispatch", "state": state}), nil
				}
				t.Fatalf("unexpected call %v", args)
				return nil, nil
			}})}
			op := executionbackend.Operation{ID: "saved-stop", PayloadDigest: "digest", Action: string(executionbackend.LifecycleStopExecution), Ref: &ref, LifecycleIdentity: &executionbackend.LifecycleOperationIdentity{ProjectID: snapshot.Session.ProjectID, ExecutionID: snapshot.Session.ExecutionID, ConfigurationDigest: snapshot.ConfigurationDigest, ResourceGeneration: snapshot.ResourceGeneration, InputBindingDigest: executionbackend.LifecycleBindingDigest(snapshot), Ref: ref}}
			request := executionbackend.LifecycleRequest{Snapshot: snapshot, Operation: op, Input: executionbackend.LifecycleInput{Stop: &executionbackend.StopInput{TurnID: stableIDValue(7)}}}
			receipt, err := port.StopLifecycle(context.Background(), request, nil)
			if err != nil || strings.Join(calls, ",") != "worker-show,worker-stop,worker-release" {
				t.Fatalf("settled stop not released: %v %v", calls, err)
			}
			if (receipt.Disposition == executionbackend.OperationConfirmed) != (release == "released" || release == "exited-terminal") || receipt.State.WorkerAbsent != (release == "released" || release == "exited-terminal") {
				t.Fatal("settled or pending release became absence")
			}
			if !strings.Contains(string(receipt.Operation.Receipt), "alreadySettled") {
				t.Fatal("stop/release receipt lost")
			}
		})
	}
}

func TestNativeHostWaitIsObservedWithoutBusinessQuestion(t *testing.T) {
	snapshot, _ := nativeTestSnapshot(t, "agent")
	snapshot.Ref.External = executionbackend.ProcessIdentity{Kind: "orca", Terminal: "terminal", RuntimeID: "runtime", Incarnation: "inc"}
	wait := json.RawMessage(`{"kind":"approval","source":"structured"}`)
	missing := false
	ids := []string{}
	port := &ExecutionLifecycle{client: New(Config{Output: func(context.Context, time.Duration, ...string) ([]byte, error) {
		terminal := map[string]any{"handle": "terminal", "incarnationId": "inc", "connected": true}
		if !missing {
			terminal["agentWait"] = wait
		}
		return nativeTestEnvelope(t, map[string]any{"terminal": terminal}), nil
	}})}
	observer := &nativeLifecycleObserver{lifecycle: port, snapshot: snapshot, state: NativeExecutionState{Turns: []NativeTurnState{{TurnID: stableIDValue(7)}}}, turn: NativeTurnState{TurnID: stableIDValue(7)}, events: make(chan executionbackend.LifecycleEvent, 1), done: make(chan error, 1)}
	inspect := func(wantWaiting bool) {
		finished := make(chan error, 1)
		go func() { finished <- observer.observeNativeHost(context.Background()) }()
		select {
		case ev := <-observer.events:
			ids = append(ids, ev.Fact.SourceEventID)
			if ev.InputWait != wantWaiting || ev.Fact.QuestionRoute != nil || len(ev.Fact.Questions) > 0 {
				t.Fatal("host wait fabricated business question")
			}
			if wantWaiting && (!strings.Contains(ev.Diagnostic, "confirmed") || ev.Fact.SourceIdentity == "" || ev.Fact.SourceEventID == "") {
				t.Fatal("host evidence missing")
			}
			ev.Persisted <- nil
			ev.Processed <- nil
		case <-time.After(time.Second):
			t.Fatal("native wait not emitted")
		}
		if err := <-finished; err != nil {
			t.Fatal(err)
		}
	}
	inspect(true)
	if err := observer.observeNativeHost(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(observer.events) != 0 {
		t.Fatal("unchanged wait repeated")
	}
	missing = true
	finished := make(chan error, 1)
	go func() { finished <- observer.observeNativeHost(context.Background()) }()
	ev := <-observer.events
	if ev.Status != "unknown" || !ev.Fact.Gap || observer.lastHostWait == nil {
		t.Fatal("missing field cleared known wait")
	}
	ev.Persisted <- nil
	ev.Processed <- nil
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	missing = false
	wait = json.RawMessage("null")
	inspect(false)
	wait = json.RawMessage(`{"kind":"approval","source":"structured"}`)
	inspect(true)
	if ids[0] == ids[2] {
		t.Fatal("new occurrence of same wait deduplicated as old")
	}
	persistedTurn := nativeTurn(&observer.state, observer.turn.TurnID)
	if persistedTurn.HostWaitSequence != 3 || persistedTurn.HostWait == nil {
		t.Fatal("wait identity not durable across observer restart")
	}
	observer.lastHostWait = persistedTurn.HostWait
	if err := observer.observeNativeHost(context.Background()); err != nil || len(observer.events) != 0 {
		t.Fatal("restored same wait repeated")
	}
}
