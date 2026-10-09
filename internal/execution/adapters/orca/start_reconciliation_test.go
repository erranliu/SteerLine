package orcaworker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"testing"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	resourceprocess "github.com/erranliu/SteerLine/internal/execution/process"
)

func TestOrcaCLIExitHelper(t *testing.T) {
	if !slices.Contains(os.Args, "-test.run=^TestOrcaCLIExitHelper$") {
		t.Skip("subprocess helper")
	}
	os.Exit(1)
}

func completedCLIExit() error {
	return exec.Command(os.Args[0], "-test.run=^TestOrcaCLIExitHelper$").Run()
}

func TestOrcaAcceptedUnobservedStartPreservesWorker(t *testing.T) {
	for _, nonzero := range []bool{false, true} {
		t.Run(fmt.Sprint(nonzero), func(t *testing.T) {
			snapshot, state := nativeTestSnapshot(t, "agent")
			port := ExecutionLifecycle{orca: ProfileConfig{WaitTimeout: "1m"}}
			port.client = New(Config{Output: func(_ context.Context, _ time.Duration, args ...string) ([]byte, error) {
				switch args[1] {
				case "worker-start":
					payload := unobservedStart("saved-start")
					if nonzero {
						// A completed CLI rejection can carry the durable accepted receipt.
						return nativeTestEnvelope(t, payload), completedCLIExit()
					}
					return nativeTestEnvelope(t, payload), nil
				case "worker-show":
					return nativeTestEnvelope(t, map[string]any{
						"dispatch": map[string]string{"id": "dispatch", "runId": "run", "taskId": "task"},
						"worker":   map[string]string{"dispatchId": "dispatch", "agentTerminalHandle": "terminal"},
						"terminal": Terminal{Handle: "terminal", IncarnationID: "inc", WorktreeID: "workspace", WorktreePath: snapshot.Workspace.Path, Branch: "branch", AgentIdentity: "agent", Connected: true},
					}), nil
				default:
					t.Fatalf("unobserved input mutated resources: %v", args)
					return nil, nil
				}
			}})
			_, started, process, err := port.startNativeWorker(context.Background(), snapshot, state, NativeTurnState{TaskID: "task"}, "saved-start")
			if err != nil || process.Terminal != "terminal" || started.TurnStart != "unobserved" {
				t.Fatalf("accepted original input was discarded or promoted to started: %+v %+v %v", started, process, err)
			}
		})
	}
}

func unobservedStart(operation string) map[string]any {
	return map[string]any{"runId": "run", "taskId": "task", "dispatchId": "dispatch", "state": "outcome_unknown", "stage": "turn_start_unobserved", "turnStart": "unobserved",
		"prompt": map[string]any{"requestId": nativeRequestID(operation), "processIncarnation": "inc", "stages": []string{"input_accepted"}}, "mutation": map[string]string{"requestId": nativeRequestID(operation)}, "effects": []any{map[string]string{"kind": "dispatch_input", "role": "agent", "id": "terminal", "state": "accepted"}}, "mode": map[string]string{"mode": "terminal"}, "launch": map[string]any{"effective": map[string]string{"agent": "agent"}}}
}

func TestOrcaUnobservedStartRequiresDurableAcceptance(t *testing.T) {
	for _, scenario := range []string{"wrong_request", "missing_acceptance", "transport_uncertain"} {
		t.Run(scenario, func(t *testing.T) {
			snapshot, state := nativeTestSnapshot(t, "agent")
			port := ExecutionLifecycle{orca: ProfileConfig{WaitTimeout: "1m"}}
			port.client = New(Config{Output: func(_ context.Context, _ time.Duration, args ...string) ([]byte, error) {
				if args[1] != "worker-start" {
					t.Fatalf("unproven receipt bound a worker: %v", args)
				}
				payload := unobservedStart("saved-start")
				switch scenario {
				case "wrong_request":
					payload["prompt"] = map[string]any{"requestId": "foreign", "stages": []string{"input_accepted"}}
				case "missing_acceptance":
					payload["prompt"] = map[string]any{"requestId": nativeRequestID("saved-start"), "stages": []string{}}
				case "transport_uncertain":
					return nativeTestEnvelope(t, payload), context.DeadlineExceeded
				}
				return nativeTestEnvelope(t, payload), nil
			}})
			_, _, process, err := port.startNativeWorker(context.Background(), snapshot, state, NativeTurnState{TaskID: "task"}, "saved-start")
			if err == nil || process.Terminal != "" {
				t.Fatal("unproven acceptance became a bound original worker")
			}
		})
	}
}

func TestOrcaUnverifiedPartialStartDoesNotStopWorker(t *testing.T) {
	snapshot, state := nativeTestSnapshot(t, "agent")
	turn := stableIDValue(6)
	state.Turns = []NativeTurnState{{TurnID: turn, TaskID: "task"}}
	port := ExecutionLifecycle{client: New(Config{Output: func(_ context.Context, _ time.Duration, args ...string) ([]byte, error) {
		if args[1] != "worker-show" {
			t.Fatalf("unverified start closed live resources: %v", args)
		}
		return nativeTestEnvelope(t, auditWorkerResource("dispatch", "task", "runtime", "workspace", "dispatch", "active", "retained")), nil
	}})}
	request := executionbackend.LifecycleRequest{Snapshot: snapshot, Operation: executionbackend.Operation{ID: "saved-start"}, Input: executionbackend.LifecycleInput{Turn: &executionbackend.TurnInput{CommandID: turn}}}
	receipt, err := port.recoverFailedStart(context.Background(), request, state, workerStartReceipt{RunID: "run", TaskID: "task", DispatchID: "dispatch", State: "outcome_unknown"}, Result{RuntimeID: "runtime", Payload: json.RawMessage(`{"input":"accepted"}`)}, errors.New("turn unverified"))
	if err == nil || receipt.Disposition != executionbackend.OperationUnknown || receipt.State.WorkerAbsent || len(receipt.Implementation) == 0 {
		t.Fatalf("unverified input lost its partial resource: %+v %v", receipt, err)
	}
}

func TestOrcaReleasedStartReconciliationDoesNotReplayMutation(t *testing.T) {
	for _, scenario := range []string{"released", "transferred", "unreachable"} {
		t.Run(scenario, func(t *testing.T) {
			snapshot, state := nativeTestSnapshot(t, "agent")
			turn := stableIDValue(6)
			snapshot.Ref.Target.Dispatch, snapshot.Ref.Target.ExternalTask = "dispatch", "task"
			state.FailedStartReleased = true
			state.Turns = []NativeTurnState{{TurnID: turn, TaskID: "task"}}
			port := ExecutionLifecycle{}
			var err error
			snapshot.Implementation, err = port.targetEnvelope(snapshot, snapshot.Ref.Target, snapshot.Ref.External, state)
			if err != nil {
				t.Fatal(err)
			}
			port.client = New(Config{Output: func(_ context.Context, _ time.Duration, args ...string) ([]byte, error) {
				if args[1] != "worker-show" || flagValue(args, "--dispatch") != "dispatch" {
					t.Fatalf("failed original start was replayed: %v", args)
				}
				if scenario == "unreachable" {
					return nil, errors.New("contact lost")
				}
				owner := "dispatch"
				if scenario == "transferred" {
					owner = "other"
				}
				return nativeTestEnvelope(t, auditWorkerResource("dispatch", "task", "runtime", "workspace", owner, "stopped", "released")), nil
			}})
			request := executionbackend.LifecycleRequest{Snapshot: snapshot, Operation: executionbackend.Operation{ID: "saved-start"}, Input: executionbackend.LifecycleInput{Turn: &executionbackend.TurnInput{CommandID: turn}}}
			original := json.RawMessage(`{"input":"already accepted"}`)
			saved := executionbackend.LifecycleReceipt{Operation: request.Operation}
			saved.Operation.Receipt = original
			request.Operation.Receipt, err = json.Marshal(saved)
			if err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2; i++ {
				receipt, observer, err := port.reconcileStart(context.Background(), request)
				if observer != nil || (scenario == "released") != (err == nil && receipt.Disposition == executionbackend.OperationRejected && receipt.State.WorkerAbsent && receipt.State.WritersAbsent && receipt.State.Stopped) {
					t.Fatalf("release evidence did not settle the original failed start: %+v %v", receipt, err)
				}
				if scenario != "released" {
					if !bytes.Equal(receipt.Operation.Receipt, original) {
						t.Fatal("unavailable observations lost or recursively nested original effects")
					}
					request.Operation.Receipt, err = json.Marshal(receipt)
					if err != nil {
						t.Fatal(err)
					}
				} else if !bytes.Contains(receipt.Operation.Receipt, original) {
					t.Fatal("confirmed release erased original input effects")
				}
			}
		})
	}
}

func TestOrcaLifecycleCommandHelper(t *testing.T) {
	if !slices.Contains(os.Args, "-test.run=^TestOrcaLifecycleCommandHelper$") {
		t.Skip("subprocess helper")
	}
	time.Sleep(200 * time.Millisecond)
	fmt.Print(`{"ok":true,"result":{"budget":"completed"},"_meta":{"runtimeId":"runtime"}}`)
	os.Exit(0)
}

func TestOrcaLifecycleOperationBudgetReachesProcess(t *testing.T) {
	snapshot, _ := nativeTestSnapshot(t, "agent")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	profile := ProfileConfig{Executable: executable, Shell: "powershell.exe", Args: []string{"-test.run=^TestOrcaLifecycleCommandHelper$", "--"}, WaitTimeout: "4m"}
	snapshot.Ref.Target.ID = NativeTargetID(profile)
	raw, err := EncodeProfileConfig(profile)
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := executionbackend.NewImplementationDataEnvelope(executionbackend.ImplementationDataEnvelope{CodecVersion: ProfileCodecVersion, Purpose: executionbackend.ImplementationConfigPurpose, ImplementationKey: ProfileCodecKey, ProjectID: snapshot.Session.ProjectID, ProfileID: "profile", AcceptedConfigVersion: "accepted"}, raw)
	if err != nil {
		t.Fatal(err)
	}
	port, err := NewExecutionLifecycle(executionbackend.LifecycleSetup{Snapshot: snapshot, ProfileID: "profile", AcceptedConfigVersion: "accepted", ProfileData: []executionbackend.ImplementationDataEnvelope{envelope}, ProcessHost: resourceprocess.New(50 * time.Millisecond), CommandTimeout: 50 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	// Scale the 30s/15m production mismatch down without a minute-long test.
	result, err := port.(*ExecutionLifecycle).client.metaCallTimeout(context.Background(), 5*time.Second, "status", "--json")
	if err != nil || result.RuntimeID != "runtime" {
		t.Fatalf("startup budget was capped by ordinary host timeout: %+v %v", result, err)
	}
}
