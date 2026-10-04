package orca_remote_worker_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	remote "github.com/erranliu/SteerLine/internal/steerline/orca_remote_worker"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
	"github.com/erranliu/SteerLine/internal/steerline/testprotocol"
)

func TestSupervisedOrcaPreflightPinsRemoteIdentityAndVersionFamily(t *testing.T) {
	c := supervisedFixtureConfig(t)
	pair, err := remote.Preflight(context.Background(), c)
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

func TestSupervisedOrcaPreflightRejectsVersionOrCapabilityDrift(t *testing.T) {
	c := supervisedFixtureConfig(t)
	t.Setenv("STEERLINE_FAKE_SUPERVISED_REMOTE_VERSION", "1.5.0")
	if _, err := remote.Preflight(context.Background(), c); err == nil || !strings.Contains(err.Error(), "version-family mismatch") {
		t.Fatalf("version mismatch error = %v", err)
	}
	t.Setenv("STEERLINE_FAKE_SUPERVISED_REMOTE_VERSION", "1.4.210")
	t.Setenv("STEERLINE_FAKE_SUPERVISED_MISSING_CAPABILITY", "1")
	if _, err := remote.Preflight(context.Background(), c); err == nil || !strings.Contains(err.Error(), "agent.launch.v2") {
		t.Fatalf("missing capability error = %v", err)
	}
	t.Setenv("STEERLINE_FAKE_SUPERVISED_MISSING_CAPABILITY", "")
	t.Setenv("STEERLINE_FAKE_SUPERVISED_PAIRED_DEVICE_ID", "replacement-device")
	if _, err := remote.Preflight(context.Background(), c); err == nil || !strings.Contains(err.Error(), "paired-device") {
		t.Fatalf("connection replacement error = %v", err)
	}
}

func TestSupervisedOrcaClientPinsLaunchAndReadsExactDispatch(t *testing.T) {
	c := supervisedFixtureConfig(t)
	t.Setenv("STEERLINE_FAKE_SUPERVISED_WORKTREE_ID", "repo-test::C:/remote/spec03-test")
	workflowRunID := newID()
	client := remote.New(c)
	coordinator, err := client.CreateCoordinator(context.Background(), workflowRunID, "fake-center-runtime")
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := orcaworker.CreateRun(context.Background(), client.CenterCall, "remote worker fixture", coordinator, "fake-center-runtime")
	runID, requestID := receipt.ID, receipt.RequestID
	remoteSettings := c.Settings
	remoteSettings.CoordinatorTerminal = coordinator
	c.Settings = remoteSettings
	backend := remote.NewBackend(c)
	if err != nil {
		t.Fatal(err)
	}
	if runID != "orca-run-test" {
		t.Fatalf("created Orca Run ID = %q", runID)
	}
	if requestID != "request-run-create" || coordinator != "term-"+workflowRunID {
		t.Fatalf("Orca Run-create request ID = %q", requestID)
	}
	started, err := remote.New(c).StartTask(context.Background(), runID, "task-test", "repo-test::C:/remote/spec03-test", nil)
	if err != nil {
		t.Fatal(err)
	}
	if started.TaskID != "task-test" || started.DispatchID != "dispatch-test" || started.WorktreeID != "repo-test::C:/remote/spec03-test" || started.PlatformRequestID != "request-worker-start" || started.State != "ready" || started.Stage != "input_accepted" {
		t.Fatalf("worker start receipt = %+v", started)
	}
	observed, err := remote.New(c).Show(context.Background(), started)
	if err != nil {
		t.Fatal(err)
	}
	if observed.Identity.RuntimeID != "fake-remote-runtime" || observed.Identity.OrchestrationRun != runID || observed.Identity.Task != started.TaskID || observed.Identity.Dispatch != started.DispatchID || observed.Identity.WorktreeID != started.WorktreeID || observed.Liveness != "live" || observed.WorktreePath != "C:/remote/spec03-test" || observed.Branch != "erranliu/spec03-test" {
		t.Fatalf("worker observation = %+v", observed)
	}
	target := targetFor(c)
	target.OrchestrationRun, target.ExternalTask, target.Dispatch, target.WorktreeID, target.ExecutionHost = runID, started.TaskID, started.DispatchID, started.WorktreeID, "env-home"
	ref := executionbackend.ExecutionRef{Version: executionbackend.ExecutionRecordVersion, Backend: executionbackend.BackendOrca, Protocol: executionbackend.ProtocolOrcaSupervised, Target: target, Agent: agentFor(c), RunID: workflowRunID, AttemptID: newID(), External: observed.Identity}
	t.Setenv("STEERLINE_FAKE_SUPERVISED_WORKER_EPOCH", "replacement-remote-runtime")
	if _, err := backend.Observe(context.Background(), ref); err == nil || !isBackendError(err, executionbackend.BackendIdentityMismatch) {
		t.Fatalf("replacement remote runtime was treated as the original Dispatch: %v", err)
	}
}

func TestSupervisedOrcaClientRejectsReturnedReplacementWorktree(t *testing.T) {
	c := supervisedFixtureConfig(t)
	_, err := remote.New(c).StartTask(context.Background(), "orca-run-test", "task-test", "repo-test::C:/remote/expected", nil)
	err = identityError(err)
	if err == nil || !isBackendError(err, executionbackend.BackendIdentityMismatch) {
		t.Fatalf("replacement worktree error = %v", err)
	}
}

func TestSupervisedRunCoordinatorsAreDistinctAndFenced(t *testing.T) {
	c := supervisedFixtureConfig(t)
	client := remote.New(c)
	a, b := newID(), newID()
	first, err := client.CreateCoordinator(context.Background(), a, "fake-center-runtime")
	if err != nil {
		t.Fatal(err)
	}
	second, err := client.CreateCoordinator(context.Background(), b, "fake-center-runtime")
	if err != nil || first == second {
		t.Fatalf("WorkflowRuns shared a coordinator: %q / %q, %v", first, second, err)
	}
	t.Setenv("STEERLINE_FAKE_SUPERVISED_RUN_COORDINATOR", first)
	ref := executionbackend.ExecutionRef{RunID: a, Target: executionbackend.ExecutionTarget{CoordinatorTerminal: first}, External: executionbackend.ProcessIdentity{Kind: "orca_supervised", OrchestrationRun: "orca-run-test", CoordinatorRuntimeID: "fake-center-runtime"}}
	if _, err := client.Check(context.Background(), remote.FromExecutionRef(ref)); err != nil {
		t.Fatalf("exact coordinator was fenced: %v", err)
	}
	t.Setenv("STEERLINE_FAKE_SUPERVISED_RUN_COORDINATOR", second)
	if _, err := client.Check(context.Background(), remote.FromExecutionRef(ref)); err == nil || !isBackendError(identityError(err), executionbackend.BackendIdentityMismatch) {
		t.Fatalf("Run rebinding was not rejected before inbox read: %v", err)
	}
}

func TestSettledSupervisedDispatchRetainsExactBindingWithoutLiveTerminal(t *testing.T) {
	c := supervisedFixtureConfig(t)
	t.Setenv("STEERLINE_FAKE_SUPERVISED_DISPATCH_STATE", "completed")
	started := remote.StartReceipt{RunID: "orca-run-test", TaskID: "task-test", DispatchID: "dispatch-test", WorktreeID: "repo-test::C:/remote/steerline-run", CenterRuntimeID: "fake-center-runtime"}
	observed, err := remote.New(c).Show(context.Background(), started)
	if err != nil {
		t.Fatal(err)
	}
	ref := supervisedPromptTestRef(c, started, observed)
	t.Setenv("STEERLINE_FAKE_SUPERVISED_SETTLED_NO_TERMINAL", "1")
	if _, err := remote.New(c).Show(context.Background(), started); err == nil {
		t.Fatal("settled worker unexpectedly retained a live terminal observation")
	}
	if state, err := remote.New(c).Settlement(context.Background(), remote.FromExecutionRef(ref), "succeeded"); err != nil || state != "succeeded" {
		t.Fatalf("durable settled Dispatch was rejected: %q, %v", state, err)
	}
	if _, err := remote.New(c).Settlement(context.Background(), remote.FromExecutionRef(ref), "failed"); !isBackendError(identityError(err), executionbackend.BackendIdentityMismatch) {
		t.Fatalf("conflicting worker_done outcome was accepted: %v", err)
	}
	t.Setenv("STEERLINE_FAKE_SUPERVISED_WORKER_EPOCH", "replacement-remote-runtime")
	if _, err := remote.New(c).Settlement(context.Background(), remote.FromExecutionRef(ref), "succeeded"); !isBackendError(identityError(err), executionbackend.BackendIdentityMismatch) {
		t.Fatalf("replacement remote runtime was accepted: %v", err)
	}
}

func TestSupervisedReplyRejectsEscapedOversizeBeforeOrcaCall(t *testing.T) {
	c := supervisedFixtureConfig(t)
	started := remote.StartReceipt{RunID: "orca-run-test", TaskID: "task-test", DispatchID: "dispatch-test"}
	worktreeID := "repo-test::C:/remote/steerline-run"
	observation := remote.WorkerObservation{Identity: executionbackend.ProcessIdentity{Kind: "orca_supervised", RuntimeID: "fake-remote-runtime", CoordinatorRuntimeID: "fake-center-runtime", Terminal: "terminal-test", Incarnation: "inc-test", OrchestrationRun: started.RunID, Task: started.TaskID, Dispatch: started.DispatchID, ExecutionHost: "env-home", WorktreeID: worktreeID}, WorktreePath: "C:/remote/steerline-run", Branch: "erranliu/steerline-run"}
	ref := supervisedPromptTestRef(c, started, observation)
	t.Setenv("STEERLINE_FAKE_SUPERVISED_OBSERVE_UNAVAILABLE", "1")
	_, err := remote.New(c).Reply(context.Background(), remote.FromExecutionRef(ref), "question-test", "x"+strings.Repeat("\n", 4000))
	if !isBackendError(identityError(err), executionbackend.BackendRejected) || !strings.Contains(err.Error(), "serialized message exceeds") {
		t.Fatalf("escaped oversized reply reached Orca observation or send: %v", err)
	}
	_, err = remote.New(c).Reply(context.Background(), remote.FromExecutionRef(ref), "question-test", "yes")
	if !isBackendError(identityError(err), executionbackend.BackendUnreachable) {
		t.Fatalf("bounded reply was rejected before Orca observation: %v", err)
	}
}

func supervisedPromptTestRef(c remote.Config, started remote.StartReceipt, observation remote.WorkerObservation) executionbackend.ExecutionRef {
	target := targetFor(c)
	target.OrchestrationRun = started.RunID
	target.ExternalTask = started.TaskID
	target.Dispatch = started.DispatchID
	target.ExecutionHost = observation.Identity.ExecutionHost
	target.WorktreeID = observation.Identity.WorktreeID
	target.Branch = observation.Branch
	target.BaseRevision = strings.Repeat("a", 40)
	target.CoordinatorTerminal = c.Settings.CoordinatorTerminal
	return executionbackend.ExecutionRef{Version: executionbackend.ExecutionRecordVersion, Backend: executionbackend.BackendOrca, Protocol: executionbackend.ProtocolOrcaSupervised, Target: target, Agent: executionbackend.AgentSpec{Name: "codex"}, RunID: newID(), AttemptID: "attempt-test", WorkspacePath: observation.WorktreePath, WorkspaceBranch: observation.Branch, BaseCommit: target.BaseRevision, External: observation.Identity, Generation: observation.Identity.RuntimeID + ":" + observation.Identity.Incarnation}
}

func TestMain(m *testing.M) {
	for i, arg := range os.Args {
		if arg == "--fake-supervised" {
			os.Exit(testprotocol.FakeSupervisedOrca(os.Args[i+1:]))
		}
	}
	os.Exit(m.Run())
}

func supervisedFixtureConfig(t *testing.T) remote.Config {
	t.Helper()
	remoteURL := "https://git.example/owner/repo.git"
	t.Setenv("STEERLINE_FAKE_SUPERVISED_REMOTE_URL", remoteURL)
	return remote.Config{Repository: t.TempDir(), ProjectID: "fixture-project", Agent: "codex", Model: "gpt-6-luna", ReasoningEffort: "high", CommandTimeout: "5s",
		Settings: remote.Settings{ConnectionID: "home-connection", EnvironmentID: "env-home", RepositorySelector: "id:repo-test", CoordinatorTerminal: "term-coordinator", ExpectedCodexVersion: "codex-cli 0.156.1", PollInterval: "2s"},
		Origin:   func(context.Context) ([]byte, error) { return []byte(remoteURL), nil },
		Call: func(ctx context.Context, timeout time.Duration, args ...string) ([]byte, error) {
			ctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			return exec.CommandContext(ctx, os.Args[0], append([]string{"--fake-supervised"}, args...)...).Output()
		}}
}

func targetFor(c remote.Config) executionbackend.ExecutionTarget {
	return executionbackend.ExecutionTarget{ID: "orca-supervised:" + c.Settings.ConnectionID, ConnectionID: c.Settings.ConnectionID, ExternalRuntime: c.Settings.EnvironmentID}
}

func agentFor(c remote.Config) executionbackend.AgentSpec {
	return executionbackend.AgentSpec{Name: c.Agent}
}

func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

func identityError(err error) error { return err }

func isBackendError(err error, code executionbackend.BackendErrorCode) bool {
	var e *remote.Error
	if errors.As(err, &e) {
		return e.Code == code
	}
	var backend *executionbackend.BackendError
	return errors.As(err, &backend) && backend.Code == code
}
