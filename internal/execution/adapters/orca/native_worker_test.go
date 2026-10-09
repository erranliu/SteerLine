package orcaworker

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func TestOrcaNativeAgentBoundary(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"codex", "claude", "exec_command", "SLREADINESS", "Pasted Content"} {
			if strings.Contains(strings.ToLower(string(data)), strings.ToLower(forbidden)) {
				t.Errorf("%s contains agent-specific implementation %q", file, forbidden)
			}
		}
	}
}

func TestOrcaNativeProfile(t *testing.T) {
	for _, agent := range []string{"codex", "claude", "future-agent"} {
		payload, err := EncodeProfileConfig(ProfileConfig{Executable: "orca", Shell: "cmd.exe", WaitTimeout: "1m", Model: "opaque-model", ReasoningEffort: "medium"})
		if err != nil {
			t.Fatal(err)
		}
		if err = ValidateProfileSelection(agent, payload); err != nil {
			t.Fatalf("%s: %v", agent, err)
		}
	}
	if err := ValidateProfileSelection("", []byte(`{}`)); err == nil {
		t.Fatal("empty agent accepted")
	}
	if err := ValidateProfileSelection("agent", []byte(`{"executable":"orca","shell":"cmd.exe","wait_timeout":"1m","reasoning_effort":"high"}`)); err == nil {
		t.Fatal("effort without model accepted")
	}
	if err := ValidateProfileSelection("agent", []byte(`{"executable":"orca","shell":"cmd.exe","wait_timeout":"1m","sandbox":"disabled"}`)); err == nil {
		t.Fatal("client-owned permission override accepted")
	}
}

func nativeTestSnapshot(t *testing.T, agent string) (executionbackend.LifecycleSnapshot, NativeExecutionState) {
	t.Helper()
	path := t.TempDir()
	profile := ProfileConfig{Executable: "orca", Shell: "cmd.exe", WaitTimeout: "1m", Model: "opaque-model", ReasoningEffort: "medium"}
	target := executionbackend.ExecutionTarget{ID: NativeTargetID(profile), WorktreeID: "workspace", OrchestrationRun: "run", CoordinatorTerminal: "coordinator"}
	snapshot := executionbackend.LifecycleSnapshot{Ref: executionbackend.ExecutionRef{Backend: executionbackend.BackendOrca, Protocol: executionbackend.ProtocolOrcaTerminalV2, Target: target, Agent: executionbackend.AgentSpec{Name: agent}}, Workspace: executionbackend.WorkspaceIntent{Path: path, Branch: "branch"}, Session: executionbackend.SessionContext{ProjectID: "project", WorkflowRunID: stableIDValue(1), ExecutionID: stableIDValue(2), AttemptID: stableIDValue(3), TaskID: "business-task"}, ConfigurationDigest: "profile", ResourceGeneration: "generation"}
	state := NativeExecutionState{Coordinator: executionbackend.ProcessIdentity{Terminal: "coordinator", RuntimeID: "runtime", Incarnation: "coordinator-inc"}}
	return snapshot, state
}

func TestOrcaWorkerStartOwnsLaunchAndOriginalContinuation(t *testing.T) {
	for _, agent := range []string{"codex", "claude"} {
		t.Run(agent, func(t *testing.T) {
			snapshot, state := nativeTestSnapshot(t, agent)
			calls := [][]string{}
			dispatch := "dispatch-one"
			task := "task-one"
			native := ProfileConfig{Executable: "orca", Shell: "cmd.exe", WaitTimeout: "1m", Model: "opaque-model", ReasoningEffort: "medium"}
			port := ExecutionLifecycle{orca: native}
			port.client = New(Config{Output: func(_ context.Context, _ time.Duration, args ...string) ([]byte, error) {
				calls = append(calls, append([]string(nil), args...))
				terminal := Terminal{Handle: "terminal", IncarnationID: "inc", WorktreeID: "workspace", WorktreePath: snapshot.Workspace.Path, Branch: "branch", AgentIdentity: agent, Connected: true, Status: "running"}
				var payload any
				switch args[1] {
				case "worker-start":
					payload = map[string]any{"runId": "run", "taskId": task, "dispatchId": dispatch, "state": "ready", "stage": "input_accepted", "turnStart": "observed", "mode": map[string]string{"mode": "terminal"}, "launch": map[string]any{"effective": map[string]string{"agent": agent, "model": "opaque-model", "effort": "medium"}}}
				case "show":
					payload = map[string]any{"terminal": terminal}
				case "worker-show":
					shownTask, shownDispatch := task, dispatch
					if flagValue(args, "--dispatch") == "dispatch-one" {
						shownTask, shownDispatch = "task-one", "dispatch-one"
					}
					payload = map[string]any{"dispatch": map[string]string{"id": shownDispatch, "taskId": shownTask, "runId": "run", "status": "completed"}, "worker": map[string]string{"dispatchId": shownDispatch, "agentTerminalHandle": "terminal", "stage": "input_accepted"}, "terminal": terminal}
				default:
					t.Fatalf("unexpected native command %v", args)
				}
				return nativeTestEnvelope(t, payload), nil
			}})
			_, _, process, err := port.startNativeWorker(context.Background(), snapshot, state, NativeTurnState{TaskID: task}, "operation-one")
			if err != nil {
				t.Fatal(err)
			}
			if flagValue(calls[0], "--agent") != agent || flagValue(calls[0], "--model") != "opaque-model" || flagValue(calls[0], "--retry-request") != nativeRequestID("operation-one") {
				t.Fatalf("native first launch %v", calls[0])
			}
			snapshot.Ref.External = process
			task, dispatch = "task-two", "dispatch-two"
			calls = nil
			_, _, continued, err := port.startNativeWorker(context.Background(), snapshot, state, NativeTurnState{TaskID: task}, "operation-two")
			if err != nil {
				t.Fatal(err)
			}
			var start []string
			for _, call := range calls {
				if call[1] == "worker-start" {
					start = call
				}
			}
			if flagValue(start, "--terminal") != process.Terminal || flagValue(start, "--agent") != "" || flagValue(start, "--model") != "" || flagValue(start, "--effort") != "" || continued.Incarnation != process.Incarnation {
				t.Fatalf("continuation changed original launch/session: %v %+v", start, continued)
			}
		})
	}
}

func TestOrcaUnsupportedWorkerPreservesNativeEffects(t *testing.T) {
	snapshot, state := nativeTestSnapshot(t, "agent")
	port := ExecutionLifecycle{orca: ProfileConfig{Executable: "orca"}}
	payload := []byte(`{"runId":"run","taskId":"task","dispatchId":"original-dispatch","state":"ready","stage":"input_accepted","turnStart":"observed","mode":{"mode":"structured"},"effects":[{"kind":"dispatch_input","state":"accepted"}]}`)
	port.client = New(Config{Output: func(context.Context, time.Duration, ...string) ([]byte, error) {
		return nativeTestEnvelope(t, json.RawMessage(payload)), nil
	}})
	result, started, _, err := port.startNativeWorker(context.Background(), snapshot, state, NativeTurnState{TaskID: "task"}, "original-operation")
	if err == nil || started.DispatchID != "original-dispatch" || string(result.Payload) != string(payload) {
		t.Fatalf("unsupported launch lost original effects: %+v %+v %v", result, started, err)
	}
}

func TestOrcaReleaseRequiresActualReleasedProof(t *testing.T) {
	for _, stateName := range []string{"released", "already_released", "retained", "release_pending", "release_unknown"} {
		t.Run(stateName, func(t *testing.T) {
			snapshot, state := nativeTestSnapshot(t, "agent")
			process := executionbackend.ProcessIdentity{Kind: "orca", Terminal: "terminal", RuntimeID: "runtime", Incarnation: "inc", WorktreeID: "workspace", OrchestrationRun: "run", Task: "task", Dispatch: "dispatch"}
			snapshot.Ref.External = process
			requests := []string{}
			port := ExecutionLifecycle{client: New(Config{Output: func(_ context.Context, _ time.Duration, args ...string) ([]byte, error) {
				if args[1] != "worker-release" {
					t.Fatalf("release bypassed native worker interface: %v", args)
				}
				requests = append(requests, flagValue(args, "--retry-request"))
				return nativeTestEnvelope(t, map[string]string{"dispatchId": "dispatch", "state": stateName}), nil
			}})}
			request := executionbackend.LifecycleRequest{Snapshot: snapshot, Operation: executionbackend.Operation{ID: "release-operation"}, Input: executionbackend.LifecycleInput{Cleanup: &executionbackend.CleanupInput{Resources: []executionbackend.ProcessIdentity{process}}}}
			receipt, err := port.releaseOriginalWorker(context.Background(), request, state)
			if err != nil {
				t.Fatal(err)
			}
			want := stateName == "released" || stateName == "already_released"
			if (receipt.Disposition == executionbackend.OperationConfirmed) != want {
				t.Fatalf("%s became %s", stateName, receipt.Disposition)
			}
			_, err = port.releaseOriginalWorker(context.Background(), request, state)
			if err != nil {
				t.Fatal(err)
			}
			if requests[0] != requests[1] || requests[0] != nativeRequestID(request.Operation.ID) {
				t.Fatal("unknown release changed its original mutation request")
			}
		})
	}
}

func flagValue(args []string, key string) string {
	for i, arg := range args {
		if arg == key && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func TestFailedCompositeStartStopsItsExactPartialWorker(t *testing.T) {
	snapshot, state := nativeTestSnapshot(t, "configured-agent")
	turn := stableIDValue(6)
	state.Turns = []NativeTurnState{{TurnID: turn, TaskID: "original-task"}}
	calls := []string{}
	port := ExecutionLifecycle{client: New(Config{Output: func(_ context.Context, _ time.Duration, args ...string) ([]byte, error) {
		if flagValue(args, "--dispatch") != "original-dispatch" {
			t.Fatal("partial cleanup targeted another Dispatch")
		}
		calls = append(calls, args[1])
		if args[1] == "worker-show" {
			return nativeTestEnvelope(t, auditWorkerResource("original-dispatch", "original-task", "runtime", "workspace", "original-dispatch", "active", "retained")), nil
		}
		status := "stopped"
		if args[1] == "worker-release" {
			status = "released"
		}
		return nativeTestEnvelope(t, map[string]string{"dispatchId": "original-dispatch", "state": status}), nil
	}})}
	request := executionbackend.LifecycleRequest{Snapshot: snapshot, Operation: executionbackend.Operation{ID: "failed-original-start"}, Input: executionbackend.LifecycleInput{Turn: &executionbackend.TurnInput{CommandID: turn}}}
	started := workerStartReceipt{RunID: "run", TaskID: "original-task", DispatchID: "original-dispatch"}
	receipt, err := port.recoverFailedStart(context.Background(), request, state, started, Result{RuntimeID: "runtime", Payload: json.RawMessage(`{"first_input":"already_sent"}`)}, errors.New("unsupported structured mode"))
	if err == nil || receipt.Disposition == executionbackend.OperationConfirmed || !receipt.State.WorkerAbsent || !receipt.State.WritersAbsent || strings.Join(calls, ",") != "worker-show,worker-stop,worker-release" {
		t.Fatalf("partial startup result: %+v %v %v", receipt, err, calls)
	}
	if !strings.Contains(string(receipt.Operation.Receipt), "already_sent") {
		t.Fatal("first-turn effect was erased during cleanup")
	}
}

func TestReleasedResourceReconciliationUsesPersistentOriginalOwnership(t *testing.T) {
	identity := executionbackend.ProcessIdentity{RuntimeID: "runtime", Terminal: "terminal", WorktreeID: "workspace", Dispatch: "dispatch", Task: "task", OrchestrationRun: "run"}
	owner := "dispatch"
	call := func(context.Context, ...string) (Result, error) {
		raw, _ := json.Marshal(map[string]any{"dispatch": map[string]string{"id": "dispatch", "taskId": "task", "runId": "run"}, "worker": map[string]string{"dispatchId": "dispatch"}, "terminal": nil, "terminalResource": map[string]string{"terminalHandle": "terminal", "worktreeId": "workspace", "ownerDispatchId": owner, "originDispatchId": "dispatch", "releaseState": "released"}})
		return Result{RuntimeID: "runtime", Payload: raw}, nil
	}
	if released, _, err := originalResourceReleased(context.Background(), call, identity); err != nil || !released {
		t.Fatalf("persistent release proof lost: %v %v", released, err)
	}
	owner = "next-dispatch"
	if released, _, err := originalResourceReleased(context.Background(), call, identity); err == nil || released {
		t.Fatal("transferred resource was released by its old owner")
	}
}
func nativeTestEnvelope(t *testing.T, payload any) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"ok": true, "_meta": map[string]string{"runtimeId": "runtime"}, "result": payload})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestUnresolvedNativeStartCannotProveAbsence(t *testing.T) {
	snapshot, state := nativeTestSnapshot(t, "agent")
	snapshot.Operations = []executionbackend.Operation{{ID: "original-start", Action: string(executionbackend.LifecycleStartTurn), Status: executionbackend.OperationUnknown}}
	port := ExecutionLifecycle{setup: executionbackend.LifecycleSetup{Snapshot: snapshot}, orca: ProfileConfig{Executable: "orca", Shell: "cmd.exe", WaitTimeout: "1m", Model: "opaque-model", ReasoningEffort: "medium"}}
	_ = state
	evidence, err := port.VerifyManualRecovery(context.Background(), snapshot)
	if err == nil || evidence.ProcessAbsent {
		t.Fatalf("unknown native start became absence: %+v %v", evidence, err)
	}
}

func TestStoppedResourceRequiresPersistentOriginalIdentity(t *testing.T) {
	identity := executionbackend.ProcessIdentity{RuntimeID: "runtime", Terminal: "terminal", WorktreeID: "workspace", Dispatch: "dispatch", Task: "task", OrchestrationRun: "run"}
	owner := "dispatch"
	state := "stopped"
	call := func(context.Context, ...string) (Result, error) {
		raw, _ := json.Marshal(map[string]any{"dispatch": map[string]string{"id": "dispatch", "taskId": "task", "runId": "run"}, "worker": map[string]string{"dispatchId": "dispatch", "state": state}, "terminal": nil, "terminalResource": map[string]string{"terminalHandle": "terminal", "worktreeId": "workspace", "ownerDispatchId": owner, "originDispatchId": "dispatch"}})
		return Result{RuntimeID: "runtime", Payload: raw}, nil
	}
	if stopped, _, err := originalWorkerStopped(context.Background(), call, identity); err != nil || !stopped {
		t.Fatalf("lost stop receipt not recovered: %v %v", stopped, err)
	}
	state = "failed"
	if stopped, _, err := originalWorkerStopped(context.Background(), call, identity); err != nil || stopped {
		t.Fatal("failed settlement became stopped")
	}
	state = "stopped"
	owner = "new-dispatch"
	if stopped, _, err := originalWorkerStopped(context.Background(), call, identity); err == nil || stopped {
		t.Fatal("transferred worker stopped by old owner")
	}
}

func TestSettledWorkerMissingOutputHasDiagnostic(t *testing.T) {
	snapshot, _ := nativeTestSnapshot(t, "agent")
	port := ExecutionLifecycle{setup: executionbackend.LifecycleSetup{RunDirectory: t.TempDir()}}
	if err := port.freezeOriginalOutput(context.Background(), snapshot, NativeTurnState{TurnID: stableIDValue(4)}); err == nil {
		t.Fatal("missing canonical output silently accepted")
	}
}

func TestStopReconciliationKeepsSavedDispatchAfterContinuation(t *testing.T) {
	snapshot, _ := nativeTestSnapshot(t, "agent")
	original := snapshot.Ref
	original.External = executionbackend.ProcessIdentity{RuntimeID: "runtime", Terminal: "terminal", WorktreeID: "workspace", Dispatch: "original-dispatch", Task: "original-task", OrchestrationRun: "run"}
	snapshot.Ref.External = original.External
	snapshot.Ref.External.Dispatch, snapshot.Ref.External.Task = "new-dispatch", "new-task"
	port := ExecutionLifecycle{client: New(Config{Output: func(_ context.Context, _ time.Duration, args ...string) ([]byte, error) {
		if flagValue(args, "--dispatch") != "original-dispatch" {
			t.Fatal("old stop used current Dispatch")
		}
		return nativeTestEnvelope(t, map[string]any{"dispatch": map[string]string{"id": "original-dispatch", "taskId": "original-task", "runId": "run"}, "worker": map[string]string{"dispatchId": "original-dispatch", "state": "stopped"}, "terminalResource": map[string]string{"terminalHandle": "terminal", "worktreeId": "workspace", "ownerDispatchId": "original-dispatch", "originDispatchId": "original-dispatch"}}), nil
	}})}
	receipt, _, err := port.reconcileStop(context.Background(), executionbackend.LifecycleRequest{Snapshot: snapshot, Operation: executionbackend.Operation{ID: "saved-stop", Ref: &original}})
	if err != nil || receipt.Disposition != executionbackend.OperationConfirmed || !receipt.State.Stopped {
		t.Fatalf("original stop not reconciled: %+v %v", receipt, err)
	}
}
