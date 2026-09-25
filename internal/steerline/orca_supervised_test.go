package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fakeSupervisedEnvelope(runtime string, result any) int {
	data, err := json.Marshal(map[string]any{"ok": true, "result": result, "_meta": map[string]any{"runtimeId": runtime}})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	fmt.Println(string(data))
	return 0
}

func fakeSupervisedOrca(args []string) int {
	if len(args) == 1 && args[0] == "--version" {
		fmt.Println("1.4.209")
		return 0
	}
	if len(args) == 0 {
		return 2
	}
	switch args[0] {
	case "status":
		remote := arg(args, "--environment") != ""
		version, runtimeID := "1.4.209", "fake-center-runtime"
		if remote {
			version, runtimeID = "1.4.210", "fake-remote-runtime"
			if override := os.Getenv("STEERLINE_FAKE_SUPERVISED_REMOTE_VERSION"); override != "" {
				version = override
			}
		}
		caps := []string{"orchestration.federation.v1", "orchestration.federation-control-mail.v1", "orchestration.federation-lifecycle-settlement.v1", "orchestration.worker-launch-preferences.v1", "orchestration.federation-structured-read.v1", "orchestration.federation-fleet-snapshot.v1", "orchestration.federation-release-archive.v1", "agent.launch.v2"}
		if remote && os.Getenv("STEERLINE_FAKE_SUPERVISED_MISSING_CAPABILITY") != "" {
			caps = append(caps[:0:0], caps[:len(caps)-1]...)
		}
		return fakeSupervisedEnvelope(runtimeID, map[string]any{"runtime": map[string]any{"state": "ready", "reachable": true, "runtimeId": runtimeID, "appVersion": version, "capabilities": caps}})
	case "worktree":
		if len(args) < 2 || args[1] != "show" {
			return fakeOrcaErr("invalid_argument", "unexpected worktree verb")
		}
		worktreeID := strings.TrimPrefix(arg(args, "--worktree"), "id:")
		parts := strings.SplitN(worktreeID, "::", 2)
		if len(parts) != 2 || parts[0] != "repo-test" {
			return fakeOrcaErr("worktree_not_found", "unknown worktree")
		}
		head := os.Getenv("STEERLINE_FAKE_SUPERVISED_HEAD")
		if head == "" {
			head = strings.Repeat("a", 40)
		}
		name := filepath.Base(parts[1])
		return fakeSupervisedEnvelope("fake-remote-runtime", map[string]any{"worktree": map[string]any{"id": worktreeID, "repoId": "repo-test", "path": parts[1], "head": head, "branch": "refs/heads/erranliu/" + name}})
	case "environment":
		pairedID := os.Getenv("STEERLINE_FAKE_SUPERVISED_PAIRED_DEVICE_ID")
		if pairedID == "" {
			pairedID = "home-connection"
		}
		return fakeSupervisedEnvelope("fake-center-runtime", map[string]any{"environments": []any{map[string]any{"id": "env-home", "runtimeId": "fake-remote-runtime", "pairedDeviceId": pairedID}}})
	case "repo":
		if len(args) < 2 || args[1] != "show" || arg(args, "--repo") != "id:repo-test" || arg(args, "--environment") != "env-home" {
			return fakeOrcaErr("repo_not_found", "unexpected repository selector")
		}
		return fakeSupervisedEnvelope("fake-remote-runtime", map[string]any{"repo": map[string]any{"id": "repo-test", "kind": "git", "gitRemoteIdentity": map[string]any{"remoteUrl": os.Getenv("STEERLINE_FAKE_SUPERVISED_REMOTE_URL")}}})
	case "terminal":
		if len(args) < 2 || args[1] != "show" || arg(args, "--terminal") != "term-coordinator" {
			return fakeOrcaErr("terminal_handle_stale", "coordinator terminal missing")
		}
		return fakeSupervisedEnvelope("fake-center-runtime", map[string]any{"terminal": map[string]any{"handle": "term-coordinator", "connected": true, "orphaned": false}})
	case "orchestration":
		if len(args) < 2 {
			return fakeOrcaErr("invalid_argument", "missing orchestration verb")
		}
		switch args[1] {
		case "run-create":
			if arg(args, "--from") != "term-coordinator" || arg(args, "--retry-request") != "" || arg(args, "--objective") == "" {
				return fakeOrcaErr("invalid_argument", "missing Run identity")
			}
			return fakeSupervisedEnvelope("fake-center-runtime", map[string]any{"run": map[string]any{"id": "orca-run-test", "objective": arg(args, "--objective")}, "mutation": map[string]any{"requestId": "request-run-create"}})
		case "worker-start":
			if arg(args, "--from") != "term-coordinator" || arg(args, "--on") != "env-home" || arg(args, "--agent") != "codex" || arg(args, "--model") != "gpt-6-luna" || arg(args, "--effort") != "high" || arg(args, "--retry-request") != "" {
				return fakeOrcaErr("invalid_argument", "launch identity/model was not fixed")
			}
			if marker := os.Getenv("STEERLINE_FAKE_SUPERVISED_START_MARKER"); marker != "" {
				file, err := os.OpenFile(marker, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
				if err != nil {
					return fakeOrcaErr("internal", "worker-start marker could not be written")
				}
				_, err = file.WriteString("attempt\n")
				_ = file.Close()
				if err != nil {
					return fakeOrcaErr("internal", "worker-start marker could not be written")
				}
			}
			if os.Getenv("STEERLINE_FAKE_SUPERVISED_DROP_START_RESPONSE") == "1" {
				fmt.Fprintln(os.Stderr, "injected worker-start response loss after application")
				return 75
			}
			worktree := os.Getenv("STEERLINE_FAKE_SUPERVISED_WORKTREE_ID")
			if worktree == "" {
				worktree = "repo-test::C:/remote/steerline-run"
			}
			if base := arg(args, "--base-branch"); base != "" {
				if pinned := os.Getenv("STEERLINE_FAKE_SUPERVISED_HEAD"); pinned != "" && base != pinned {
					return fakeOrcaErr("base_mismatch", "fixed base did not match")
				}
			}
			return fakeSupervisedEnvelope("fake-center-runtime", map[string]any{"runId": arg(args, "--run"), "taskId": "task-test", "dispatchId": "dispatch-test", "state": "ready", "stage": "input_accepted", "server": map[string]any{"environmentId": "env-home"}, "launch": map[string]any{"effective": map[string]any{"agent": "codex", "model": "gpt-6-luna", "effort": "high"}}, "worker": map[string]any{"worktreeId": worktree, "effects": []any{map[string]any{"kind": "worktree", "action": "created_top_level", "id": worktree}}}, "mutation": map[string]any{"requestId": "request-worker-start"}})
		case "worker-show":
			if os.Getenv("STEERLINE_FAKE_SUPERVISED_OBSERVE_UNAVAILABLE") != "" {
				return fakeOrcaErr("transport_unavailable", "execution host is temporarily unreachable")
			}
			worktree := os.Getenv("STEERLINE_FAKE_SUPERVISED_WORKTREE_ID")
			if worktree == "" {
				worktree = "repo-test::C:/remote/steerline-run"
			}
			dispatchState := os.Getenv("STEERLINE_FAKE_SUPERVISED_DISPATCH_STATE")
			if dispatchState == "" {
				dispatchState = "dispatched"
			}
			worktreePath := strings.SplitN(worktree, "::", 2)[1]
			name := filepath.Base(worktreePath)
			runtimeEpoch := os.Getenv("STEERLINE_FAKE_SUPERVISED_WORKER_EPOCH")
			if runtimeEpoch == "" {
				runtimeEpoch = "fake-remote-runtime"
			}
			observation := map[string]any{"exactWorker": true}
			switch os.Getenv("STEERLINE_FAKE_SUPERVISED_AGENT_WAIT") {
			case "present":
				observation["agentWait"] = map[string]any{"kind": "host_permission"}
			case "none":
				observation["agentWait"] = nil
			}
			return fakeSupervisedEnvelope("fake-center-runtime", map[string]any{"dispatch": map[string]any{"id": "dispatch-test", "runId": "orca-run-test", "taskId": "task-test", "status": dispatchState}, "worker": map[string]any{"dispatchId": "dispatch-test", "worktreeId": worktree}, "projection": map[string]any{"liveness": map[string]any{"verdict": "live"}, "host": map[string]any{"id": "env-home"}}, "server": map[string]any{"environmentId": "env-home"}, "remoteRuntimeEpoch": runtimeEpoch, "terminal": map[string]any{"handle": "terminal-test", "incarnationId": "inc-test", "worktreeId": worktree, "worktreePath": worktreePath, "branch": "refs/heads/erranliu/" + name}, "observation": observation})
		case "worker-read":
			return fakeSupervisedEnvelope("fake-center-runtime", map[string]any{"dispatchId": arg(args, "--dispatch"), "source": "transcript", "cursor": "cursor-1", "contentComplete": true, "clipping": []string{}, "status": map[string]any{"worker": "ready", "terminal": "running", "liveness": "live"}, "transcript": map[string]any{"messages": []any{}}})
		case "check":
			return fakeSupervisedEnvelope("fake-center-runtime", map[string]any{"runId": arg(args, "--run"), "deliveryId": nil, "messages": []any{}, "count": 0, "timedOut": true, "connectionLost": false, "cancelled": false})
		case "worker-release":
			return fakeSupervisedEnvelope("fake-center-runtime", map[string]any{"dispatchId": arg(args, "--dispatch"), "state": "released", "processAction": "closed_agent_terminal", "residualResources": []string{}, "mutation": map[string]any{"requestId": "request-worker-release"}})
		case "reply":
			if os.Getenv("STEERLINE_FAKE_SUPERVISED_DENY_REPLY") != "" {
				return fakeOrcaErr("unexpected_mutation", "reply was resent during read-only reconciliation")
			}
			if arg(args, "--run") != "orca-run-test" || arg(args, "--from") != "term-coordinator" || arg(args, "--id") == "" || arg(args, "--body") == "" {
				return fakeOrcaErr("invalid_argument", "reply lost its original question or Run")
			}
			return fakeSupervisedEnvelope("fake-center-runtime", map[string]any{"message": map[string]any{"from_handle": "run:orca-run-test", "to_handle": "dispatch:dispatch-test"}, "question": map[string]any{"message_id": arg(args, "--id"), "run_id": "orca-run-test", "dispatch_id": "dispatch-test", "answer_body": arg(args, "--body"), "status": "answered"}, "mutation": map[string]any{"requestId": "request-reply"}})
		case "inbox":
			return fakeSupervisedEnvelope("fake-center-runtime", map[string]any{"messages": []any{map[string]any{"id": "answer-message-test", "from_handle": "run:orca-run-test", "to_handle": "dispatch:dispatch-test", "thread_id": "relay-answer-test", "body": "left", "type": "status"}}})
		default:
			return fakeOrcaErr("unknown", "unknown supervised orchestration verb")
		}
	default:
		return fakeOrcaErr("unknown", "unknown supervised Orca test command")
	}
}

func supervisedFixtureConfig(t *testing.T) Config {
	t.Helper()
	f := newFixture(t)
	return supervisedConfigForFixture(t, f)
}

func supervisedConfigForFixture(t *testing.T, f *fixture) Config {
	t.Helper()
	remoteBytes, err := gitRunner(f.c, f.c.Repository).Output(context.Background(), f.c.Git, "remote", "get-url", f.c.Remote)
	if err != nil {
		t.Fatal(err)
	}
	remoteURL := strings.TrimSpace(string(remoteBytes))
	t.Setenv("STEERLINE_FAKE_SUPERVISED_REMOTE_URL", remoteURL)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	c := f.c
	c.Host = "orca_supervised"
	c.Agent = "codex"
	c.Model = "gpt-6-luna"
	c.ReasoningEffort = "high"
	c.Orca = OrcaConfig{Executable: exe, Args: []string{"--fake", "orca-supervised"}, Shell: "powershell", WaitTimeout: "5s", Supervised: &SupervisedOrcaConfig{ConnectionID: "home-connection", EnvironmentID: "env-home", RepositorySelector: "id:repo-test", CoordinatorTerminal: "term-coordinator", ExpectedCodexVersion: "codex-cli 0.156.1", PollInterval: "2s", CapabilityReportVersion: 1}}
	return c
}

func TestSupervisedOrcaPreflightPinsRemoteIdentityAndVersionFamily(t *testing.T) {
	c := supervisedFixtureConfig(t)
	pair, err := probeSupervisedOrcaTarget(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	if pair.CenterVersion != "1.4.209" || pair.RemoteVersion != "1.4.210" || pair.EnvironmentID != "env-home" || pair.RepositoryID != "repo-test" || pair.Coordinator != "term-coordinator" {
		t.Fatalf("preflight identity = %+v", pair)
	}
	if got := targetFor(c); got.ID != "orca-supervised:home-connection" || got.ExternalRuntime != "env-home" {
		t.Fatalf("target = %+v", got)
	}
}

func TestSupervisedHostWaitAndLaunchFailureRequireManualCheck(t *testing.T) {
	c := supervisedFixtureConfig(t)
	b := orcaSupervisedBackend{config: c}
	expected := supervisedStartReceipt{RunID: "orca-run-test", TaskID: "task-test", DispatchID: "dispatch-test", WorktreeID: "repo-test::C:/remote/steerline-run", CenterRuntimeID: "fake-center-runtime"}
	for _, tc := range []struct{ raw, want string }{{"present", "present"}, {"none", "none"}, {"", "unknown"}} {
		t.Setenv("STEERLINE_FAKE_SUPERVISED_AGENT_WAIT", tc.raw)
		observed, err := b.showWorker(context.Background(), expected)
		if err != nil || observed.AgentWaitState != tc.want {
			t.Fatalf("agentWait %q = %+v, %v; want %q", tc.raw, observed, err, tc.want)
		}
	}
	t.Setenv("STEERLINE_FAKE_SUPERVISED_AGENT_WAIT", "present")
	ref := ExecutionRef{Version: ExecutionRecordVersion, Backend: BackendOrca, Protocol: ProtocolOrcaSupervised, Target: targetFor(c), Agent: AgentSpec{Name: "codex"}, External: ProcessIdentity{Kind: "orca_supervised", RuntimeID: "fake-remote-runtime", CoordinatorRuntimeID: "fake-center-runtime", Terminal: "terminal-test", Incarnation: "inc-test", OrchestrationRun: "orca-run-test", Task: "task-test", Dispatch: "dispatch-test", ExecutionHost: "env-home", WorktreeID: expected.WorktreeID}}
	ref.Target.OrchestrationRun, ref.Target.ExternalTask, ref.Target.Dispatch = ref.External.OrchestrationRun, ref.External.Task, ref.External.Dispatch
	ref.Target.ExecutionHost, ref.Target.WorktreeID = ref.External.ExecutionHost, ref.External.WorktreeID
	if observed, err := b.Observe(context.Background(), ref); err != nil || observed.Diagnostic != "host_interaction_pending" {
		t.Fatalf("host wait observation = %+v, %v", observed, err)
	}
	batch := OutputBatch{Data: mustJSON(map[string]any{"transcript": map[string]any{"messages": []any{map[string]any{"role": "tool", "blocks": []any{map[string]any{"type": "tool-result", "output": "orchestrator_helper_launch_canceled: ShellExecuteExW failed to launch setup helper: 1223"}}}}}}), Source: "transcript", NextCursor: "host-cursor", ObservedAt: now()}
	activity := supervisedOutputActivity(batch, "local-run", "local-task")
	if activity.Kind != "error" || activity.Status != "manual_host_check" || !strings.Contains(activity.Detail, "will not treat this as a business question") {
		t.Fatalf("host command launch activity = %+v", activity)
	}
	observerCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	worker := &Worker{Events: make(chan WorkerEvent, 16), Done: make(chan error, 1), observationDone: make(chan struct{})}
	input := MinimalStepInput{WorkflowRunID: "local-run", TaskID: "local-task", AttemptID: "local-attempt"}
	go b.observeWorker(observerCtx, worker, ref, StepAttemptRecord{AttemptID: input.AttemptID, Status: "dispatched"}, input, filepath.Join(t.TempDir(), "result.json"))
	seenPending, seenCleared := false, false
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for !seenCleared {
		select {
		case event := <-worker.Events:
			if event.Activity.Status == "manual_host_check" {
				seenPending = true
				t.Setenv("STEERLINE_FAKE_SUPERVISED_AGENT_WAIT", "none")
			}
			if event.Activity.Title == "Remote host interaction cleared" {
				seenCleared = true
				if event.Activity.ItemID != "host-wait:dispatch-test" || event.Activity.Status != "completed" {
					t.Fatalf("host wait clear activity = %+v", event.Activity)
				}
			}
			event.Processed <- nil
		case <-deadline.C:
			t.Fatal("observer did not publish and clear the exact host wait")
		}
	}
	if !seenPending {
		t.Fatal("observer cleared a host wait that it never reported")
	}
	cancel()
	<-worker.observationDone
}

func TestSupervisedOrcaPreflightRejectsVersionOrCapabilityDrift(t *testing.T) {
	c := supervisedFixtureConfig(t)
	t.Setenv("STEERLINE_FAKE_SUPERVISED_REMOTE_VERSION", "1.5.0")
	if _, err := probeSupervisedOrcaTarget(context.Background(), c); err == nil || !strings.Contains(err.Error(), "version-family mismatch") {
		t.Fatalf("version mismatch error = %v", err)
	}
	t.Setenv("STEERLINE_FAKE_SUPERVISED_REMOTE_VERSION", "1.4.210")
	t.Setenv("STEERLINE_FAKE_SUPERVISED_MISSING_CAPABILITY", "1")
	if _, err := probeSupervisedOrcaTarget(context.Background(), c); err == nil || !strings.Contains(err.Error(), "agent.launch.v2") {
		t.Fatalf("missing capability error = %v", err)
	}
	t.Setenv("STEERLINE_FAKE_SUPERVISED_MISSING_CAPABILITY", "")
	t.Setenv("STEERLINE_FAKE_SUPERVISED_PAIRED_DEVICE_ID", "replacement-device")
	if _, err := probeSupervisedOrcaTarget(context.Background(), c); err == nil || !strings.Contains(err.Error(), "paired-device") {
		t.Fatalf("connection replacement error = %v", err)
	}
}

func TestSupervisedOrcaRequiresPinnedModelAndKeepsStopUnsupported(t *testing.T) {
	c := supervisedFixtureConfig(t)
	c.Model = ""
	if err := validateSupervisedOrcaConfig(&c); err == nil {
		t.Fatal("missing model unexpectedly accepted")
	}
	c.Model = "gpt-6-luna"
	c.Orca.Supervised.RemoteCancelAvailable = true
	if err := validateSupervisedOrcaConfig(&c); err == nil {
		t.Fatal("remote cancellation was enabled")
	}
	c.Orca.Supervised.RemoteCancelAvailable = false
	report, err := (orcaSupervisedBackend{config: c}).Probe(context.Background(), targetFor(c), agentFor(c))
	if err != nil {
		t.Fatal(err)
	}
	if capabilityStatus(report, "stop") != CapabilityUnsupported || capabilityStatus(report, "start") != CapabilityNotTested {
		t.Fatalf("supervised capabilities = %+v", report.Capabilities)
	}
	if _, err = (orcaSupervisedBackend{}).StopExecution(context.Background(), ExecutionRef{}, Operation{}, nil); err == nil || !isBackendError(err, BackendUnsupported) {
		t.Fatalf("StopExecution error = %v", err)
	}
}

func TestSupervisedOrcaCapabilityGateBlocksBeforeClaim(t *testing.T) {
	f := newFixture(t, task("remote-task"))
	f.c = supervisedConfigForFixture(t, f)
	s := f.service(t)
	t.Cleanup(func() { _ = s.Close() })

	err := s.dispatch(context.Background())
	if err == nil || !strings.Contains(err.Error(), "start capability is not_tested") {
		t.Fatalf("dispatch capability gate error = %v", err)
	}
	db := readFake(f.db)
	if len(db.Claims) != 0 {
		t.Fatalf("remote task was claimed before start capability verification: %+v", db.Claims)
	}
}

func TestSupervisedOrcaExecutionBindingRequiresExactDispatchIdentity(t *testing.T) {
	c := supervisedFixtureConfig(t)
	runID, attemptID := newID(), newID()
	ref := ExecutionRef{Version: ExecutionRecordVersion, Backend: BackendOrca, Protocol: ProtocolOrcaSupervised, Target: targetFor(c), Agent: agentFor(c), RunID: runID, AttemptID: attemptID, WorkspacePath: "C:/remote/worktree", WorkspaceBranch: "erranliu/remote", BaseCommit: strings.Repeat("a", 40), External: ProcessIdentity{Kind: "orca_supervised", RuntimeID: "runtime-1", OrchestrationRun: "run-1", Task: "task-1", Dispatch: "dispatch-1", ExecutionHost: "host-1", WorktreeID: "repo-1::C:/remote/worktree"}}
	if err := validateExecutionRef(ref); err != nil {
		t.Fatalf("complete supervised binding rejected: %v", err)
	}
	ref.External.Dispatch = ""
	if err := validateExecutionRef(ref); err == nil {
		t.Fatal("supervised binding without its exact Dispatch was accepted")
	}
	backend, protocol, err := ExecutionBackendKind("orca_supervised")
	if err != nil || backend != string(BackendOrca) || protocol != string(ProtocolOrcaSupervised) {
		t.Fatalf("backend resolution = %q/%q, %v", backend, protocol, err)
	}
}

func TestSupervisedOrcaClientPinsLaunchAndReadsExactDispatch(t *testing.T) {
	c := supervisedFixtureConfig(t)
	t.Setenv("STEERLINE_FAKE_SUPERVISED_WORKTREE_ID", "repo-test::C:/remote/spec03-test")
	backend := orcaSupervisedBackend{config: c}
	workflowRunID := newID()
	runID, requestID, err := backend.createOrchestrationRun(context.Background(), workflowRunID, "spec03 fixture")
	if err != nil {
		t.Fatal(err)
	}
	if runID != "orca-run-test" {
		t.Fatalf("created Orca Run ID = %q", runID)
	}
	if requestID != "request-run-create" {
		t.Fatalf("Orca Run-create request ID = %q", requestID)
	}
	base := strings.Repeat("a", 40)
	started, err := backend.startWorker(context.Background(), runID, newID(), "prepare", "verify workspace", "new-top-level", "spec03-test", "id:repo-test", base)
	if err != nil {
		t.Fatal(err)
	}
	if started.TaskID != "task-test" || started.DispatchID != "dispatch-test" || started.WorktreeID != "repo-test::C:/remote/spec03-test" || started.PlatformRequestID != "request-worker-start" || started.State != "ready" || started.Stage != "input_accepted" {
		t.Fatalf("worker start receipt = %+v", started)
	}
	observed, err := backend.showWorker(context.Background(), started)
	if err != nil {
		t.Fatal(err)
	}
	if observed.Identity.RuntimeID != "fake-remote-runtime" || observed.Identity.OrchestrationRun != runID || observed.Identity.Task != started.TaskID || observed.Identity.Dispatch != started.DispatchID || observed.Identity.WorktreeID != started.WorktreeID || observed.Liveness != "live" || observed.WorktreePath != "C:/remote/spec03-test" || observed.Branch != "erranliu/spec03-test" {
		t.Fatalf("worker observation = %+v", observed)
	}
	target := targetFor(c)
	target.OrchestrationRun, target.ExternalTask, target.Dispatch, target.WorktreeID, target.ExecutionHost = runID, started.TaskID, started.DispatchID, started.WorktreeID, "env-home"
	ref := ExecutionRef{Version: ExecutionRecordVersion, Backend: BackendOrca, Protocol: ProtocolOrcaSupervised, Target: target, Agent: agentFor(c), RunID: workflowRunID, AttemptID: newID(), External: observed.Identity}
	t.Setenv("STEERLINE_FAKE_SUPERVISED_WORKER_EPOCH", "replacement-remote-runtime")
	if _, err := backend.Observe(context.Background(), ref); err == nil || !isBackendError(err, BackendIdentityMismatch) {
		t.Fatalf("replacement remote runtime was treated as the original Dispatch: %v", err)
	}
}

func TestSupervisedOrcaClientRejectsReturnedReplacementWorktree(t *testing.T) {
	c := supervisedFixtureConfig(t)
	backend := orcaSupervisedBackend{config: c}
	_, err := backend.startWorker(context.Background(), "orca-run-test", newID(), "verify", "read only", "id:repo-test::C:/remote/expected", "", "", "")
	if err == nil || !isBackendError(err, BackendIdentityMismatch) {
		t.Fatalf("replacement worktree error = %v", err)
	}
}

func TestSupervisedRunCreationHasSeparateDurableOperation(t *testing.T) {
	f := newFixture(t)
	f.c = supervisedConfigForFixture(t, f)
	s := f.service(t)
	run := &Run{RunID: newID(), ExecutionID: newID(), TaskID: "provider-task", DomainTaskID: "business-task", Target: targetFor(f.c)}
	s.State.Runs = append(s.State.Runs, run)
	if err := s.ensureSupervisedRun(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	if run.Target.OrchestrationRun != "orca-run-test" || run.Target.OrchestrationRunRequestID != "request-run-create" || len(s.State.Operations) != 1 {
		t.Fatalf("remote Run identity or operation missing: target=%+v operations=%+v", run.Target, s.State.Operations)
	}
	op := s.State.Operations[0]
	if op.Kind != "create_orca_run" || op.Status != "confirmed" || op.PlatformRequestID != "request-run-create" || !strings.Contains(string(op.Receipt), "orca-run-test") {
		t.Fatalf("Run-create receipt was not recorded: %+v", op)
	}
	// An uncertain replay cannot silently issue another create, even when the
	// target binding has not yet been recovered from the remote platform.
	run.Target.OrchestrationRun = ""
	if err := s.ensureSupervisedRun(context.Background(), run); err == nil || !strings.Contains(err.Error(), "reconcile the original operation") {
		t.Fatalf("duplicate Run creation was not blocked: %v", err)
	}
	if len(s.State.Operations) != 1 {
		t.Fatalf("duplicate Run-create operation was appended: %+v", s.State.Operations)
	}
}

func TestSupervisedWorkerStartResponseLossKeepsOneUnknownAttempt(t *testing.T) {
	f := newFixture(t)
	f.c = supervisedConfigForFixture(t, f)
	f.c.WorktreeRoot = filepath.Join(f.c.RunDir, "worktrees")
	contract := func(provider, entry string, command []string) StepContract {
		return StepContract{Provider: provider, Entry: entry, Version: MinimalWorkflowVersion, InputSchema: MinimalStepInputSchema, OutputSchema: MinimalStepOutputSchema, ExecutionSite: "target", Timeout: time.Minute, Command: command}
	}
	definition := DefaultMinimalWorkflowDefinition(false)
	steps := ProjectSteps{Implement: contract("agent_skill", "builtin:implement", nil), Verify: contract("command", "verify.ps1", []string{"pwsh.exe", "-File", "verify.ps1"}), PublishPR: contract("command", "publish.ps1", []string{"pwsh.exe", "-File", "publish.ps1"})}
	steps.Verify.RequiredChecks = []string{"smoke"}
	f.c.WorkflowDefinition, f.c.ProjectSteps = &definition, &steps
	s := f.service(t)
	baseBytes, err := gitRunner(f.c, f.c.Repository).Output(context.Background(), f.c.Git, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	base := strings.TrimSpace(string(baseBytes))
	runID := newID()
	intent, err := planWorkspace(f.c, "response-loss-task", runID, base)
	if err != nil {
		t.Fatal(err)
	}
	source := BeadsContext{ProjectID: "isolated-response-loss"}
	task := Task{ID: "response-loss-task", Title: "one remote prepare", Description: "read-only", Status: "open", Type: "task"}
	run := &Run{TaskID: task.ID, DomainTaskID: task.ID, Title: task.Title, RunID: runID, Claimant: f.c.Claimant, Host: f.c.Host, Backend: string(BackendOrca), Protocol: string(ProtocolOrcaSupervised), Agent: "codex", Target: targetFor(f.c), WorkspaceIntent: intent, BaseCommit: &base, ExecutionStatus: "pending", FinishTaskStatus: "pending", BeadsContext: &source}
	run.Target.OrchestrationRun = "orca-run-test"
	taskRecord, workflow, execution, attempt, err := InitializeMinimalWorkflow(f.c.identity, f.c, task, source, run)
	if err != nil {
		t.Fatal(err)
	}
	workflow.Phase, execution.Status = "executing", "active"
	s.State.Tasks = append(s.State.Tasks, taskRecord)
	s.State.WorkflowRuns = append(s.State.WorkflowRuns, workflow)
	s.State.Executions = append(s.State.Executions, execution)
	s.State.Attempts = append(s.State.Attempts, attempt)
	s.State.Runs = append(s.State.Runs, run)
	s.State.CurrentTask, s.State.SlotOccupied = ptr(task.ID), true
	if err = s.save(); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(t.TempDir(), "worker-start-attempts.txt")
	t.Setenv("STEERLINE_FAKE_SUPERVISED_START_MARKER", marker)
	t.Setenv("STEERLINE_FAKE_SUPERVISED_DROP_START_RESPONSE", "1")
	startErr := s.startMinimalStep(context.Background(), task, run, source, s.minimalAttempt(run, attempt.AttemptID))
	if startErr == nil || !isBackendError(startErr, BackendOutcomeUnknown) {
		t.Fatalf("lost worker-start response did not block as unknown: %v", startErr)
	}
	unknownAttempt := s.minimalAttempt(run, attempt.AttemptID)
	if unknownAttempt == nil || unknownAttempt.Status != "outcome_unknown" || run.FinishTaskStatus != "blocked" || run.ExecutionStatus != "unknown" || !s.State.SlotOccupied || run.Process != nil {
		t.Fatalf("uncertain worker-start did not preserve authority: run=%+v attempt=%+v slot=%t", run, unknownAttempt, s.State.SlotOccupied)
	}
	if err = s.startMinimalStep(context.Background(), task, run, source, unknownAttempt); err == nil {
		t.Fatal("uncertain worker-start was dispatched again")
	}
	callLog, err := os.ReadFile(marker)
	if err != nil || string(callLog) != "attempt\n" {
		t.Fatalf("worker-start mutation count changed after response loss: %q %v", callLog, err)
	}
}

func TestSupervisedDeliveryAckPersistsPlatformReceiptAndDeduplicates(t *testing.T) {
	f := newFixture(t)
	s := f.service(t)
	runID, executionID, attemptID := newID(), newID(), newID()
	s.State.Runs = append(s.State.Runs, &Run{RunID: runID, ExecutionID: executionID, TaskID: "provider-task", DomainTaskID: "business-task"})
	ackCalls := 0
	event := WorkerEvent{RunID: runID, AttemptID: attemptID, RemoteRunID: "run-orca-test", RemoteDeliveryID: "delivery-123", RemoteMessages: []OrcaInboxMessage{{ID: "relay-123", Type: "status"}}, Acknowledge: func() (string, error) {
		ackCalls++
		stored, err := s.domainStore.Events(context.Background(), s.Config.identity, "", "", 0, 10)
		if err != nil || len(stored) != 1 || stored[0].ExternalID != "relay-123" || len(s.State.Operations) != 1 || s.State.Operations[0].Status != "in_flight" {
			t.Fatal("raw Delivery and in-flight ack operation were not persisted before platform ack")
		}
		return "request-platform-ack-123", nil
	}}
	if err := s.event(event); err != nil {
		t.Fatal(err)
	}
	storedEvents, err := s.domainStore.Events(context.Background(), s.Config.identity, "", "", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if ackCalls != 1 || len(storedEvents) != 1 || len(s.State.Operations) != 1 {
		t.Fatalf("first Delivery application was not singular: ackCalls=%d events=%d operations=%d", ackCalls, len(storedEvents), len(s.State.Operations))
	}
	operation := s.State.Operations[0]
	if operation.Kind != "ack_orca_delivery" || operation.Status != "confirmed" || operation.PlatformRequestID != "request-platform-ack-123" || !strings.Contains(string(operation.Receipt), "delivery-123") {
		t.Fatalf("ack receipt was not durably recorded: %+v", operation)
	}
	if err := s.event(event); err != nil {
		t.Fatal(err)
	}
	storedEvents, err = s.domainStore.Events(context.Background(), s.Config.identity, "", "", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if ackCalls != 1 || len(storedEvents) != 1 || len(s.State.Operations) != 1 {
		t.Fatalf("replayed Delivery duplicated an event or ack: ackCalls=%d events=%d operations=%d", ackCalls, len(storedEvents), len(s.State.Operations))
	}
}

func TestSupervisedQuestionKeepsOriginalAttemptWaiting(t *testing.T) {
	f := newFixture(t)
	s := f.service(t)
	runID, executionID, attemptID := newID(), newID(), newID()
	resultPath := filepath.Join(f.c.RunDir, "runs", runID, "result.json")
	if err := os.MkdirAll(filepath.Dir(resultPath), 0700); err != nil {
		t.Fatal(err)
	}
	run := &Run{RunID: runID, ExecutionID: executionID, AttemptID: attemptID, TaskID: "provider-task", DomainTaskID: "business-task", ResultPath: &resultPath, Protocol: string(ProtocolOrcaSupervised), ExecutionStatus: "running", FinishTaskStatus: "pending", Process: &ProcessIdentity{Kind: "orca_supervised", OrchestrationRun: "orca-run-test", Task: "task-test", Dispatch: "dispatch-test"}}
	s.State.Runs = append(s.State.Runs, run)
	s.State.WorkflowRuns = append(s.State.WorkflowRuns, WorkflowRunRecord{WorkflowRunID: runID, Phase: "executing"})
	s.State.Executions = append(s.State.Executions, ExecutionRecord{ExecutionID: executionID, WorkflowRunID: runID, CurrentStep: "implement", Status: "active"})
	s.State.Attempts = append(s.State.Attempts, StepAttemptRecord{AttemptID: attemptID, ExecutionID: executionID, StepID: "implement", Status: "running"})
	question := OrcaInboxMessage{ID: "relay-question-1", Type: "question", FromHandle: "dispatch:dispatch-test", Payload: json.RawMessage(`{"taskId":"task-test","dispatchId":"dispatch-test","question":"Choose a route","options":["left","right"]}`)}
	event := WorkerEvent{AttemptID: attemptID, RemoteRunID: "orca-run-test", RemoteMessages: []OrcaInboxMessage{question}}
	if err := s.applySupervisedQuestions(run, event); err != nil {
		t.Fatal(err)
	}
	if run.Decision == nil || run.Decision.ID != question.ID || run.Decision.AttemptID != attemptID || run.Decision.Request.Options[0].ID != "left" || run.FinishTaskStatus != "waiting_decision" || run.ExecutionStatus != "running" || run.Process.Dispatch != "dispatch-test" {
		t.Fatalf("question detached from its original live attempt: %+v", run)
	}
	if s.State.Attempts[0].Status != "waiting_decision" || s.State.Executions[0].Status != "waiting_decision" || s.State.WorkflowRuns[0].Phase != "waiting_decision" || len(s.State.Decisions) != 1 {
		t.Fatal("question did not transition the original workflow into a durable decision")
	}
	if err := s.applySupervisedQuestions(run, event); err != nil || len(s.State.Decisions) != 1 {
		t.Fatalf("question replay was not idempotent: %v", err)
	}
	question.ID = "relay-question-2"
	event.RemoteMessages = []OrcaInboxMessage{question}
	if err := s.applySupervisedQuestions(run, event); err == nil || !isBackendError(err, BackendRejected) {
		t.Fatalf("competing question was accepted: %v", err)
	}
}

func TestMinimalImplementGateAcceptsSameCanonicalRemoteWorktree(t *testing.T) {
	base, head := strings.Repeat("a", 40), strings.Repeat("b", 40)
	path := "C:/Users/liuming/orca/workspaces/SteerlineTest/probe"
	workflow := &WorkflowRunRecord{WorkflowVersion: MinimalWorkflowVersion, Definition: WorkflowDefinition{ID: MinimalWorkflowVersion, Version: MinimalWorkflowVersion}, Phase: "executing"}
	execution := &ExecutionRecord{Status: "active", CurrentStep: "implement", Workspace: WorkspaceEvidence{Repository: "repo", Worktree: canonical(path), Branch: "erranliu/probe", Base: base, Head: base}}
	result := StepGateResult{StepID: "implement", Succeeded: true, InputRevision: base, Revision: head, Workspace: WorkspaceEvidence{Repository: "repo", Worktree: path, Branch: "erranliu/probe", Base: base, Head: head, VerifiedAt: now()}}
	if next, err := AdvanceMinimalExecution(workflow, execution, result, nil); err != nil || next != "verify" {
		t.Fatalf("equivalent Windows worktree spelling blocked the candidate: next=%q err=%v", next, err)
	}
}

func TestSupervisedTurnRequiresAssistantTranscriptEvidence(t *testing.T) {
	for _, tc := range []struct {
		name   string
		source string
		data   string
		want   bool
	}{
		{"input-only", "transcript", `{"transcript":{"messages":[{"role":"user"}]}}`, false},
		{"terminal-tail", "terminal", `{"transcript":{"messages":[{"role":"assistant"}]}}`, false},
		{"assistant-turn", "transcript", `{"transcript":{"messages":[{"role":"user"},{"role":"assistant"}]}}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := supervisedTranscriptTurnStarted(OutputBatch{Source: tc.source, Data: []byte(tc.data)}); got != tc.want {
				t.Fatalf("turn evidence = %t, want %t", got, tc.want)
			}
		})
	}
}

func TestSupervisedSettlementPersistsReleaseRequestReceipt(t *testing.T) {
	f := newFixture(t)
	f.c = supervisedConfigForFixture(t, f)
	base := strings.Repeat("a", 40)
	worktreeID := "repo-test::C:/remote/release-test"
	t.Setenv("STEERLINE_FAKE_SUPERVISED_WORKTREE_ID", worktreeID)
	t.Setenv("STEERLINE_FAKE_SUPERVISED_DISPATCH_STATE", "completed")
	definition := DefaultMinimalWorkflowDefinition(false)
	contract := func(provider, entry string, command []string) StepContract {
		return StepContract{Provider: provider, Entry: entry, Version: MinimalWorkflowVersion, InputSchema: MinimalStepInputSchema, OutputSchema: MinimalStepOutputSchema, ExecutionSite: "target", Timeout: 10 * time.Minute, Command: command}
	}
	steps := ProjectSteps{Implement: contract("agent_skill", "builtin:implement", nil), Verify: contract("command", "verify.ps1", []string{"powershell", "-NoProfile", "-File", "verify.ps1"}), PublishPR: contract("command", "publish.ps1", []string{"powershell", "-NoProfile", "-File", "publish.ps1"})}
	steps.Verify.RequiredChecks = []string{"prepare-smoke"}
	f.c.WorkflowDefinition, f.c.ProjectSteps = &definition, &steps
	s := f.service(t)
	runID := newID()
	target := targetFor(f.c)
	target.OrchestrationRun = "orca-run-test"
	target.WorktreeID = worktreeID
	target.Branch = "erranliu/release-test"
	target.BaseRevision = base
	target.ExternalTask = "task-test"
	target.Dispatch = "dispatch-test"
	target.ExecutionHost = "env-home"
	remotePath := "C:/remote/release-test"
	source := BeadsContext{ProjectID: "isolated-settlement-probe"}
	taskValue := Task{ID: "provider-task", Title: "settlement receipt probe", Status: "in_progress", Type: "task"}
	runResultPath := filepath.Join(f.c.RunDir, "runs", runID, "result.json")
	if err := os.MkdirAll(filepath.Dir(runResultPath), 0700); err != nil {
		t.Fatal(err)
	}
	run := &Run{TaskID: taskValue.ID, DomainTaskID: "business-task", RunID: runID, Claimant: f.c.Claimant, Host: f.c.Host, Backend: string(BackendOrca), Protocol: string(ProtocolOrcaSupervised), Agent: "codex", Target: target, BeadsContext: &source, PR: PRState{State: "unknown"}, ExecutionStatus: "pending", FinishTaskStatus: "pending", ResultPath: &runResultPath, WorkspaceIntent: &WorkspaceIntent{Path: remotePath, Branch: target.Branch, BaseCommit: base}, WorktreePath: &remotePath, WorktreeBranch: &target.Branch, BaseCommit: &base}
	taskRecord, workflow, execution, attempt, err := InitializeMinimalWorkflow(f.c.identity, f.c, taskValue, source, run)
	if err != nil {
		t.Fatal(err)
	}
	run.ExecutionStatus = "pending"
	run.FinishTaskStatus = "pending"
	run.DomainAttemptNumber = attempt.AttemptNumber
	workflow.Phase = "executing"
	execution.Status = "active"
	execution.CurrentStep = "prepare"
	execution.Target = target
	attempt.ExternalBinding = target
	external := ProcessIdentity{Kind: "orca_supervised", RuntimeID: "fake-remote-runtime", CoordinatorRuntimeID: "fake-center-runtime", Terminal: "terminal-test", Incarnation: "inc-test", OrchestrationRun: target.OrchestrationRun, Task: target.ExternalTask, Dispatch: target.Dispatch, ExecutionHost: target.ExecutionHost, WorktreeID: target.WorktreeID}
	run.Process, execution.Process = &external, &external
	execution.Workspace = WorkspaceEvidence{Repository: f.c.identity, Worktree: remotePath, Branch: target.Branch, Base: base, Head: base}
	for _, status := range []string{"dispatching", "dispatched"} {
		if err = TransitionStepAttempt(&attempt, status); err != nil {
			t.Fatal(err)
		}
	}
	ref := ExecutionRef{Version: ExecutionRecordVersion, Backend: BackendOrca, Protocol: ProtocolOrcaSupervised, Target: target, Agent: AgentSpec{Name: "codex"}, RunID: runID, AttemptID: attempt.AttemptID, WorkspacePath: remotePath, WorkspaceBranch: target.Branch, BaseCommit: base, External: external, Generation: external.RuntimeID + ":" + external.Incarnation}
	if err := (executionRecords{root: f.c.RunDir}).saveBinding(ref); err != nil {
		t.Fatal(err)
	}
	s.State.Tasks = append(s.State.Tasks, taskRecord)
	s.State.WorkflowRuns = append(s.State.WorkflowRuns, workflow)
	s.State.Executions = append(s.State.Executions, execution)
	s.State.Attempts = append(s.State.Attempts, attempt)
	s.State.Runs = append(s.State.Runs, run)
	s.State.CurrentTask = ptr(run.TaskID)
	s.State.SlotOccupied = true
	resultPath := filepath.Join(f.c.RunDir, "runs", runID, "attempts", attempt.AttemptID, "result.json")
	operationID := "dispatch-" + attempt.AttemptID
	input := buildMinimalStepInput(f.c.identity, taskValue, run, &workflow, &execution, &attempt, s.State.Attempts...)
	request := mustJSON(map[string]any{"input": input})
	s.State.Attempts[0].OperationID = operationID
	s.State.Attempts[0].ResultPath = resultPath
	s.State.Operations = append(s.State.Operations, OperationRecord{ProjectIdentity: f.c.identity, OperationID: operationID, IdempotencyKey: "step:" + execution.ExecutionID + ":" + attempt.AttemptID, Kind: "dispatch_step", Status: "confirmed", PlatformRequestID: "request-worker-start", PayloadDigest: operationDigest(request), Request: request, CreatedAt: now(), UpdatedAt: now()})
	if err = s.save(); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = NewService(f.c)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.worker == nil || s.active == nil || s.active.RunID != runID || s.active.Process == nil || s.active.Process.Dispatch != "dispatch-test" {
		t.Fatal("restart did not reattach the original supervised Dispatch")
	}
	run = s.active
	if err := s.event(WorkerEvent{RunID: runID, AttemptID: attempt.AttemptID}); err != nil || s.State.Attempts[0].Status != "dispatched" || run.ExecutionStatus != "pending" {
		t.Fatalf("input acceptance prematurely marked the step running: run=%s attempt=%s err=%v", run.ExecutionStatus, s.State.Attempts[0].Status, err)
	}
	if err := s.event(WorkerEvent{RunID: runID, AttemptID: attempt.AttemptID, TurnStarted: true}); err != nil || s.State.Attempts[0].Status != "running" || run.ExecutionStatus != "running" {
		t.Fatalf("same-Dispatch turn evidence did not advance the step from dispatched: run=%s attempt=%s err=%v", run.ExecutionStatus, s.State.Attempts[0].Status, err)
	}
	question := OrcaInboxMessage{ID: "relay-answer-test", Type: "question", FromHandle: "dispatch:dispatch-test", Payload: json.RawMessage(`{"taskId":"task-test","dispatchId":"dispatch-test","question":"Choose a route","options":["left","right"]}`)}
	if err := s.applySupervisedQuestions(run, WorkerEvent{AttemptID: attempt.AttemptID, RemoteRunID: target.OrchestrationRun, RemoteMessages: []OrcaInboxMessage{question}}); err != nil {
		t.Fatal(err)
	}
	answeredAt := now()
	run.Decision.Status = "answer_saved"
	run.Decision.Version++
	run.Decision.OptionID = "left"
	run.Decision.AnsweredBy = "tester"
	run.Decision.AnsweredAt = &answeredAt
	if err := s.syncMinimalDecision(run); err != nil {
		t.Fatal(err)
	}
	if err := s.save(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("STEERLINE_FAKE_SUPERVISED_OBSERVE_UNAVAILABLE", "1")
	if err := s.applyDecisions(context.Background()); err != nil {
		t.Fatal(err)
	}
	if run.Decision.Status != "answer_saved" || run.Error == nil {
		t.Fatal("saved answer was not held while the original Dispatch was unreachable")
	}
	for _, op := range s.State.Operations {
		if op.Kind == "reply_orca_question" {
			t.Fatal("remote reply operation was started before the original Dispatch was observed")
		}
	}
	t.Setenv("STEERLINE_FAKE_SUPERVISED_OBSERVE_UNAVAILABLE", "")
	if err := s.applyDecisions(context.Background()); err != nil {
		t.Fatal(err)
	}
	if run.Decision.Status != "reply_accepted" || run.Decision.AppliedAt != nil || run.AttemptID != attempt.AttemptID || run.Process.Dispatch != "dispatch-test" || s.State.Attempts[0].Status != "waiting_decision" {
		t.Fatal("reply acceptance was mistaken for application or moved to a replacement attempt")
	}
	var replyOperation *OperationRecord
	for i := range s.State.Operations {
		if s.State.Operations[i].Kind == "reply_orca_question" {
			replyOperation = &s.State.Operations[i]
			break
		}
	}
	if replyOperation == nil || replyOperation.Status != "confirmed" || replyOperation.PlatformRequestID != "request-reply" {
		t.Fatalf("reply did not persist its platform receipt: %+v", replyOperation)
	}
	// Model a lost reply receipt after Orca applied the answer. Reconciliation
	// must use the exact inbox thread and must not send another reply mutation.
	replyOperation.Status, replyOperation.PlatformRequestID = "outcome_unknown", ""
	run.Decision.Status = "answer_saved"
	run.Decision.Version--
	if err := s.syncMinimalDecision(run); err != nil {
		t.Fatal(err)
	}
	if err := s.save(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("STEERLINE_FAKE_SUPERVISED_DENY_REPLY", "1")
	if err := s.applyDecisions(context.Background()); err != nil {
		t.Fatal(err)
	}
	for i := range s.State.Operations {
		if s.State.Operations[i].Kind == "reply_orca_question" {
			replyOperation = &s.State.Operations[i]
			break
		}
	}
	if replyOperation.Status != "confirmed" || replyOperation.PlatformRequestID != "" || !strings.Contains(string(replyOperation.Receipt), "answer-message-test") || run.Decision.Status != "reply_accepted" {
		t.Fatalf("uncertain reply did not reconcile from the exact answer message: status=%s request=%q receipt=%s decision=%s", replyOperation.Status, replyOperation.PlatformRequestID, replyOperation.Receipt, run.Decision.Status)
	}
	t.Setenv("STEERLINE_FAKE_SUPERVISED_DENY_REPLY", "")
	proof := OrcaInboxMessage{ID: "relay-applied-test", Type: "status", FromHandle: "dispatch:dispatch-test", Payload: json.RawMessage(`{"taskId":"task-test","dispatchId":"dispatch-test","remote_codex_version":"codex-cli 0.156.1","decision_applied":"relay-answer-test"}`)}
	proofEvent := WorkerEvent{AttemptID: attempt.AttemptID, RemoteRunID: target.OrchestrationRun, RemoteMessages: []OrcaInboxMessage{proof}}
	if err := s.applySupervisedDecisionProof(run, proofEvent); err != nil {
		t.Fatal(err)
	}
	if run.Decision.Status != "applied" || run.Decision.AppliedAt == nil || run.FinishTaskStatus != "pending" || s.State.Attempts[0].Status != "running" {
		t.Fatal("exact decision application proof did not resume the same attempt")
	}
	if err := s.applySupervisedDecisionProof(run, proofEvent); err != nil {
		t.Fatalf("duplicate decision application proof was not idempotent: %v", err)
	}
	s.worker.Close()
	<-s.worker.observationDone
	s.worker, s.active = nil, nil
	run = s.State.Runs[0]
	if err := s.settleSupervisedStepResources(run, attempt.AttemptID, "succeeded"); err != nil {
		t.Fatal(err)
	}
	var release *OperationRecord
	for i := range s.State.Operations {
		if s.State.Operations[i].OperationID == "release-"+attempt.AttemptID {
			release = &s.State.Operations[i]
			break
		}
	}
	if release == nil || release.Status != "confirmed" || release.PlatformRequestID != "request-worker-release" || len(release.Receipt) == 0 {
		t.Fatalf("release platform receipt was not persisted: %+v", release)
	}
}

func TestSupervisedStartMinimalStepBindsRemoteRunAndWorktree(t *testing.T) {
	c := supervisedFixtureConfig(t)
	base := strings.Repeat("a", 40)
	t.Setenv("STEERLINE_FAKE_SUPERVISED_HEAD", base)
	t.Setenv("STEERLINE_FAKE_SUPERVISED_WORKTREE_ID", "repo-test::C:/remote/steerline-run")
	runID, executionID, attemptID := newID(), newID(), newID()
	taskValue := Task{ID: "business-task", Title: "prepare remote workspace", Description: "read-only prepare"}
	definition := DefaultMinimalWorkflowDefinition(false)
	workflow := WorkflowRunRecord{WorkflowRunID: runID, WorkflowVersion: MinimalWorkflowVersion, Definition: definition, Runner: StepRunnerSettings{Model: c.Model, ReasoningEffort: c.ReasoningEffort}}
	execution := ExecutionRecord{ExecutionID: executionID, WorkflowRunID: runID, Status: "active", CurrentStep: "prepare", Target: targetFor(c)}
	attempt := StepAttemptRecord{AttemptID: attemptID, ExecutionID: executionID, StepID: "prepare", Status: "planned", InputRevision: base}
	workspaceIntent := &WorkspaceIntent{BaseCommit: base}
	run := Run{TaskID: "beads-task", DomainTaskID: "business-task", RunID: runID, ExecutionID: executionID, AttemptID: attemptID, WorkflowVersion: MinimalWorkflowVersion, Backend: string(BackendOrca), Protocol: string(ProtocolOrcaSupervised), Agent: "codex", Target: targetFor(c), BaseCommit: &base, WorkspaceIntent: workspaceIntent}
	run.Target.OrchestrationRun = "orca-run-test"
	input := MinimalStepInput{SchemaVersion: 1, ProjectIdentity: c.identity, TaskID: run.DomainTaskID, WorkflowRunID: runID, ExecutionID: executionID, AttemptID: attemptID, StepID: "prepare", InputRevision: base, WorkflowDefinition: definition, Task: taskValue}
	pair, err := probeSupervisedOrcaTarget(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	taskSpec, err := supervisedStepTaskSpec(c, taskValue, run, workflow, attempt, input, pair, base)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(taskSpec, "shell set to pwsh.exe") || !strings.Contains(taskSpec, "FromBase64String") || !strings.Contains(taskSpec, "Do not append --task-id, --dispatch-id") || strings.Contains(taskSpec, "ProcessStartInfo") || strings.Contains(taskSpec, "spawnSync") {
		t.Fatalf("supervised task spec lacks direct PowerShell 7 status sender")
	}
	reviewRevision := strings.Repeat("b", 40)
	reviewAttempt := attempt
	reviewAttempt.StepID = "review"
	reviewInput := input
	reviewInput.StepID = "review"
	reviewInput.CandidateRevision = reviewRevision
	reviewSpec, err := supervisedStepTaskSpec(c, taskValue, run, workflow, reviewAttempt, reviewInput, pair, base)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reviewSpec, "Review the already verified candidate "+reviewRevision) || !strings.Contains(reviewSpec, `"review_verdict":`) || !strings.Contains(reviewSpec, "let the center decide repair or budget exhaustion") || !strings.Contains(reviewSpec, "Use $ids.input_revision") || !strings.Contains(reviewSpec, "$ids.candidate_revision for result.revision") || !strings.Contains(reviewSpec, "Federated Dispatch messages must omit --run and --to") {
		t.Fatal("supervised review task spec lacks a pinned verdict and center-owned repair boundary")
	}
	resultPath := filepath.Join(t.TempDir(), "result.json")
	withoutRun := run
	withoutRun.Target.OrchestrationRun = ""
	if _, err := (orcaSupervisedBackend{config: c}).StartMinimalStep(context.Background(), taskValue, withoutRun, workflow, execution, attempt, input, resultPath); err == nil || !isBackendError(err, BackendRejected) {
		t.Fatalf("unjournaled Run creation was allowed: %v", err)
	}
	receipt, err := (orcaSupervisedBackend{config: c}).StartMinimalStep(context.Background(), taskValue, run, workflow, execution, attempt, input, resultPath)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Disposition != "accepted" || receipt.Ref.Protocol != ProtocolOrcaSupervised || receipt.Ref.External.OrchestrationRun != "orca-run-test" || receipt.Ref.External.Task != "task-test" || receipt.Ref.External.Dispatch != "dispatch-test" || receipt.Ref.External.WorktreeID != "repo-test::C:/remote/steerline-run" {
		t.Fatalf("remote start binding = %+v", receipt)
	}
	if receipt.Run.Target.OrchestrationRun != "orca-run-test" || receipt.Run.Target.WorktreeID != receipt.Ref.External.WorktreeID || receipt.Run.WorktreeBranch == nil || *receipt.Run.WorktreeBranch != "erranliu/steerline-run" || receipt.Run.BaseCommit == nil || *receipt.Run.BaseCommit != base {
		t.Fatalf("remote Run target/workspace = %+v", receipt.Run)
	}
	receipt.Worker.Close()
	<-receipt.Worker.observationDone
}

func TestLoadConfigRequiresExplicitRemoteCancellationFalse(t *testing.T) {
	c := supervisedFixtureConfig(t)
	encoded, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]json.RawMessage
	if err = json.Unmarshal(encoded, &config); err != nil {
		t.Fatal(err)
	}
	var orca map[string]json.RawMessage
	if err = json.Unmarshal(config["orca"], &orca); err != nil {
		t.Fatal(err)
	}
	var supervised map[string]json.RawMessage
	if err = json.Unmarshal(orca["supervised"], &supervised); err != nil {
		t.Fatal(err)
	}
	delete(supervised, "remote_cancel_available")
	orca["supervised"], _ = json.Marshal(supervised)
	config["orca"], _ = json.Marshal(orca)
	withoutPolicy, _ := json.Marshal(config)
	configPath := filepath.Join(t.TempDir(), "steerline.json")
	if err = os.WriteFile(configPath, withoutPolicy, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = LoadConfig(configPath); err == nil || !strings.Contains(err.Error(), "explicitly set to false") {
		t.Fatalf("missing remote cancellation policy error = %v", err)
	}
	supervised["remote_cancel_available"] = json.RawMessage("true")
	orca["supervised"], _ = json.Marshal(supervised)
	config["orca"], _ = json.Marshal(orca)
	withEnabledStop, _ := json.Marshal(config)
	if err = os.WriteFile(configPath, withEnabledStop, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = LoadConfig(configPath); err == nil || !strings.Contains(err.Error(), "fixed unavailable") {
		t.Fatalf("enabled remote cancellation error = %v", err)
	}
}

func TestSupervisedOrcaRealPrepareRoundtrip(t *testing.T) {
	if os.Getenv("STEERLINE_REAL_SUPERVISED_PREPARE_PROBE") != "1" {
		t.Skip("set STEERLINE_REAL_SUPERVISED_PREPARE_PROBE=1 to run the authorized isolated remote prepare probe")
	}
	need := func(name string) string {
		t.Helper()
		value := strings.TrimSpace(os.Getenv(name))
		if value == "" {
			t.Fatalf("%s is required for the real supervised prepare probe", name)
		}
		return value
	}
	base := need("STEERLINE_REAL_SUPERVISED_BASE")
	fullProbe := os.Getenv("STEERLINE_REAL_SUPERVISED_FULL_PROBE") == "1"
	publishProbe := os.Getenv("STEERLINE_REAL_SUPERVISED_PUBLISH_PROBE") == "1" || fullProbe
	if !commitHash.MatchString(base) {
		t.Fatalf("invalid fixed base %q", base)
	}
	root := t.TempDir()
	repo := filepath.Join(root, "local-identity")
	runDir := filepath.Join(root, "runtime")
	if err := os.MkdirAll(repo, 0700); err != nil {
		t.Fatal(err)
	}
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	gitSetup := [][]string{{"init", "--quiet", "-b", "main", repo}, {"-C", repo, "remote", "add", "origin", "https://github.com/erranliu/SteerlineTest.git"}}
	if publishProbe {
		gitSetup = [][]string{{"clone", "--quiet", "--branch", "main", "https://github.com/erranliu/SteerlineTest.git", repo}}
	}
	for _, args := range gitSetup {
		if output, runErr := exec.Command(git, args...).CombinedOutput(); runErr != nil {
			t.Fatalf("git %v: %v: %s", args, runErr, output)
		}
	}
	if publishProbe {
		output, runErr := exec.Command(git, "-C", repo, "rev-parse", "HEAD").CombinedOutput()
		if runErr != nil || strings.TrimSpace(string(output)) != base {
			t.Fatalf("test repository main differs from fixed base: %s %v", output, runErr)
		}
	}
	c := Config{Repository: repo, Remote: "origin", Branch: "main", RunDir: runDir, WorktreeRoot: filepath.Join(runDir, "worktrees"), RuntimeFile: "runtime.json", Claimant: "supervised-probe", Git: Command{Executable: git}, BD: Command{Executable: "bd"}, Codex: Command{Executable: "codex"}, TaskPoll: "2s", PRPoll: "30s", Heartbeat: "5s", CommandTimeout: "30s", ShutdownGrace: "5s", Sandbox: "workspace-write", Agent: "codex", Host: "orca_supervised", Model: "gpt-6-luna", ReasoningEffort: "high", Orca: OrcaConfig{Executable: "orca", Shell: "powershell", WaitTimeout: "15m", Supervised: &SupervisedOrcaConfig{ConnectionID: need("STEERLINE_REAL_SUPERVISED_CONNECTION_ID"), EnvironmentID: need("STEERLINE_REAL_SUPERVISED_ENVIRONMENT_ID"), RepositorySelector: need("STEERLINE_REAL_SUPERVISED_REPOSITORY_SELECTOR"), CoordinatorTerminal: need("STEERLINE_REAL_SUPERVISED_COORDINATOR_TERMINAL"), ExpectedCodexVersion: "codex-cli 0.156.1", PollInterval: "2s", CapabilityReportVersion: 1}}}
	if err = c.ResolveIdentity(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err = validateSupervisedOrcaConfig(&c); err != nil {
		t.Fatal(err)
	}
	reviewProbe := os.Getenv("STEERLINE_REAL_SUPERVISED_REVIEW_BUDGET_PROBE") == "1"
	reviewRequired := reviewProbe || fullProbe
	definition := DefaultMinimalWorkflowDefinition(reviewRequired)
	if reviewProbe {
		definition.MaxRepairRounds = 0
	}
	contract := func(provider, entry string, command []string) StepContract {
		return StepContract{Provider: provider, Entry: entry, Version: MinimalWorkflowVersion, InputSchema: MinimalStepInputSchema, OutputSchema: MinimalStepOutputSchema, ExecutionSite: "target", Timeout: 10 * time.Minute, Command: command}
	}
	steps := ProjectSteps{Implement: contract("agent_skill", "builtin:implement", nil), Verify: contract("command", "verify.ps1", []string{"powershell", "-NoProfile", "-File", "verify.ps1"}), PublishPR: contract("command", "publish.ps1", []string{"powershell", "-NoProfile", "-File", "publish.ps1"})}
	if reviewRequired {
		steps.Review = contract("agent_skill", "builtin:review", nil)
	}
	steps.Verify.RequiredChecks = []string{"prepare-smoke"}
	stagedProbe := os.Getenv("STEERLINE_REAL_SUPERVISED_STAGED_PROBE") == "1" || fullProbe
	negativeVerifyProbe := os.Getenv("STEERLINE_REAL_SUPERVISED_NEGATIVE_VERIFY_PROBE") == "1"
	if publishProbe && !stagedProbe {
		t.Fatal("publish probe requires center-accepted implement and verify stages")
	}
	if negativeVerifyProbe && (!stagedProbe || publishProbe) {
		t.Fatal("negative verify probe requires staged execution without PR publication")
	}
	if reviewProbe && (!stagedProbe || publishProbe || negativeVerifyProbe) {
		t.Fatal("review budget probe requires staged execution without negative verify or PR publication")
	}
	if fullProbe && (reviewProbe || negativeVerifyProbe) {
		t.Fatal("full supervised probe requires successful review before publication")
	}
	stageFile := "steerline-spec03-supervised-stage.txt"
	if fullProbe {
		stageFile = "steerline-spec03-full-stage.txt"
	}
	if stagedProbe {
		verifyScript := fmt.Sprintf("$actual = git show HEAD:%s; if ($LASTEXITCODE -ne 0 -or $actual.Trim() -ne 'spec03-supervised-stage-v1') { exit 7 }; exit 0", stageFile)
		if negativeVerifyProbe {
			verifyScript = "$actual = git show HEAD:steerline-spec03-supervised-stage.txt; if ($LASTEXITCODE -ne 0 -or $actual.Trim() -ne 'deliberately-wrong-content') { exit 7 }; exit 0"
		}
		steps.Verify = contract("command", "spec03-content-smoke-v1", []string{"pwsh.exe", "-NoProfile", "-Command", verifyScript})
		steps.Verify.RequiredChecks = []string{"content-smoke"}
	}
	if publishProbe {
		publishScript := "$ErrorActionPreference='Stop'; $branch=git branch --show-current; $head=git rev-parse HEAD; git push -u origin HEAD; if($LASTEXITCODE -ne 0){exit $LASTEXITCODE}; $existing=@(gh pr list --repo erranliu/SteerlineTest --head $branch --state all --json url,headRefOid,baseRefName | ConvertFrom-Json | Where-Object { $_.headRefOid -eq $head -and $_.baseRefName -eq 'main' }); if($LASTEXITCODE -ne 0){exit $LASTEXITCODE}; if($existing.Count -gt 1){exit 17}; if($existing.Count -eq 1){Write-Output ('EXISTING:'+$existing[0].url); exit 0}; $url=gh pr create --repo erranliu/SteerlineTest --draft --base main --head $branch --title ('Spec 03 acceptance '+$branch) --body 'Isolated SteerLine Spec 03 remote execution acceptance; verified test-only commit.'; if($LASTEXITCODE -ne 0){exit $LASTEXITCODE}; Write-Output ('CREATED:'+($url|Select-Object -Last 1)); exit 0"
		steps.PublishPR = contract("command", "README.md", []string{"pwsh.exe", "-NoProfile", "-Command", publishScript})
		c.PR = PRConfig{Kind: "command", BaseURL: "https://github.com", Repository: "erranliu/SteerlineTest", Query: Command{Executable: "gh", Args: []string{"api", "repos/{repository}/pulls/{number}", "--jq", `{url:.html_url,repository:.base.repo.full_name,base_branch:.base.ref,head_revision:.head.sha,state:(if .merged then "merged" elif .state=="closed" then "closed_unmerged" else .state end),merged:.merged,merged_at:.merged_at,merge_commit:.merge_commit_sha}`}}}
	}
	c.WorkflowDefinition, c.ProjectSteps = &definition, &steps
	service, err := NewService(c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close() })
	runID := newID()
	taskID := "remote-probe-" + runID[:10]
	intent, err := planWorkspace(c, taskID, runID, base)
	if err != nil {
		t.Fatal(err)
	}
	externalRunID := need("STEERLINE_REAL_SUPERVISED_ORCA_RUN_ID")
	run := &Run{TaskID: taskID, DomainTaskID: taskID, Title: "isolated supervised prepare probe", RunID: runID, Claimant: c.Claimant, Host: c.Host, Backend: string(BackendOrca), Protocol: string(ProtocolOrcaSupervised), Agent: c.Agent, Target: targetFor(c), BeadsStatus: "in_progress", ExecutionStatus: "pending", FinishTaskStatus: "pending", PR: PRState{State: "unknown"}, WorkspaceIntent: intent, BaseCommit: ptr(base), ResultPath: ptr(filepath.Join(runDir, "runs", runID, "result.json")), LogPath: ptr(filepath.Join(runDir, "runs", runID, "events.jsonl")), BeadsContext: &BeadsContext{ProjectID: "isolated-protocol-probe"}}
	run.Target.OrchestrationRun = externalRunID
	if existingWorktreeID := strings.TrimSpace(os.Getenv("STEERLINE_REAL_SUPERVISED_PREPARE_WORKTREE_ID")); existingWorktreeID != "" {
		existingPath := need("STEERLINE_REAL_SUPERVISED_PREPARE_WORKTREE_PATH")
		existingBranch := need("STEERLINE_REAL_SUPERVISED_PREPARE_WORKTREE_BRANCH")
		repositoryID := strings.TrimPrefix(c.Orca.Supervised.RepositorySelector, "id:")
		if !strings.HasPrefix(existingWorktreeID, repositoryID+"::") || !strings.HasSuffix(existingWorktreeID, "::"+existingPath) {
			t.Fatalf("existing worktree selector %q does not bind fixed repo/path %q", existingWorktreeID, existingPath)
		}
		run.Target.WorktreeID = existingWorktreeID
		run.Target.Branch = existingBranch
		run.Target.BaseRevision = base
		run.WorktreePath = ptr(existingPath)
		run.WorktreeBranch = ptr(existingBranch)
		run.WorkspaceIntent.Path = existingPath
		run.WorkspaceIntent.Branch = existingBranch
		run.WorkspaceIntent.BaseCommit = base
	}
	askProbe := os.Getenv("STEERLINE_REAL_SUPERVISED_ASK_PROBE") == "1" || fullProbe
	replyLossProbe := os.Getenv("STEERLINE_REAL_SUPERVISED_REPLY_LOSS_PROBE") == "1"
	answerOfflineProbe := os.Getenv("STEERLINE_REAL_SUPERVISED_ANSWER_OFFLINE_PROBE") == "1"
	restartAfterAnswer := os.Getenv("STEERLINE_REAL_SUPERVISED_RESTART_AFTER_ANSWER") == "1"
	if (replyLossProbe || answerOfflineProbe || restartAfterAnswer) && !askProbe {
		t.Fatal("answer fault probes require the original ask/reply path")
	}
	description := "No Beads access. Verify a clean remote worktree and return prepare evidence only."
	if askProbe {
		description += " Before reporting prepare, use Orca ask to request the business decision 'Proceed with read-only prepare?' with exact options yes,no. Wait for the coordinator's answer, report decision_applied for that original question, then finish the same prepare attempt without editing files."
	}
	task := Task{ID: taskID, Title: run.Title, Description: description, Status: "open", Type: "task"}
	source := *run.BeadsContext
	taskRecord, workflow, execution, attempt, err := InitializeMinimalWorkflow(c.identity, c, task, source, run)
	if err != nil {
		t.Fatal(err)
	}
	workflow.Phase = "executing"
	execution.Status = "active"
	run.ExecutionStatus = "pending"
	run.FinishTaskStatus = "pending"
	run.DomainAttemptNumber = attempt.AttemptNumber
	service.State.Tasks = append(service.State.Tasks, taskRecord)
	service.State.WorkflowRuns = append(service.State.WorkflowRuns, workflow)
	service.State.Executions = append(service.State.Executions, execution)
	service.State.Attempts = append(service.State.Attempts, attempt)
	service.State.Runs = append(service.State.Runs, run)
	service.State.CurrentTask = ptr(taskID)
	service.State.SlotOccupied = true
	if err = service.save(); err != nil {
		t.Fatal(err)
	}
	activeAttempt := service.minimalAttempt(run, attempt.AttemptID)
	if err = service.startMinimalStep(context.Background(), task, run, source, activeAttempt); err != nil {
		t.Fatalf("supervised prepare dispatch: %v", err)
	}
	if os.Getenv("STEERLINE_REAL_SUPERVISED_RESTART_AFTER_DISPATCH") == "1" {
		service.worker.Close() // Detach the center observer; leave the remote worker running.
		<-service.worker.observationDone
		if err = service.Close(); err != nil {
			t.Fatal(err)
		}
		service, err = NewService(c)
		if err != nil {
			t.Fatalf("reattach the exact supervised Dispatch after center restart: %v", err)
		}
		if service.worker == nil || service.active == nil || service.active.RunID != runID || service.active.Process == nil || service.active.Process.Dispatch == "" {
			t.Fatal("center restart did not reattach the original supervised Dispatch")
		}
		run = service.active
	}
	deadline := time.NewTimer(15 * time.Minute)
	defer deadline.Stop()
	restartedAfterStatus := false
	for service.worker != nil {
		select {
		case event := <-service.worker.Events:
			if event.RunID != "" {
				if eventErr := service.event(event); eventErr != nil {
					t.Fatalf("persist/ack remote event: %v", eventErr)
				}
				if askProbe && service.active != nil && service.active.Decision != nil && service.active.Decision.Status == "pending" {
					decision := service.active.Decision
					if err = AnswerDecision(c, AnswerOptions{RunID: runID, DecisionID: decision.ID, Option: "yes", By: "spec03-acceptance", ExpectedVersion: decision.Version}); err != nil {
						t.Fatalf("submit answer for exact remote question: %v", err)
					}
					if restartAfterAnswer {
						if err = service.processPendingCommands(context.Background()); err != nil {
							t.Fatalf("persist answer before center restart: %v", err)
						}
						if service.active.Decision.Status != "answer_saved" {
							t.Fatal("center restart injection did not retain the saved answer")
						}
						for _, operation := range service.State.Operations {
							if operation.Kind == "reply_orca_question" {
								t.Fatal("reply was sent before the center restart injection")
							}
						}
						service.worker.Close()
						<-service.worker.observationDone
						if err = service.Close(); err != nil {
							t.Fatal(err)
						}
						service, err = NewService(c)
						if err != nil {
							t.Fatalf("reopen center with original pending decision: %v", err)
						}
						if service.worker == nil || service.active == nil || service.active.Process == nil || service.active.Process.Dispatch != run.Process.Dispatch || service.active.Decision == nil || service.active.Decision.Status != "answer_saved" {
							t.Fatal("center did not reattach the original Dispatch and saved answer")
						}
						run = service.active
					}
					if answerOfflineProbe {
						originalOrca := service.Config.Orca
						service.Config.Orca.Executable = filepath.Join(root, "temporarily-unavailable-orca.exe")
						offlineErr := service.processCommandsAndDecisions(context.Background())
						service.Config.Orca = originalOrca
						if offlineErr != nil || service.active.Decision.Status != "answer_saved" {
							t.Fatalf("saved answer was not held while Orca was unavailable: status=%s err=%v", service.active.Decision.Status, offlineErr)
						}
						for _, operation := range service.State.Operations {
							if operation.Kind == "reply_orca_question" {
								t.Fatal("reply mutation was started during the injected center disconnect")
							}
						}
						if checkpointPath := strings.TrimSpace(os.Getenv("STEERLINE_REAL_SUPERVISED_ANSWER_OFFLINE_EVIDENCE_PATH")); checkpointPath != "" {
							checkpoint := mustJSON(map[string]any{"captured_at": now(), "dispatch_id": service.active.Process.Dispatch, "question_id": service.active.Decision.ID, "decision_status": service.active.Decision.Status, "reply_operation_started": false, "injected_center_orca_unavailable": true})
							if err = os.MkdirAll(filepath.Dir(checkpointPath), 0700); err != nil {
								t.Fatal(err)
							}
							if err = AtomicWrite(checkpointPath, append(checkpoint, '\n')); err != nil {
								t.Fatal(err)
							}
						}
					}
					if replyLossProbe {
						pwshPath, lookupErr := exec.LookPath("pwsh.exe")
						if lookupErr != nil {
							t.Fatal(lookupErr)
						}
						orcaPath := need("STEERLINE_REAL_SUPERVISED_ORCA_EXE")
						wrapperPath := filepath.Join(root, "reply-response-loss.ps1")
						wrapper := `param([string]$OrcaPath, [Parameter(ValueFromRemainingArguments=$true)][string[]]$Rest)
if ($Rest.Count -ge 2 -and $Rest[0] -eq 'orchestration' -and $Rest[1] -eq 'reply') {
  & $OrcaPath @Rest | Out-Null
  if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
  exit 75
}
& $OrcaPath @Rest
exit $LASTEXITCODE
`
						if writeErr := os.WriteFile(wrapperPath, []byte(wrapper), 0600); writeErr != nil {
							t.Fatal(writeErr)
						}
						originalOrca := service.Config.Orca
						service.Config.Orca.Executable = pwshPath
						service.Config.Orca.Args = []string{"-NoProfile", "-File", wrapperPath, orcaPath}
						processErr := service.processCommandsAndDecisions(context.Background())
						service.Config.Orca = originalOrca
						if processErr != nil {
							t.Fatalf("persist uncertain original reply: %v", processErr)
						}
						if service.active.Decision.Status != "answer_saved" {
							t.Fatalf("lost reply response was not held for exact reconciliation: %s", service.active.Decision.Status)
						}
						reconcileDeadline := time.Now().Add(30 * time.Second)
						for service.active.Decision.Status == "answer_saved" && time.Now().Before(reconcileDeadline) {
							if err = service.applyDecisions(context.Background()); err != nil {
								t.Fatalf("read-only reconcile original question: %v", err)
							}
							if service.active.Decision.Status == "answer_saved" {
								time.Sleep(500 * time.Millisecond)
							}
						}
						if service.active.Decision.Status != "reply_accepted" {
							t.Fatal("exact Orca answer was not reconciled after the injected response loss")
						}
						if checkpointPath := strings.TrimSpace(os.Getenv("STEERLINE_REAL_SUPERVISED_REPLY_LOSS_EVIDENCE_PATH")); checkpointPath != "" {
							var replyOperation *OperationRecord
							for i := range service.State.Operations {
								if service.State.Operations[i].Kind == "reply_orca_question" && service.State.Operations[i].IdempotencyKey == "orca-question-reply:"+runID+":"+attempt.AttemptID+":"+service.active.Decision.ID {
									replyOperation = &service.State.Operations[i]
									break
								}
							}
							if replyOperation == nil || replyOperation.Status != "confirmed" || !strings.Contains(string(replyOperation.Receipt), "answer_message_id") {
								t.Fatal("reply loss checkpoint lacks exact read-only answer reconciliation")
							}
							checkpoint := mustJSON(map[string]any{"captured_at": now(), "dispatch_id": service.active.Process.Dispatch, "question_id": service.active.Decision.ID, "decision_status": service.active.Decision.Status, "reply_operation": replyOperation, "injected_reply_response_loss": true})
							if err = os.MkdirAll(filepath.Dir(checkpointPath), 0700); err != nil {
								t.Fatal(err)
							}
							if err = AtomicWrite(checkpointPath, append(checkpoint, '\n')); err != nil {
								t.Fatal(err)
							}
						}
					} else if err = service.processCommandsAndDecisions(context.Background()); err != nil {
						t.Fatalf("send saved answer to original remote Dispatch: %v", err)
					}
				}
				if os.Getenv("STEERLINE_REAL_SUPERVISED_RESTART_AFTER_STATUS") == "1" && !restartedAfterStatus && run.Process != nil {
					currentStatus, currentDone := false, false
					for _, message := range event.RemoteMessages {
						if supervisedMessageMatches(message, *run.Process) {
							currentStatus = currentStatus || message.Type == "status"
							currentDone = currentDone || message.Type == "worker_done"
						}
					}
					if currentStatus && !currentDone {
						service.worker.Close()
						<-service.worker.observationDone
						if err = service.Close(); err != nil {
							t.Fatal(err)
						}
						service, err = NewService(c)
						if err != nil {
							t.Fatalf("reattach after accepted status Delivery: %v", err)
						}
						if service.worker == nil || service.active == nil || service.active.Process == nil || service.active.Process.Dispatch != run.Process.Dispatch {
							t.Fatal("center restart after status did not reattach the same Dispatch")
						}
						run = service.active
						restartedAfterStatus = true
					}
				}
			}
		case workerErr := <-service.worker.Done:
			if finishErr := service.finished(workerErr); finishErr != nil {
				t.Fatalf("finish remote prepare: %v", finishErr)
			}
		case <-deadline.C:
			service.worker.Close() // Detach this observer only; remote cancellation remains unsupported.
			<-service.worker.observationDone
			t.Fatal("supervised prepare probe timed out; exact Dispatch/worktree remain available for manual inspection")
		}
	}
	if os.Getenv("STEERLINE_REAL_SUPERVISED_RESTART_AFTER_STATUS") == "1" && !restartedAfterStatus {
		t.Fatal("status Delivery and worker_done were not separated, so the completion-window restart was not injected")
	}
	if run.ExecutionStatus != "pending" || run.FinishTaskStatus != "pending" || service.executionRecord(run).CurrentStep != "implement" || service.minimalAttempt(run, attempt.AttemptID).Status != "succeeded" {
		t.Fatalf("prepare did not advance to implement: run=%+v execution=%+v attempt=%+v", run, service.executionRecord(run), service.minimalAttempt(run, attempt.AttemptID))
	}
	if service.minimalAttempt(run, attempt.AttemptID).ResultID == "" || service.executionRecord(run).Workspace.Head != base || service.executionRecord(run).Workspace.Dirty {
		t.Fatalf("prepare evidence is incomplete: execution=%+v attempt=%+v", service.executionRecord(run), service.minimalAttempt(run, attempt.AttemptID))
	}
	if askProbe && (run.Decision == nil || run.Decision.Status != "applied" || run.Decision.AttemptID != attempt.AttemptID || run.Decision.AppliedAt == nil) {
		t.Fatalf("remote question was not answered and applied by the original attempt: %+v", run.Decision)
	}
	events, err := service.domainStore.Events(context.Background(), c.identity, execution.ExecutionID, "", 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	seenStatus, seenWorkerDone := false, false
	messageEvidence := make([]map[string]any, 0, len(events))
	for _, event := range events {
		if event.ExternalSource != "orca-run:"+externalRunID {
			continue
		}
		var message OrcaInboxMessage
		_ = json.Unmarshal(event.Payload, &message)
		currentDispatch := message.FromHandle == "dispatch:"+run.Target.Dispatch
		messageEvidence = append(messageEvidence, map[string]any{"message_id": event.ExternalID, "type": event.Type, "attempt_id": event.AttemptID, "dispatch_id": strings.TrimPrefix(message.FromHandle, "dispatch:"), "payload_bytes": event.PayloadBytes, "payload_digest": event.PayloadDigest})
		seenStatus = seenStatus || currentDispatch && event.Type == "orca.status"
		seenWorkerDone = seenWorkerDone || currentDispatch && event.Type == "orca.worker_done"
	}
	if !seenStatus || !seenWorkerDone {
		t.Fatalf("prepare did not persist both status and worker_done from the current exact Dispatch: %+v", messageEvidence)
	}
	operations := make([]OperationRecord, 0, 8)
	startConfirmed, ackConfirmed, replyConfirmed, releaseConfirmed := false, false, false, false
	for _, operation := range service.State.Operations {
		switch operation.Kind {
		case "dispatch_step":
			if operation.IdempotencyKey == "step:"+execution.ExecutionID+":"+attempt.AttemptID {
				startConfirmed = operation.Status == "confirmed" && operation.PlatformRequestID != ""
				operations = append(operations, operation)
			}
		case "ack_orca_delivery":
			ackConfirmed = ackConfirmed || operation.Status == "confirmed" && operation.PlatformRequestID != ""
			operations = append(operations, operation)
		case "reply_orca_question":
			if askProbe && run.Decision != nil && operation.IdempotencyKey == "orca-question-reply:"+run.RunID+":"+attempt.AttemptID+":"+run.Decision.ID {
				replyConfirmed = operation.Status == "confirmed" && (operation.PlatformRequestID != "" || replyLossProbe && strings.Contains(string(operation.Receipt), "answer_message_id"))
				operations = append(operations, operation)
			}
		case "settle_resources":
			if operation.OperationID == "release-"+attempt.AttemptID {
				releaseConfirmed = operation.Status == "confirmed" && operation.PlatformRequestID != ""
				operations = append(operations, operation)
			}
		}
	}
	if !startConfirmed || !ackConfirmed || !releaseConfirmed || askProbe && !replyConfirmed {
		t.Fatalf("prepare external mutations lack exact persisted request receipts: start=%t ack=%t reply=%t release=%t operations=%+v", startConfirmed, ackConfirmed, replyConfirmed, releaseConfirmed, operations)
	}
	binding, err := (executionRecords{root: runDir}).loadBinding(run.RunID, attempt.AttemptID)
	if err != nil {
		t.Fatal(err)
	}
	if evidencePath := strings.TrimSpace(os.Getenv("STEERLINE_REAL_SUPERVISED_EVIDENCE_PATH")); evidencePath != "" {
		pair, pairErr := probeSupervisedOrcaTarget(context.Background(), c)
		if pairErr != nil {
			t.Fatal(pairErr)
		}
		workflowRecord, executionRecord := service.minimalRecords(run)
		attemptResult := service.minimalAttempt(run, attempt.AttemptID)
		evidence := map[string]any{"captured_at": now(), "center_orca_version": pair.CenterVersion, "remote_orca_version": pair.RemoteVersion, "remote_codex_cli_version": c.Orca.Supervised.ExpectedCodexVersion, "run": run, "workflow": workflowRecord, "execution": executionRecord, "step_attempt": attemptResult, "execution_binding": binding, "domain_messages": messageEvidence, "operations": operations, "worker_done_outcome": "succeeded", "resources_released": true, "worktree_retained": true, "files_changed": []string{}, "fault_injection": map[string]bool{"restart_after_dispatch": os.Getenv("STEERLINE_REAL_SUPERVISED_RESTART_AFTER_DISPATCH") == "1", "restart_after_status": os.Getenv("STEERLINE_REAL_SUPERVISED_RESTART_AFTER_STATUS") == "1", "restart_after_answer_saved": restartAfterAnswer, "answer_offline": answerOfflineProbe, "reply_response_lost": replyLossProbe}}
		data, marshalErr := json.MarshalIndent(evidence, "", "  ")
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if err = os.MkdirAll(filepath.Dir(evidencePath), 0700); err != nil {
			t.Fatal(err)
		}
		if err = AtomicWrite(evidencePath, append(data, '\n')); err != nil {
			t.Fatal(err)
		}
	}
	if stagedProbe {
		prepareDispatch := run.Target.Dispatch
		fixedWorktree := run.Target.WorktreeID
		driveStage := func(step string) {
			t.Helper()
			stageDeadline := time.NewTimer(15 * time.Minute)
			defer stageDeadline.Stop()
			for service.worker != nil {
				select {
				case event := <-service.worker.Events:
					if event.RunID != "" {
						if eventErr := service.event(event); eventErr != nil {
							t.Fatalf("%s Delivery persistence/ack: %v", step, eventErr)
						}
					}
				case workerErr := <-service.worker.Done:
					if finishErr := service.finished(workerErr); finishErr != nil && !(reviewProbe && step == "review" && strings.Contains(finishErr.Error(), "repair round limit reached: review requested changes")) {
						t.Fatalf("%s remote result: %v", step, finishErr)
					}
				case <-stageDeadline.C:
					service.worker.Close()
					<-service.worker.observationDone
					t.Fatalf("%s timed out; exact Dispatch remains for inspection", step)
				}
			}
		}
		task.Title = "isolated supervised implement probe"
		task.Description = fmt.Sprintf("Create exactly %s containing spec03-supervised-stage-v1 followed by a newline. Commit that one file on the assigned worktree branch. Do not push, create a PR, edit another file, or run later workflow steps. Return implement result.revision as the actual full commit HEAD and include clean workspace/base ancestry evidence.", stageFile)
		implementAttempt := service.minimalAttempt(run, run.AttemptID)
		if implementAttempt == nil || implementAttempt.StepID != "implement" {
			t.Fatal("prepare did not plan an implement attempt")
		}
		if err = service.startMinimalStep(context.Background(), task, run, source, implementAttempt); err != nil {
			t.Fatalf("start isolated supervised implement: %v", err)
		}
		implementDispatch := run.Target.Dispatch
		if implementDispatch == prepareDispatch || run.Target.WorktreeID != fixedWorktree {
			t.Fatal("implement did not get an independent Dispatch on the prepared worktree")
		}
		driveStage("implement")
		currentExecution := service.executionRecord(run)
		if currentExecution == nil || currentExecution.CurrentStep != "verify" || !commitHash.MatchString(currentExecution.CandidateRevision) || currentExecution.CandidateRevision == base || currentExecution.Workspace.Head != currentExecution.CandidateRevision || currentExecution.Workspace.Dirty {
			t.Fatalf("implement did not produce a clean candidate and center-owned verify step: %+v", currentExecution)
		}
		candidate := currentExecution.CandidateRevision
		task.Title = "isolated supervised verify probe"
		task.Description = "Run the pinned content-smoke command exactly against the current candidate commit. Decode candidate_revision from the task spec base64 input object into $ids.candidate_revision and use that variable for every SHA argument and result field; do not retype its hex digits. Also run git merge-base --is-ancestor <fixed_base> $ids.candidate_revision; include result.base_ancestor=true only after its exit code is zero. This key is mandatory even when content-smoke passes. Read-only verification only: do not edit, commit, push, create a PR, or run a later step. Return one check named content-smoke with the actual command, script_version spec03-content-smoke-v1, observed exit_code, passed/status, evidence_ref git:HEAD:steerline-spec03-supervised-stage.txt, and actual_commit equal to the full HEAD. Return verify result.revision equal to that same candidate commit."
		if fullProbe {
			task.Description = strings.ReplaceAll(task.Description, "steerline-spec03-supervised-stage.txt", stageFile)
		}
		if negativeVerifyProbe {
			task.Description = "Run the pinned content-smoke command exactly. Its expected content is deliberately wrong, so report the actual nonzero exit code, one content-smoke check with status=failed and passed=false, and result.succeeded=false; omit failure_kind. Still report base_ancestor=true only after the actual ancestry command exits zero, and keep the worktree clean at the candidate. Deliver the valid failed-check result via type=status, then worker_done outcome succeeded because verification execution completed. Do not edit, repair, push or publish; only the center may plan the next implement repair attempt."
		}
		verifyAttempt := service.minimalAttempt(run, run.AttemptID)
		if verifyAttempt == nil || verifyAttempt.StepID != "verify" {
			t.Fatal("implement did not plan a verify attempt")
		}
		if err = service.startMinimalStep(context.Background(), task, run, source, verifyAttempt); err != nil {
			t.Fatalf("start isolated supervised verify: %v", err)
		}
		verifyDispatch := run.Target.Dispatch
		if verifyDispatch == implementDispatch || run.Target.WorktreeID != fixedWorktree {
			t.Fatal("verify did not get an independent Dispatch on the same worktree")
		}
		driveStage("verify")
		currentExecution = service.executionRecord(run)
		if negativeVerifyProbe {
			if currentExecution == nil || currentExecution.CurrentStep != "implement" || currentExecution.RepairRounds != 1 || currentExecution.VerifiedRevision != "" || currentExecution.CandidateRevision != candidate || run.FinishTaskStatus != "pending" || run.Target.WorktreeID != fixedWorktree {
				t.Fatalf("failed required check did not schedule one bounded repair attempt: %+v run=%+v", currentExecution, run)
			}
			repairAttempt := service.minimalAttempt(run, run.AttemptID)
			if repairAttempt == nil || repairAttempt.StepID != "implement" || repairAttempt.Status != "planned" {
				t.Fatalf("center did not create a planned implement repair attempt: %+v", repairAttempt)
			}
			if outputPath := strings.TrimSpace(os.Getenv("STEERLINE_REAL_SUPERVISED_NEGATIVE_VERIFY_EVIDENCE_PATH")); outputPath != "" {
				workflowRecord, executionRecord := service.minimalRecords(run)
				negativeEvidence := map[string]any{"captured_at": now(), "run": run, "workflow": workflowRecord, "execution": executionRecord, "attempts": service.State.Attempts, "operations": service.State.Operations, "dispatches": map[string]string{"prepare": prepareDispatch, "implement": implementDispatch, "verify_failed_check": verifyDispatch}, "candidate_revision": candidate, "repair_planned": true, "pr_created": false}
				data, marshalErr := json.MarshalIndent(negativeEvidence, "", "  ")
				if marshalErr != nil {
					t.Fatal(marshalErr)
				}
				if err = os.MkdirAll(filepath.Dir(outputPath), 0700); err != nil {
					t.Fatal(err)
				}
				if err = AtomicWrite(outputPath, append(data, '\n')); err != nil {
					t.Fatal(err)
				}
			}
			t.Logf("failed content-smoke scheduled repair round %d on candidate %s", currentExecution.RepairRounds, candidate)
			return
		}
		reviewDispatch := ""
		if reviewProbe {
			if currentExecution == nil || currentExecution.CurrentStep != "review" || currentExecution.VerifiedRevision != candidate || run.Target.WorktreeID != fixedWorktree {
				t.Fatalf("verified candidate did not advance to center-owned review: %+v", currentExecution)
			}
			task.Title = "isolated supervised review budget probe"
			task.Description = "Review the current candidate read-only against this fixed review policy: a new file named steerline-spec03-supervised-stage.txt requires changes. Inspect the actual commit diff and verify that this file was added. If so, report review_verdict=changes_requested, result.succeeded=true, and result.revision equal to the verified candidate. Do not edit, commit, push, create a PR or start a repair yourself. The center has a zero-round repair budget and must block this workflow after accepting your review result."
			reviewAttempt := service.minimalAttempt(run, run.AttemptID)
			if reviewAttempt == nil || reviewAttempt.StepID != "review" || reviewAttempt.Status != "planned" {
				t.Fatalf("verify did not plan a review attempt: %+v", reviewAttempt)
			}
			if err = service.startMinimalStep(context.Background(), task, run, source, reviewAttempt); err != nil {
				t.Fatalf("start isolated supervised review: %v", err)
			}
			reviewDispatch := run.Target.Dispatch
			if reviewDispatch == verifyDispatch || run.Target.WorktreeID != fixedWorktree {
				t.Fatal("review did not get an independent Dispatch on the verified worktree")
			}
			driveStage("review")
			workflowRecord, executionRecord := service.minimalRecords(run)
			settledReview := service.minimalAttempt(run, reviewAttempt.AttemptID)
			if settledReview == nil || len(settledReview.Result) == 0 {
				t.Fatalf("center did not retain the review result on its exact Attempt: %+v", settledReview)
			}
			var reviewResult StepGateResult
			if err = json.Unmarshal(settledReview.Result, &reviewResult); err != nil {
				t.Fatalf("decode accepted review receipt: %v", err)
			}
			if workflowRecord == nil || executionRecord == nil || workflowRecord.Phase != "blocked" || executionRecord.RepairRounds != 0 || executionRecord.CandidateRevision != candidate || executionRecord.VerifiedRevision != candidate || run.FinishTaskStatus != "blocked" || run.PR.URL != nil || run.Error == nil || !strings.Contains(*run.Error, "repair round limit reached: review requested changes") || reviewResult.ReviewVerdict != "changes_requested" || reviewResult.Revision != candidate || reviewResult.ResultID == "" {
				t.Fatalf("review-requested changes did not exhaust the zero repair budget without publication: workflow=%+v execution=%+v run=%+v", workflowRecord, executionRecord, run)
			}
			if outputPath := strings.TrimSpace(os.Getenv("STEERLINE_REAL_SUPERVISED_REVIEW_BUDGET_EVIDENCE_PATH")); outputPath != "" {
				output := map[string]any{"captured_at": now(), "run": run, "workflow": workflowRecord, "execution": executionRecord, "attempts": service.State.Attempts, "operations": service.State.Operations, "dispatches": map[string]string{"prepare": prepareDispatch, "implement": implementDispatch, "verify": verifyDispatch, "review_changes_requested": reviewDispatch}, "candidate_revision": candidate, "review_budget_exhausted": true, "pr_created": false}
				data, marshalErr := json.MarshalIndent(output, "", "  ")
				if marshalErr != nil {
					t.Fatal(marshalErr)
				}
				if err = os.MkdirAll(filepath.Dir(outputPath), 0700); err != nil {
					t.Fatal(err)
				}
				if err = AtomicWrite(outputPath, append(data, '\n')); err != nil {
					t.Fatal(err)
				}
			}
			t.Logf("review changes request exhausted the center repair budget on candidate %s", candidate)
			return
		}
		if fullProbe {
			if currentExecution == nil || currentExecution.CurrentStep != "review" || currentExecution.VerifiedRevision != candidate || run.Target.WorktreeID != fixedWorktree {
				t.Fatalf("verified candidate did not plan the full-path review: %+v", currentExecution)
			}
			task.Title = "isolated supervised passing review probe"
			task.Description = fmt.Sprintf("Review the actual diff at the verified candidate read-only. The acceptance policy is exactly one new file named %s containing spec03-supervised-stage-v1 followed by a newline, with no other changes. Verify the clean HEAD and fixed-base ancestry. If those facts hold, return review_verdict=passed, result.succeeded=true, result.revision equal to the exact verified candidate, and the same prerequisite result IDs. Do not edit, commit, push, create a PR, or run publication yourself.", stageFile)
			reviewAttempt := service.minimalAttempt(run, run.AttemptID)
			if reviewAttempt == nil || reviewAttempt.StepID != "review" || reviewAttempt.Status != "planned" {
				t.Fatalf("verify did not plan full-path review: %+v", reviewAttempt)
			}
			if err = service.startMinimalStep(context.Background(), task, run, source, reviewAttempt); err != nil {
				t.Fatalf("start full-path supervised review: %v", err)
			}
			reviewDispatch = run.Target.Dispatch
			if reviewDispatch == verifyDispatch || run.Target.WorktreeID != fixedWorktree {
				t.Fatal("full-path review did not get an independent Dispatch on the verified worktree")
			}
			driveStage("review")
			currentExecution = service.executionRecord(run)
			if currentExecution == nil || currentExecution.CurrentStep != "publish_pr" || currentExecution.ReviewedRevision != candidate || currentExecution.VerifiedRevision != candidate || run.Target.WorktreeID != fixedWorktree {
				t.Fatalf("passing review did not preserve verified revision and plan publication: %+v", currentExecution)
			}
		}
		if currentExecution == nil || currentExecution.CurrentStep != "publish_pr" || currentExecution.CandidateRevision != candidate || currentExecution.VerifiedRevision != candidate || currentExecution.Workspace.Head != candidate || currentExecution.Workspace.Dirty || run.Target.WorktreeID != fixedWorktree {
			t.Fatalf("verify gate did not pin the candidate before publication: %+v", currentExecution)
		}
		if stagedEvidencePath := strings.TrimSpace(os.Getenv("STEERLINE_REAL_SUPERVISED_STAGED_EVIDENCE_PATH")); stagedEvidencePath != "" {
			workflowRecord, executionRecord := service.minimalRecords(run)
			dispatches := map[string]string{"prepare": prepareDispatch, "implement": implementDispatch, "verify": verifyDispatch}
			if fullProbe {
				dispatches["review"] = reviewDispatch
			}
			stagedEvidence := map[string]any{"captured_at": now(), "run": run, "workflow": workflowRecord, "execution": executionRecord, "attempts": service.State.Attempts, "operations": service.State.Operations, "dispatches": dispatches, "worktree_id": fixedWorktree, "fixed_base": base, "candidate_revision": candidate, "verified_revision": currentExecution.VerifiedRevision, "pr_created": false, "worktree_retained": true}
			data, marshalErr := json.MarshalIndent(stagedEvidence, "", "  ")
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			if err = os.MkdirAll(filepath.Dir(stagedEvidencePath), 0700); err != nil {
				t.Fatal(err)
			}
			if err = AtomicWrite(stagedEvidencePath, append(data, '\n')); err != nil {
				t.Fatal(err)
			}
		}
		t.Logf("supervised implement/verify accepted on worktree=%s candidate=%s", fixedWorktree, candidate)
		if publishProbe {
			task.Title = "isolated supervised draft PR probe"
			task.Description = "Publish only the already verified candidate commit from this exact worktree branch to erranliu/SteerlineTest main. First verify clean HEAD and fixed-base ancestry. Push this branch, then create one draft GitHub PR if no exact PR exists; if a unique PR already matches the fixed source branch, head and base, reconcile that URL without creating another. Do not merge or close it. Return the exact created or reconciled PR URL, revision and published_head equal to the candidate commit, the same prerequisite result IDs, clean workspace evidence, and base_ancestor=true in the strict publish_pr step_result. Do not edit or commit files in this step."
			publishAttempt := service.minimalAttempt(run, run.AttemptID)
			if publishAttempt == nil || publishAttempt.StepID != "publish_pr" {
				t.Fatal("verify did not plan a publish_pr attempt")
			}
			if err = service.startMinimalStep(context.Background(), task, run, source, publishAttempt); err != nil {
				t.Fatalf("start isolated supervised publish_pr: %v", err)
			}
			publishDispatch := run.Target.Dispatch
			if publishDispatch == verifyDispatch || run.Target.WorktreeID != fixedWorktree {
				t.Fatal("publish_pr did not get an independent Dispatch on the verified worktree")
			}
			driveStage("publish_pr")
			workflowRecord, executionRecord := service.minimalRecords(run)
			if run.FinishTaskStatus != "waiting_pr_merge" || run.PR.URL == nil || executionRecord == nil || executionRecord.CandidateRevision != candidate || executionRecord.VerifiedRevision != candidate || workflowRecord == nil || workflowRecord.SelectedResult != executionRecord.ExecutionID {
				t.Fatalf("publish_pr did not bind a waiting PR to the verified candidate: run=%+v execution=%+v", run, executionRecord)
			}
			prInfo, queryErr := service.prs.Query(context.Background(), *run.PR.URL)
			if queryErr != nil || prInfo.HeadRevision != candidate || prInfo.BaseBranch != "main" || prInfo.Repository != "erranliu/SteerlineTest" || prInfo.Merged {
				t.Fatalf("published test PR identity is not verified: %+v %v", prInfo, queryErr)
			}
			if publishEvidencePath := strings.TrimSpace(os.Getenv("STEERLINE_REAL_SUPERVISED_PUBLISH_EVIDENCE_PATH")); publishEvidencePath != "" {
				dispatches := map[string]string{"prepare": prepareDispatch, "implement": implementDispatch, "verify": verifyDispatch, "publish_pr": publishDispatch}
				if fullProbe {
					dispatches["review"] = reviewDispatch
				}
				publishEvidence := map[string]any{"captured_at": now(), "run": run, "workflow": workflowRecord, "execution": executionRecord, "attempts": service.State.Attempts, "operations": service.State.Operations, "dispatches": dispatches, "worktree_id": fixedWorktree, "candidate_revision": candidate, "pr": prInfo, "waiting_pr_merge": true, "merge_performed": false}
				data, marshalErr := json.MarshalIndent(publishEvidence, "", "  ")
				if marshalErr != nil {
					t.Fatal(marshalErr)
				}
				if err = os.MkdirAll(filepath.Dir(publishEvidencePath), 0700); err != nil {
					t.Fatal(err)
				}
				if err = AtomicWrite(publishEvidencePath, append(data, '\n')); err != nil {
					t.Fatal(err)
				}
			}
			t.Logf("supervised draft PR accepted: %s head=%s dispatch=%s", *run.PR.URL, candidate, publishDispatch)
		}
	}
	t.Logf("supervised prepare accepted: run=%s task=%s dispatch=%s worktree=%s", run.Target.OrchestrationRun, run.Target.ExternalTask, run.Target.Dispatch, run.Target.WorktreeID)
}

func TestSupervisedOrcaRealAnsweredQuestionHistory(t *testing.T) {
	path := strings.TrimSpace(os.Getenv("STEERLINE_REAL_SUPERVISED_ANSWER_EVIDENCE"))
	if path == "" {
		t.Skip("set STEERLINE_REAL_SUPERVISED_ANSWER_EVIDENCE to inspect an authorized answered question")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var evidence struct {
		Run              Run          `json:"run"`
		ExecutionBinding ExecutionRef `json:"execution_binding"`
	}
	if err := json.Unmarshal(data, &evidence); err != nil {
		t.Fatal(err)
	}
	if evidence.Run.Decision == nil || evidence.Run.Decision.Status != "applied" {
		t.Fatal("evidence does not contain an applied remote question")
	}
	answer := evidence.Run.Decision.OptionID
	if answer == "" {
		answer = evidence.Run.Decision.Note
	}
	c := Config{Host: "orca_supervised", Orca: OrcaConfig{Executable: "orca", Supervised: &SupervisedOrcaConfig{ConnectionID: evidence.ExecutionBinding.Target.ConnectionID, EnvironmentID: evidence.ExecutionBinding.Target.ExternalRuntime}}}
	messageID, err := (orcaSupervisedBackend{config: c}).findAnsweredQuestion(context.Background(), evidence.ExecutionBinding, evidence.Run.Decision.ID, answer)
	if err != nil || messageID == "" {
		t.Fatalf("exact answered question is absent from real read-only Orca history: %q %v", messageID, err)
	}
	t.Logf("exact question %s has answer message %s", evidence.Run.Decision.ID, messageID)
}

func TestSupervisedOrcaRealPublishExistingDraft(t *testing.T) {
	evidencePath := strings.TrimSpace(os.Getenv("STEERLINE_REAL_SUPERVISED_PUBLISH_RECONCILE_SOURCE"))
	if evidencePath == "" {
		t.Skip("set STEERLINE_REAL_SUPERVISED_PUBLISH_RECONCILE_SOURCE to recover an authorized verified draft PR")
	}
	data, err := os.ReadFile(evidencePath)
	if err != nil {
		t.Fatal(err)
	}
	var sourceEvidence struct {
		Run        Run                 `json:"run"`
		Workflow   WorkflowRunRecord   `json:"workflow"`
		Execution  ExecutionRecord     `json:"execution"`
		Attempts   []StepAttemptRecord `json:"attempts"`
		Operations []OperationRecord   `json:"operations"`
		WorktreeID string              `json:"worktree_id"`
		Candidate  string              `json:"candidate_revision"`
		Verified   string              `json:"verified_revision"`
	}
	if err := json.Unmarshal(data, &sourceEvidence); err != nil {
		t.Fatal(err)
	}
	run := &sourceEvidence.Run
	if run.FinishTaskStatus != "pending" || sourceEvidence.Execution.CurrentStep != "publish_pr" || sourceEvidence.Candidate != sourceEvidence.Verified || sourceEvidence.Candidate != sourceEvidence.Execution.CandidateRevision || sourceEvidence.Candidate != sourceEvidence.Execution.VerifiedRevision || sourceEvidence.WorktreeID != run.Target.WorktreeID || run.ResultPath == nil {
		t.Fatal("source evidence is not a center-verified publish_pr plan")
	}
	root := t.TempDir()
	repoAbs := filepath.Join(root, "local-identity")
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	if output, cloneErr := exec.Command(git, "clone", "--quiet", "--branch", "main", "https://github.com/erranliu/SteerlineTest.git", repoAbs).CombinedOutput(); cloneErr != nil {
		t.Fatalf("clone isolated test repository for publication reconciliation: %v %s", cloneErr, output)
	}
	runDir := filepath.Join(root, "runtime")
	c := Config{Repository: repoAbs, Remote: "origin", Branch: "main", RunDir: runDir, WorktreeRoot: filepath.Join(runDir, "worktrees"), RuntimeFile: "runtime.json", Claimant: run.Claimant, Git: Command{Executable: git}, BD: Command{Executable: "bd"}, Codex: sourceEvidence.Workflow.Runner.Codex, TaskPoll: "2s", PRPoll: "30s", Heartbeat: "5s", CommandTimeout: "30s", ShutdownGrace: "5s", Sandbox: sourceEvidence.Workflow.Runner.Sandbox, Agent: "codex", Host: "orca_supervised", Model: sourceEvidence.Workflow.Runner.Model, ReasoningEffort: sourceEvidence.Workflow.Runner.ReasoningEffort, Orca: OrcaConfig{Executable: "orca", Shell: "powershell", WaitTimeout: "15m", Supervised: &SupervisedOrcaConfig{ConnectionID: run.Target.ConnectionID, EnvironmentID: run.Target.ExternalRuntime, RepositorySelector: "id:09e35751-6517-4e7c-ae20-178ed0e90000", CoordinatorTerminal: "term_5253eb1e-0e83-409e-b1df-5c4aef4bea74", ExpectedCodexVersion: "codex-cli 0.156.1", PollInterval: "2s", CapabilityReportVersion: 1}}, PR: PRConfig{Kind: "command", BaseURL: "https://github.com", Repository: "erranliu/SteerlineTest", Query: Command{Executable: "gh", Args: []string{"api", "repos/{repository}/pulls/{number}", "--jq", `{url:.html_url,repository:.base.repo.full_name,base_branch:.base.ref,head_revision:.head.sha,state:(if .merged then "merged" elif .state=="closed" then "closed_unmerged" else .state end),merged:.merged,merged_at:.merged_at,merge_commit:.merge_commit_sha}`}}}}
	c.WorkflowDefinition, c.ProjectSteps = &sourceEvidence.Workflow.Definition, &sourceEvidence.Workflow.ProjectSteps
	if err := c.ResolveIdentity(context.Background()); err != nil {
		t.Fatal(err)
	}
	if c.identity == sourceEvidence.Workflow.ProjectIdentity {
		t.Fatal("publication reconciliation fixture unexpectedly reused the original deleted local identity")
	}
	run.ResultPath = ptr(filepath.Join(runDir, "runs", run.RunID, "result.json"))
	run.LogPath = ptr(filepath.Join(runDir, "runs", run.RunID, "events.jsonl"))
	run.StepResultPath = nil
	sourceEvidence.Workflow.ProjectIdentity = c.identity
	sourceEvidence.Workflow.Repository = repoAbs
	sourceEvidence.Execution.ProjectIdentity = c.identity
	sourceEvidence.Execution.Workspace.Repository = c.identity
	for i := range sourceEvidence.Attempts {
		attempt := &sourceEvidence.Attempts[i]
		attempt.ProjectIdentity = c.identity
		attempt.OperationID = ""
		attempt.EventPath = filepath.Join(runDir, "runs", run.RunID, "steps", attempt.AttemptID, "events.jsonl")
		attempt.ResultPath = filepath.Join(runDir, "runs", run.RunID, "steps", attempt.AttemptID, "result.json")
		if len(attempt.Result) > 0 {
			var result StepGateResult
			if err := json.Unmarshal(attempt.Result, &result); err != nil {
				t.Fatal(err)
			}
			result.Workspace.Repository = c.identity
			attempt.Result = mustJSON(result)
		}
	}
	service, err := NewService(c)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	var verifyAttempt *StepAttemptRecord
	for i := range sourceEvidence.Attempts {
		if sourceEvidence.Attempts[i].StepID == "verify" && sourceEvidence.Attempts[i].Status == "succeeded" {
			verifyAttempt = &sourceEvidence.Attempts[i]
			break
		}
	}
	if verifyAttempt == nil || verifyAttempt.ExternalBinding.Dispatch == "" {
		t.Fatal("verified source attempt lacks its exact Dispatch binding")
	}
	backend := orcaSupervisedBackend{config: c}
	pair, err := probeSupervisedOrcaTarget(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	prior, err := backend.showWorker(context.Background(), supervisedStartReceipt{RunID: run.Target.OrchestrationRun, TaskID: verifyAttempt.ExternalBinding.ExternalTask, DispatchID: verifyAttempt.ExternalBinding.Dispatch, WorktreeID: sourceEvidence.WorktreeID, CenterRuntimeID: pair.CenterRuntime})
	if err != nil {
		t.Fatal(err)
	}
	ref := ExecutionRef{Version: ExecutionRecordVersion, Backend: BackendOrca, Protocol: ProtocolOrcaSupervised, Target: verifyAttempt.ExternalBinding, Agent: AgentSpec{Name: "codex"}, RunID: run.RunID, AttemptID: verifyAttempt.AttemptID, WorkspacePath: deref(run.WorktreePath), WorkspaceBranch: deref(run.WorktreeBranch), BaseCommit: deref(run.BaseCommit), External: prior.Identity, Generation: prior.Identity.RuntimeID + ":" + prior.Identity.Incarnation}
	if err := (executionRecords{root: runDir}).saveBinding(ref); err != nil {
		t.Fatal(err)
	}
	source := *run.BeadsContext
	taskRecord := TaskRecord{ProjectIdentity: c.identity, TaskID: run.DomainTaskID, ExternalProvider: "beads", ExternalProject: source.ProjectID, ExternalID: run.TaskID, Title: run.Title, Claimant: run.Claimant, Source: "beads", Status: "open", Version: 1, CreatedAt: now(), UpdatedAt: now()}
	service.State.Tasks = append(service.State.Tasks, taskRecord)
	service.State.WorkflowRuns = append(service.State.WorkflowRuns, sourceEvidence.Workflow)
	service.State.Executions = append(service.State.Executions, sourceEvidence.Execution)
	service.State.Attempts = append(service.State.Attempts, sourceEvidence.Attempts...)
	service.State.Runs = append(service.State.Runs, run)
	service.State.CurrentTask, service.State.SlotOccupied = ptr(run.TaskID), true
	if err := service.save(); err != nil {
		t.Fatal(err)
	}
	prURL := "https://github.com/erranliu/SteerlineTest/pull/4"
	prInfo, err := service.prs.Query(context.Background(), prURL)
	if err != nil || prInfo.HeadRevision != sourceEvidence.Candidate || prInfo.BaseBranch != "main" || prInfo.Merged {
		t.Fatalf("existing draft PR cannot be uniquely reconciled to the verified candidate: %+v %v", prInfo, err)
	}
	task := Task{ID: run.DomainTaskID, Title: "reconcile original supervised draft PR", Description: "The prior publish Dispatch created draft PR #4 but its worker_done reported failure after an ambiguous command output. Use the pinned publish command once; if it reports the unique existing PR for this exact source branch, candidate head and main base, return that exact URL as reconciliation. Do not create another PR, edit files, merge or close the task. Return a strict publish_pr step_result with revision and published_head equal to the verified candidate, clean workspace, exact prerequisite result IDs and base_ancestor=true.", Status: "open", Type: "task"}
	attempt := service.minimalAttempt(run, run.AttemptID)
	if attempt == nil || attempt.StepID != "publish_pr" || attempt.Status != "planned" {
		t.Fatal("saved publish_pr attempt is no longer planned")
	}
	if err := service.startMinimalStep(context.Background(), task, run, source, attempt); err != nil {
		t.Fatalf("start exact PR publication reconciliation: %v", err)
	}
	deadline := time.NewTimer(15 * time.Minute)
	defer deadline.Stop()
	for service.worker != nil {
		select {
		case event := <-service.worker.Events:
			if event.RunID != "" {
				if eventErr := service.event(event); eventErr != nil {
					t.Fatalf("persist PR reconciliation Delivery: %v", eventErr)
				}
			}
		case workerErr := <-service.worker.Done:
			if finishErr := service.finished(workerErr); finishErr != nil {
				t.Fatalf("reconciled publish_pr result: %v", finishErr)
			}
		case <-deadline.C:
			service.worker.Close()
			<-service.worker.observationDone
			t.Fatal("publish_pr reconciliation timed out; exact Dispatch remains for inspection")
		}
	}
	workflow, execution := service.minimalRecords(run)
	if run.FinishTaskStatus != "waiting_pr_merge" || run.PR.URL == nil || *run.PR.URL != prURL || workflow == nil || execution == nil || workflow.SelectedResult != execution.ExecutionID || execution.VerifiedRevision != sourceEvidence.Candidate || execution.CandidateRevision != sourceEvidence.Candidate {
		t.Fatalf("existing exact PR was not bound to the selected verified execution: run=%+v execution=%+v", run, execution)
	}
	if outputPath := strings.TrimSpace(os.Getenv("STEERLINE_REAL_SUPERVISED_PUBLISH_RECONCILE_OUTPUT")); outputPath != "" {
		output := map[string]any{"captured_at": now(), "run": run, "workflow": workflow, "execution": execution, "attempts": service.State.Attempts, "operations": service.State.Operations, "pr": prInfo, "reconciled_existing_pr": true, "second_pr_created": false, "reconstructed_verified_fixture": true}
		encoded, marshalErr := json.MarshalIndent(output, "", "  ")
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if err := os.MkdirAll(filepath.Dir(outputPath), 0700); err != nil {
			t.Fatal(err)
		}
		if err := AtomicWrite(outputPath, append(encoded, '\n')); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("original draft PR reconciled by center: %s head=%s", prURL, sourceEvidence.Candidate)
}

func TestSupervisedOrcaRealMergedPRQuery(t *testing.T) {
	if os.Getenv("STEERLINE_REAL_SUPERVISED_MERGED_PR_QUERY") != "1" {
		t.Skip("set STEERLINE_REAL_SUPERVISED_MERGED_PR_QUERY=1 after the isolated test PR is actually merged")
	}
	client := PRClient{Config: PRConfig{Kind: "command", BaseURL: "https://github.com", Repository: "erranliu/SteerlineTest", Query: Command{Executable: "gh", Args: []string{"api", "repos/{repository}/pulls/{number}", "--jq", `{url:.html_url,repository:.base.repo.full_name,base_branch:.base.ref,head_revision:.head.sha,state:(if .merged then "merged" elif .state=="closed" then "closed_unmerged" else .state end),merged:.merged,merged_at:.merged_at,merge_commit:.merge_commit_sha}`}}}, Runner: Runner{Timeout: 30 * time.Second}}
	info, err := client.Query(context.Background(), "https://github.com/erranliu/SteerlineTest/pull/4")
	if err != nil {
		t.Fatal(err)
	}
	if info.State != "merged" || !info.Merged || info.MergedAt == nil || info.MergeCommit == nil || *info.MergeCommit != "fbf236e85f0fbeadc38a6e58d424bbf51c3330f0" || info.HeadRevision != "344614147480d0fcf591cf2882210bb7785c6f87" || info.BaseBranch != "main" {
		t.Fatalf("center PR adapter did not read the exact merged test PR: %+v", info)
	}
	t.Logf("merged PR evidence: head=%s merge=%s at=%s", info.HeadRevision, *info.MergeCommit, info.MergedAt.UTC().Format(time.RFC3339))
}

func TestSupervisedOrcaRealMergedTaskClose(t *testing.T) {
	path := strings.TrimSpace(os.Getenv("STEERLINE_REAL_SUPERVISED_MERGED_CLOSE_SOURCE"))
	if path == "" {
		t.Skip("set STEERLINE_REAL_SUPERVISED_MERGED_CLOSE_SOURCE to exercise isolated Beads close after the actual test PR merge")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var evidence struct {
		Run       Run                 `json:"run"`
		Workflow  WorkflowRunRecord   `json:"workflow"`
		Execution ExecutionRecord     `json:"execution"`
		Attempts  []StepAttemptRecord `json:"attempts"`
		PR        PRInfo              `json:"pr"`
	}
	if err := json.Unmarshal(data, &evidence); err != nil {
		t.Fatal(err)
	}
	run := &evidence.Run
	if run.FinishTaskStatus != "waiting_pr_merge" || run.PR.URL == nil || *run.PR.URL != evidence.PR.URL || evidence.Workflow.SelectedResult != evidence.Execution.ExecutionID || evidence.Execution.CandidateRevision != evidence.Execution.VerifiedRevision {
		t.Fatal("source evidence is not an exact selected execution awaiting the test PR merge")
	}
	root := t.TempDir()
	repo := filepath.Join(root, "provider-project")
	runDir := filepath.Join(root, "runtime")
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "--quiet", "-b", "main", repo}, {"-C", repo, "remote", "add", "origin", "https://github.com/erranliu/SteerlineTest.git"}} {
		if output, runErr := exec.Command(git, args...).CombinedOutput(); runErr != nil {
			t.Fatalf("git %v: %v %s", args, runErr, output)
		}
	}
	bd, err := exec.LookPath("bd")
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "--prefix", "remote-probe", "--quiet"}, {"create", "--id", run.TaskID, "--title", "Spec 03 merged completion probe", "--type", "task", "--json"}, {"update", run.TaskID, "--assignee", run.Claimant, "--status", "in_progress", "--json"}} {
		cmd := exec.Command(bd, args...)
		cmd.Dir = repo
		if output, runErr := cmd.CombinedOutput(); runErr != nil {
			t.Fatalf("bd %v: %v %s", args, runErr, output)
		}
	}
	c := Config{Repository: repo, Remote: "origin", Branch: "main", RunDir: runDir, WorktreeRoot: filepath.Join(runDir, "worktrees"), RuntimeFile: "runtime.json", Claimant: run.Claimant, Git: Command{Executable: git}, BD: Command{Executable: bd}, Codex: Command{Executable: "codex"}, TaskPoll: "2s", PRPoll: "30s", Heartbeat: "5s", CommandTimeout: "30s", ShutdownGrace: "5s", Sandbox: "workspace-write", Agent: "codex", Host: "orca_supervised", Model: "gpt-6-luna", ReasoningEffort: "high", Orca: OrcaConfig{Executable: "orca", Shell: "powershell", WaitTimeout: "15m", Supervised: &SupervisedOrcaConfig{ConnectionID: run.Target.ConnectionID, EnvironmentID: run.Target.ExternalRuntime, RepositorySelector: "id:09e35751-6517-4e7c-ae20-178ed0e90000", CoordinatorTerminal: "term_5253eb1e-0e83-409e-b1df-5c4aef4bea74", ExpectedCodexVersion: "codex-cli 0.156.1", PollInterval: "2s", CapabilityReportVersion: 1}}, PR: PRConfig{Kind: "command", BaseURL: "https://github.com", Repository: "erranliu/SteerlineTest", Query: Command{Executable: "gh", Args: []string{"api", "repos/{repository}/pulls/{number}", "--jq", `{url:.html_url,repository:.base.repo.full_name,base_branch:.base.ref,head_revision:.head.sha,state:(if .merged then "merged" elif .state=="closed" then "closed_unmerged" else .state end),merged:.merged,merged_at:.merged_at,merge_commit:.merge_commit_sha}`}}}}
	if err := c.ResolveIdentity(context.Background()); err != nil {
		t.Fatal(err)
	}
	run.ResultPath = ptr(filepath.Join(runDir, "runs", run.RunID, "result.json"))
	run.LogPath = ptr(filepath.Join(runDir, "runs", run.RunID, "events.jsonl"))
	run.StepResultPath = nil
	evidence.Workflow.ProjectIdentity, evidence.Workflow.Repository = c.identity, repo
	evidence.Execution.ProjectIdentity, evidence.Execution.Workspace.Repository = c.identity, c.identity
	for i := range evidence.Attempts {
		attempt := &evidence.Attempts[i]
		attempt.ProjectIdentity, attempt.OperationID = c.identity, ""
		attempt.EventPath = filepath.Join(runDir, "runs", run.RunID, "steps", attempt.AttemptID, "events.jsonl")
		attempt.ResultPath = filepath.Join(runDir, "runs", run.RunID, "steps", attempt.AttemptID, "result.json")
		if len(attempt.Result) > 0 {
			var result StepGateResult
			if err := json.Unmarshal(attempt.Result, &result); err != nil {
				t.Fatal(err)
			}
			result.Workspace.Repository = c.identity
			attempt.Result = mustJSON(result)
		}
	}
	service, err := NewService(c)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	taskRecord := TaskRecord{ProjectIdentity: c.identity, TaskID: run.DomainTaskID, ExternalProvider: "beads", ExternalProject: "isolated-merged-close-probe", ExternalID: run.TaskID, Title: run.Title, Claimant: run.Claimant, Source: "beads", Status: "in_progress", Version: 1, CreatedAt: now(), UpdatedAt: now()}
	service.State.Tasks = append(service.State.Tasks, taskRecord)
	service.State.WorkflowRuns = append(service.State.WorkflowRuns, evidence.Workflow)
	service.State.Executions = append(service.State.Executions, evidence.Execution)
	service.State.Attempts = append(service.State.Attempts, evidence.Attempts...)
	service.State.Runs = append(service.State.Runs, run)
	if err := service.save(); err != nil {
		t.Fatal(err)
	}
	if err := service.pollPRsFor(context.Background(), run.RunID); err != nil {
		t.Fatalf("merged PR completion did not close the isolated Beads task: %v", err)
	}
	providerTask, err := service.beads.Show(context.Background(), run.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	workflow, execution := service.minimalRecords(run)
	if providerTask.Status != "closed" || providerTask.Assignee != run.Claimant || run.FinishTaskStatus != "completed" || run.PR.State != "merged" || run.PR.MergeCommit == nil || workflow.Phase != "completed" || execution.CurrentStep != "completed" || service.State.Tasks[0].Status != "closed" {
		t.Fatalf("merged PR and task close facts did not converge: provider=%+v run=%+v workflow=%+v execution=%+v", providerTask, run, workflow, execution)
	}
	if err := service.pollPRsFor(context.Background(), run.RunID); err != nil {
		t.Fatalf("completed run was not idempotent on another poll: %v", err)
	}
	if outputPath := strings.TrimSpace(os.Getenv("STEERLINE_REAL_SUPERVISED_MERGED_CLOSE_OUTPUT")); outputPath != "" {
		output := map[string]any{"captured_at": now(), "run": run, "workflow": workflow, "execution": execution, "task": service.State.Tasks[0], "provider_task": providerTask, "pr": evidence.PR, "isolated_beads_fixture": true, "closed": true}
		encoded, marshalErr := json.MarshalIndent(output, "", "  ")
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if err := os.MkdirAll(filepath.Dir(outputPath), 0700); err != nil {
			t.Fatal(err)
		}
		if err := AtomicWrite(outputPath, append(encoded, '\n')); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("actual merged PR closed isolated Beads task %s at merge %s", run.TaskID, deref(run.PR.MergeCommit))
}

func isBackendError(err error, want BackendErrorCode) bool {
	var backendErr *BackendError
	return errors.As(err, &backendErr) && backendErr.Code == want
}
