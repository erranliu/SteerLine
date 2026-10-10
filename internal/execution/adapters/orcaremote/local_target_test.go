package orca_remote_worker

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	orcaworker "github.com/erranliu/SteerLine/internal/execution/adapters/orca"
	b "github.com/erranliu/SteerLine/internal/execution/backend"
)

func localTestConfig(t *testing.T, call func([]string) ([]byte, error)) Config {
	t.Helper()
	return Config{Repository: `C:/repo`, Agent: "codex", Settings: Settings{Target: "local", CoordinatorTerminal: "coordinator", PollInterval: "1ms"}, Origin: func(context.Context) ([]byte, error) { return []byte("https://forge/owner/repo.git"), nil }, Call: func(_ context.Context, _ time.Duration, args ...string) ([]byte, error) {
		if flag(args, "--on") != "" || flag(args, "--environment") != "" || flag(args, "--pairing-code") != "" {
			t.Fatalf("local operation crossed target boundary: %v", args)
		}
		if args[0] == "environment" {
			t.Fatal("local target consulted remote pairing")
		}
		if args[0] == "--version" {
			return []byte("1.4.223"), nil
		}
		if args[0] == "status" {
			return wire("local-runtime", map[string]any{"target": map[string]string{"kind": "local"}, "runtime": map[string]any{"state": "ready", "reachable": true, "runtimeId": "local-runtime", "appVersion": "1.4.223"}}), nil
		}
		if args[0] == "repo" {
			if args[1] == "list" {
				return wire("local-runtime", map[string]any{"repos": []any{map[string]string{"id": "repo", "path": "C:/repo", "kind": "git"}}}), nil
			}
			return wire("local-runtime", map[string]any{"repo": map[string]any{"id": "repo", "kind": "git", "gitRemoteIdentity": map[string]string{"remoteUrl": "https://forge/owner/repo.git"}}}), nil
		}
		if args[0] == "terminal" {
			return wire("local-runtime", map[string]any{"terminal": map[string]any{"handle": "coordinator", "incarnationId": "coord-inc", "connected": true}}), nil
		}
		return call(args)
	}}
}

func localWorker(task, dispatch, state string) map[string]any {
	status := "dispatched"
	if state == "succeeded" {
		status = "completed"
	}
	return map[string]any{"dispatch": map[string]string{"id": dispatch, "runId": "run", "taskId": task, "status": status}, "worker": map[string]string{"dispatchId": dispatch, "runtimeEpoch": "local-runtime", "worktreeId": "repo::C:/worker", "agentTerminalHandle": "agent", "state": state}, "projection": map[string]any{"host": map[string]string{"id": "local"}, "liveness": map[string]string{"verdict": "live"}}, "terminal": map[string]string{"handle": "agent", "incarnationId": "inc", "worktreeId": "repo::C:/worker", "worktreePath": "C:/worker", "branch": "worker"}, "observation": map[string]any{"exactWorker": true, "agentWait": nil}}
}

func TestSupervisedLocalTargetProfile(t *testing.T) {
	p := ProfileConfig{Target: "local", Executable: "orca", Shell: "powershell.exe", WaitTimeout: "2m", PollInterval: "1s", CapabilityReportVersion: 1}
	raw, err := EncodeProfileConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeProfileConfig(raw)
	if err != nil || got.Target != "local" {
		t.Fatalf("local profile: %+v %v", got, err)
	}
	for _, bad := range []ProfileConfig{
		func() ProfileConfig { p.ConnectionID = "foreign"; return p }(),
		{Target: "unknown", Executable: "orca", Shell: "pwsh", WaitTimeout: "2m", PollInterval: "1s", CapabilityReportVersion: 1},
	} {
		if bad.Validate() == nil {
			t.Fatal("ambiguous target accepted")
		}
	}
	c := localTestConfig(t, func([]string) ([]byte, error) { return nil, errors.New("unexpected") })
	if c.Target().ID == (Config{Settings: Settings{ConnectionID: "local"}}).Target().ID {
		t.Fatal("local target collides with a connection named local")
	}
}

func TestSupervisedLocalTargetPreflightRejectsRedirectionBeforeStart(t *testing.T) {
	for _, scenario := range []string{"environment", "missing-target", "wrong-repo", "unregistered", "changed-runtime"} {
		t.Run(scenario, func(t *testing.T) {
			mutations := 0
			c := localTestConfig(t, func([]string) ([]byte, error) { mutations++; return nil, errors.New("unexpected start") })
			call := c.Call
			if scenario == "changed-runtime" {
				c.ExpectedRemoteRuntime = "foreign"
			}
			c.Call = func(ctx context.Context, budget time.Duration, args ...string) ([]byte, error) {
				raw, err := call(ctx, budget, args...)
				if err != nil || args[0] == "--version" {
					return raw, err
				}
				var value map[string]any
				if json.Unmarshal(raw, &value) != nil {
					return raw, nil
				}
				result := value["result"].(map[string]any)
				switch {
				case args[0] == "status" && scenario == "environment":
					result["target"] = map[string]string{"kind": "environment"}
				case args[0] == "status" && scenario == "missing-target":
					delete(result, "target")
				case args[0] == "repo" && args[1] == "show" && scenario == "wrong-repo":
					result["repo"].(map[string]any)["id"] = "foreign"
				case args[0] == "repo" && args[1] == "list" && scenario == "unregistered":
					result["repos"] = []any{}
				}
				return json.Marshal(value)
			}
			if _, err := New(c).StartTask(context.Background(), "run", "task", "repo::C:/worker", nil); err == nil || mutations != 0 {
				t.Fatal("invalid local preflight sent worker input")
			}
		})
	}
}

func TestSupervisedLocalTargetStartContinuationAndIdentity(t *testing.T) {
	starts := 0
	config := localTestConfig(t, func(args []string) ([]byte, error) {
		switch args[1] {
		case "worker-show":
			return wire("local-runtime", localWorker("old", "old-dispatch", "succeeded")), nil
		case "worker-start":
			starts++
			if starts == 1 && flag(args, "--agent") != "codex" || starts == 2 && (flag(args, "--terminal") != "agent" || flag(args, "--agent") != "") {
				t.Fatalf("wrong launch/reuse: %v", args)
			}
			return wire("local-runtime", map[string]any{"runId": "run", "taskId": "next", "dispatchId": "next-dispatch", "state": "ready", "stage": "input_accepted", "worker": map[string]string{"worktreeId": "repo::C:/worker"}, "launch": map[string]any{"effective": map[string]string{"agent": "codex"}}, "mutation": map[string]string{"requestId": "request"}}), nil
		default:
			t.Fatalf("unexpected call %v", args)
			return nil, nil
		}
	})
	client := New(config)
	if _, err := client.StartTask(context.Background(), "run", "next", "repo::C:/worker", nil); err != nil {
		t.Fatal(err)
	}
	shown, err := client.Show(context.Background(), StartReceipt{RunID: "run", TaskID: "old", DispatchID: "old-dispatch", WorktreeID: "repo::C:/worker", CenterRuntimeID: "local-runtime"})
	if err != nil {
		t.Fatal(err)
	}
	if shown.Identity.ExecutionHost != "local" || shown.Identity.RuntimeID != shown.Identity.CoordinatorRuntimeID {
		t.Fatal("local identity fabricated a peer")
	}
	if _, err := client.StartTask(context.Background(), "run", "next", shown.Identity.WorktreeID, &shown.Identity); err != nil {
		t.Fatal(err)
	}
	shown.Identity.CoordinatorRuntimeID = "foreign"
	if _, err := client.StartTask(context.Background(), "run", "next", shown.Identity.WorktreeID, &shown.Identity); err == nil || starts != 2 {
		t.Fatal("mismatch launched a replacement")
	}
	for _, kind := range []string{"server", "epoch", "host", "worker-runtime", "terminal", "dispatch", "unreachable"} {
		t.Run(kind, func(t *testing.T) {
			c := localTestConfig(t, func([]string) ([]byte, error) {
				payload := localWorker("old", "old-dispatch", "succeeded")
				switch kind {
				case "server":
					payload["server"] = map[string]string{"environmentId": "remote"}
				case "epoch":
					payload["remoteRuntimeEpoch"] = "foreign"
				case "host":
					payload["projection"].(map[string]any)["host"] = map[string]string{"id": "remote"}
				case "worker-runtime":
					payload["worker"].(map[string]string)["runtimeEpoch"] = "foreign"
				case "terminal":
					payload["terminal"] = nil
				case "dispatch":
					payload["dispatch"].(map[string]string)["id"] = "foreign"
				case "unreachable":
					return nil, errors.New("contact lost")
				}
				return wire("local-runtime", payload), nil
			})
			if _, err := New(c).Show(context.Background(), StartReceipt{RunID: "run", TaskID: "old", DispatchID: "old-dispatch", WorktreeID: "repo::C:/worker", CenterRuntimeID: "local-runtime"}); err == nil {
				t.Fatal("foreign local observation accepted")
			}
		})
	}
}

func TestSupervisedLocalTargetWorkspaceAndLifecycle(t *testing.T) {
	config := localTestConfig(t, func(args []string) ([]byte, error) {
		if args[0] != "worktree" {
			t.Fatalf("unexpected call %v", args)
		}
		return wire("local-runtime", map[string]any{"worktree": map[string]string{"id": "repo::C:/worker", "repoId": "repo", "path": "C:/worker", "branch": "worker", "displayName": "worker", "baseRef": strings.Repeat("a", 40), "head": strings.Repeat("a", 40)}}), nil
	})
	pair, err := Preflight(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	if pair.CenterRuntime != pair.RemoteRuntime || pair.EnvironmentID != "local" {
		t.Fatalf("wrong host proof: %+v", pair)
	}
	if _, err := New(config).CreateWorktree(context.Background(), pair, "worker", strings.Repeat("a", 40)); err != nil {
		t.Fatal(err)
	}
	if _, err := New(config).InspectWorktree(context.Background(), pair, "repo::C:/worker", "C:/worker", "worker", strings.Repeat("a", 40), ""); err != nil {
		t.Fatal(err)
	}
	setup, snapshot := remoteLifecycleSetup(t)
	p := ProfileConfig{Target: "local", Executable: "orca", Shell: "pwsh", WaitTimeout: "2m", PollInterval: "1s", CapabilityReportVersion: 1}
	raw, _ := EncodeProfileConfig(p)
	setup.ProfileData[0], err = b.NewImplementationDataEnvelope(setup.ProfileData[0], raw)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Ref.Target = config.Target()
	snapshot.Session.Target = snapshot.Ref.Target
	setup.Snapshot = snapshot
	port, err := NewExecutionLifecycle(setup)
	if err != nil {
		t.Fatal(err)
	}
	if port.PolicyCapabilities().ServiceStopBehavior != ServiceStopBehavior {
		t.Fatal("local supervised changed stop behavior")
	}
	if _, err := port.StopLifecycle(context.Background(), b.LifecycleRequest{}, nil); err == nil {
		t.Fatal("local supervised acquired cancellation")
	}
	snapshot.WorkflowPhase = "completed"
	snapshot.TaskDelivered = true
	if next, err := port.Next(context.Background(), snapshot); err != nil || !next.Done {
		t.Fatal("local supervised acquired auto-release")
	}
	resources, err := BuildNativeResource(PolicyResourceIdentity{ProjectID: snapshot.Session.ProjectID, ExecutionID: snapshot.Session.ExecutionID, AttemptID: snapshot.Session.AttemptID, Generation: snapshot.ResourceGeneration, ConfigDigest: snapshot.ConfigurationDigest}, setup.ProfileID, setup.AcceptedConfigVersion, "C:/worker", "codex", setup.ProfileData)
	if err != nil {
		t.Fatal(err)
	}
	var frozen NativeResourcePayload
	if json.Unmarshal(resources.Payload, &frozen) != nil || frozen.Target != "local" || frozen.LaunchSource != "local-orca-native" || frozen.ConnectionID != "" {
		t.Fatal("local native resource changed placement")
	}
	// State shares the existing supervised identity and restores without dispatch.
	lifecycle := port.(*ExecutionLifecycle)
	state := orcaworker.NativeExecutionState{RemoteRuntimeID: "local-runtime", Coordinator: b.ProcessIdentity{Kind: "orca-remote-coordinator", Terminal: "coordinator", RuntimeID: "local-runtime"}}
	envelopes, err := lifecycle.stateEnvelopes(snapshot, state)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Implementation = envelopes
	if restored, found, err := lifecycle.readState(snapshot); err != nil || !found || restored.Coordinator.RuntimeID != "local-runtime" {
		t.Fatal("local restore lost native state")
	}
}

func TestSupervisedLocalTargetQuestionCompletionAndRecovery(t *testing.T) {
	setup, snapshot := remoteLifecycleSetup(t)
	checks, replies := 0, 0
	config := localTestConfig(t, func(args []string) ([]byte, error) {
		switch args[1] {
		case "run-show":
			return wire("local-runtime", map[string]any{"run": map[string]string{"id": "run", "coordinator_handle": "coordinator"}}), nil
		case "worker-read":
			return wire("local-runtime", replyOutput("Complete supervised response")), nil
		case "worker-show":
			return wire("local-runtime", localWorker("task", "dispatch", "succeeded")), nil
		case "check":
			if flag(args, "--ack") != "" {
				return wire("local-runtime", map[string]any{"acknowledged": flag(args, "--ack"), "mutation": map[string]string{"requestId": "ack-request"}}), nil
			}
			checks++
			kind, id, body := "question", "question", "Choose an answer"
			payload := map[string]any{"taskId": "task", "dispatchId": "dispatch", "question": body}
			if checks > 1 {
				kind, id, body = "worker_done", "done", "Complete supervised response"
				payload = map[string]any{"taskId": "task", "dispatchId": "dispatch", "outcome": "succeeded"}
			}
			raw, _ := json.Marshal(payload)
			return wire("local-runtime", map[string]any{"runId": "run", "deliveryId": id + "-delivery", "messages": []Message{{ID: id, RunID: "run", ToHandle: "run:run", Type: kind, FromHandle: "agent", Body: body, Payload: raw}}}), nil
		case "reply":
			replies++
			if flag(args, "--id") != "question" || flag(args, "--body") != "yes" {
				t.Fatalf("reply changed question: %v", args)
			}
			return wire("local-runtime", map[string]any{"message": map[string]string{"from_handle": "run:run", "to_handle": "dispatch:dispatch"}, "question": map[string]string{"message_id": "question", "run_id": "run", "dispatch_id": "dispatch", "status": "answered", "answer_body": "yes"}, "mutation": map[string]string{"requestId": "reply-request"}}), nil
		default:
			t.Fatalf("unexpected mutation %v", args)
			return nil, nil
		}
	})
	target := config.Target()
	target.OrchestrationRun = "run"
	target.ExternalTask = "task"
	target.Dispatch = "dispatch"
	target.ExecutionHost = "local"
	target.WorktreeID = "repo::C:/worker"
	target.CoordinatorTerminal = "coordinator"
	snapshot.Ref.Target = target
	snapshot.Session.Target = target
	snapshot.Ref.External = b.ProcessIdentity{Kind: "orca_supervised", RuntimeID: "local-runtime", CoordinatorRuntimeID: "local-runtime", Terminal: "agent", Incarnation: "inc", OrchestrationRun: "run", Task: "task", Dispatch: "dispatch", ExecutionHost: "local", WorktreeID: target.WorktreeID}
	snapshot.Ref.WorkspacePath = "C:/worker"
	snapshot.Ref.WorkspaceBranch = "worker"
	snapshot.Session.CurrentTurnID = "00000000000000000000000000000007"
	snapshot.Session.Generation = "inc"
	state := orcaworker.NativeExecutionState{RemoteRuntimeID: "local-runtime", Coordinator: b.ProcessIdentity{Terminal: "coordinator", RuntimeID: "local-runtime", Incarnation: "coord-inc"}, Turns: []orcaworker.NativeTurnState{{TurnID: snapshot.Session.CurrentTurnID, InputVersion: 1, ResultProcess: &snapshot.Ref.External, TaskID: "task", DispatchID: "dispatch", Purpose: "business"}}}
	setup.Snapshot = snapshot
	port := &ExecutionLifecycle{setup: setup, config: config, client: New(config)}
	var err error
	snapshot.Implementation, err = port.stateEnvelopes(snapshot, state)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	observer := newLifecycleObserver(ctx, port, snapshot)
	t.Cleanup(func() {
		if err := observer.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	var kinds []b.EventKind
	for {
		select {
		case event, ok := <-observer.Events():
			if !ok {
				goto observed
			}
			kinds = append(kinds, event.Fact.Kind)
			if len(event.Implementation) > 0 {
				snapshot.Implementation = event.Implementation
			}
			if event.Fact.Kind == b.EventQuestion {
				snapshot.QuestionRoute = event.Fact.QuestionRoute
				snapshot.PendingDeliveryID = event.DeliveryID
				answer := b.Answer{Answers: map[string][]string{"question": {"yes"}}}
				request := remoteLifecycleRequest(snapshot, b.LifecycleDeliverFeedback, b.LifecycleInput{Feedback: &b.FeedbackInput{Answer: &answer}})
				if receipt, _, err := port.Execute(ctx, request); err != nil || receipt.Disposition != b.OperationConfirmed {
					t.Fatalf("local question reply: %+v %v", receipt, err)
				}
				request = remoteLifecycleRequest(snapshot, b.LifecycleAcknowledge, b.LifecycleInput{Ack: &b.AcknowledgeInput{DeliveryID: event.DeliveryID}})
				if _, _, err := port.Execute(ctx, request); err != nil {
					t.Fatal(err)
				}
			}
			event.Persisted <- nil
			event.Processed <- nil
		case <-ctx.Done():
			t.Fatal("local observer did not finish")
		}
	}
observed:
	if err := <-observer.Done(); err != nil {
		t.Fatal(err)
	}
	if replies != 1 || len(kinds) != 3 || kinds[0] != b.EventStarted || kinds[1] != b.EventQuestion || kinds[2] != b.EventCompleted {
		t.Fatalf("local supervised events %v replies=%d", kinds, replies)
	}
	receipt, recoveredObserver, err := port.ObserveLifecycle(ctx, snapshot)
	if err != nil || recoveredObserver != nil || !receipt.State.TurnCompleted || receipt.TurnEvidence == nil || !receipt.TurnEvidence.FinalVerified || receipt.TurnEvidence.LastMessage != "Complete supervised response" {
		t.Fatalf("restore did not use original completed Dispatch: %+v %v", receipt, err)
	}
}
