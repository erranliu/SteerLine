package orcaworker_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	"github.com/erranliu/SteerLine/internal/steerline/orcaworker"
	"github.com/erranliu/SteerLine/internal/steerline/testprotocol"
)

func TestMain(m *testing.M) {
	for i, arg := range os.Args {
		if arg == "--terminal-fixture" && i+1 < len(os.Args) {
			os.Exit(testprotocol.FakeTerminal(os.Args[i+1], os.Args[i+2:]))
		}
	}
	os.Exit(m.Run())
}

func terminalBackendConfig(t *testing.T, dir string) orcaworker.Config {
	t.Helper()
	return orcaworker.Config{Repository: dir, CommandTimeout: "5s", Target: executionbackend.ExecutionTarget{ID: "orca:fixed-terminal-fixture"},
		Output: func(ctx context.Context, timeout time.Duration, args ...string) ([]byte, error) {
			ctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], append([]string{"--terminal-fixture", dir}, args...)...)
			cmd.Dir = dir
			return cmd.Output()
		},
	}
}

type orcaStopHandleFixture struct {
	identity       executionbackend.ProcessIdentity
	closed, killed bool
}

func (h *orcaStopHandleFixture) Identity() executionbackend.ProcessIdentity { return h.identity }

func (h *orcaStopHandleFixture) Kill() { h.killed = true }

func (h *orcaStopHandleFixture) Close() { h.closed = true }

func TestOrcaTerminalExitCauseConfirmsCommandExit(t *testing.T) {
	var term orcaworker.Terminal
	if err := json.Unmarshal([]byte(`{"handle":"term-exit","connected":false,"exitCause":{"kind":"exited","exitCode":0}}`), &term); err != nil {
		t.Fatal(err)
	}
	if !term.Exited() {
		t.Fatalf("Orca exitCause was not treated as terminal exit evidence: %+v", term)
	}
	term = orcaworker.Terminal{Status: "running", Connected: true}
	if term.Exited() {
		t.Fatal("live Orca terminal was misclassified as exited")
	}
}

func TestOrcaStopExecutionConfirmsFixedTerminalExit(t *testing.T) {
	dir := t.TempDir()
	cfg := terminalBackendConfig(t, dir)
	handle := "terminal-stop-test"
	if err := testprotocol.WriteTerminalFixtureState(dir, testprotocol.TerminalFixtureState{Handle: handle, Incarnation: "inc-stop"}); err != nil {
		t.Fatal(err)
	}
	runID, attemptID := strings.Repeat("a", 32), strings.Repeat("b", 32)
	ref := executionbackend.ExecutionRef{Version: 1, Backend: executionbackend.BackendOrca, Protocol: executionbackend.ProtocolOrcaTerminalV2, Target: cfg.Target, Agent: executionbackend.AgentSpec{Name: "codex"}, RunID: runID, AttemptID: attemptID, External: executionbackend.ProcessIdentity{Kind: "orca", Terminal: handle, RuntimeID: "fake-runtime", Incarnation: "inc-stop", Executable: os.Args[0]}}
	backend := orcaworker.NewBackend(cfg)
	observer := &orcaStopHandleFixture{identity: ref.External}
	receipt, err := backend.StopExecution(context.Background(), ref, executionbackend.Operation{ID: "stop-" + attemptID}, observer)
	if err != nil || receipt.Disposition != "confirmed" {
		t.Fatalf("stop receipt=%#v err=%v", receipt, err)
	}
	obs, err := backend.Observe(context.Background(), ref)
	if err != nil || obs.Liveness != "exited" {
		t.Fatalf("stop evidence=%#v err=%v", obs, err)
	}
	if observer.closed || observer.killed {
		t.Fatal("terminal stop invoked cleanup on the caller's handle")
	}
}

func TestOrcaStopFailureDoesNotConfirmExit(t *testing.T) {
	dir := t.TempDir()
	cfg := terminalBackendConfig(t, dir)
	handle := "terminal-stop-fails"
	if err := testprotocol.WriteTerminalFixtureState(dir, testprotocol.TerminalFixtureState{Handle: handle, Incarnation: "inc-failure"}); err != nil {
		t.Fatal(err)
	}
	ref := executionbackend.ExecutionRef{Backend: executionbackend.BackendOrca, Protocol: executionbackend.ProtocolOrcaTerminalV2, Target: cfg.Target, External: executionbackend.ProcessIdentity{Kind: "orca", Terminal: handle, RuntimeID: "fake-runtime", Incarnation: "inc-failure"}}
	t.Setenv("STEERLINE_FAKE_ORCA_CLOSE_FAIL", "1")
	receipt, err := (orcaworker.NewBackend(cfg)).StopExecution(context.Background(), ref, executionbackend.Operation{}, nil)
	var backendErr *executionbackend.BackendError
	if receipt.Disposition == "confirmed" || !errors.As(err, &backendErr) || backendErr.Code != executionbackend.BackendOutcomeUnknown {
		t.Fatalf("stop failure was misconfirmed: %#v, %v", receipt, err)
	}
	obs, err := (orcaworker.NewBackend(cfg)).Observe(context.Background(), ref)
	if err != nil || obs.Liveness != "live" {
		t.Fatalf("failed stop changed original terminal: %#v, %v", obs, err)
	}
}

func TestSlowOrcaObservationDoesNotBlockOtherRefStop(t *testing.T) {
	dir := t.TempDir()
	cfg := terminalBackendConfig(t, dir)
	cfg.CommandTimeout = "10s"
	backend := orcaworker.NewBackend(cfg)
	refFor := func(handle string) executionbackend.ExecutionRef {
		t.Helper()
		identity := executionbackend.ProcessIdentity{Kind: "orca", RuntimeID: "fake-runtime", Terminal: handle, Incarnation: "inc-" + handle}
		if err := testprotocol.WriteTerminalFixtureState(dir, testprotocol.TerminalFixtureState{Handle: handle, Incarnation: identity.Incarnation}); err != nil {
			t.Fatal(err)
		}
		return executionbackend.ExecutionRef{Backend: executionbackend.BackendOrca, Protocol: executionbackend.ProtocolOrcaTerminalV2, Target: cfg.Target, External: identity}
	}
	slow, stopped := refFor("slow-observe"), refFor("quick-stop")
	marker := filepath.Join(dir, "slow-show-started")
	t.Setenv("STEERLINE_FAKE_ORCA_SLOW_SHOW_HANDLE", slow.External.Terminal)
	t.Setenv("STEERLINE_FAKE_ORCA_SLOW_SHOW_STARTED", marker)
	t.Setenv("STEERLINE_FAKE_ORCA_SLOW_SHOW_MS", "5000")
	slowCtx, cancelSlow := context.WithCancel(context.Background())
	defer cancelSlow()
	slowDone := make(chan error, 1)
	go func() { _, err := backend.Observe(slowCtx, slow); slowDone <- err }()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("slow observation did not start")
	}
	stopCtx, cancelStop := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancelStop()
	started := time.Now()
	receipt, err := backend.StopExecution(stopCtx, stopped, executionbackend.Operation{}, nil)
	if err != nil || receipt.Disposition != "confirmed" || time.Since(started) >= 3*time.Second {
		t.Fatalf("other ref stop waited for slow observation: %#v %v", receipt, err)
	}
	cancelSlow()
	select {
	case <-slowDone:
	case <-time.After(2 * time.Second):
		t.Fatal("canceled slow observation leaked")
	}
	state, err := testprotocol.ReadTerminalFixtureState(dir, slow.External.Terminal)
	if err != nil || state.Closed {
		t.Fatalf("canceling observation closed another terminal: %+v %v", state, err)
	}
}

func TestOrcaSameRefObservationsDoNotOverlapOrLeakGate(t *testing.T) {
	dir := t.TempDir()
	cfg := terminalBackendConfig(t, dir)
	backend := orcaworker.NewBackend(cfg)
	handle := "same-ref-observe"
	identity := executionbackend.ProcessIdentity{Kind: "orca", RuntimeID: "fake-runtime", Terminal: handle, Incarnation: "inc-same-ref"}
	if err := testprotocol.WriteTerminalFixtureState(dir, testprotocol.TerminalFixtureState{Handle: handle, Incarnation: identity.Incarnation}); err != nil {
		t.Fatal(err)
	}
	ref := executionbackend.ExecutionRef{Backend: executionbackend.BackendOrca, Protocol: executionbackend.ProtocolOrcaTerminalV2, Target: cfg.Target, External: identity}
	marker := filepath.Join(dir, "first-show-started")
	t.Setenv("STEERLINE_FAKE_ORCA_SLOW_SHOW_HANDLE", handle)
	t.Setenv("STEERLINE_FAKE_ORCA_SLOW_SHOW_STARTED", marker)
	t.Setenv("STEERLINE_FAKE_ORCA_SLOW_SHOW_MS", "500")
	t.Setenv("STEERLINE_FAKE_ORCA_SHOW_EXCLUSIVE", filepath.Join(dir, "active-show"))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	results := make(chan error, 2)
	go func() { _, err := backend.Observe(ctx, ref); results <- err }()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("first observation did not enter the fake platform")
	}
	go func() { _, err := backend.Observe(ctx, ref); results <- err }()
	for i := 0; i < 2; i++ {
		if err := <-results; err != nil {
			t.Fatalf("same-ref observation overlapped or failed: %v", err)
		}
	}

}

func TestOrcaObservationRejectsReplacedIncarnationAndConnection(t *testing.T) {
	dir := t.TempDir()
	cfg := terminalBackendConfig(t, dir)
	if err := testprotocol.WriteTerminalFixtureState(dir, testprotocol.TerminalFixtureState{Handle: "term-fixed", Incarnation: "inc-original"}); err != nil {
		t.Fatal(err)
	}
	ref := executionbackend.ExecutionRef{Backend: executionbackend.BackendOrca, Protocol: executionbackend.ProtocolOrcaTerminalV2, Target: cfg.Target, Agent: executionbackend.AgentSpec{Name: "codex"}, External: executionbackend.ProcessIdentity{Kind: "orca", Terminal: "term-fixed", RuntimeID: "fake-runtime", Incarnation: "inc-original"}}
	backend := orcaworker.NewBackend(cfg)
	if obs, err := backend.Observe(context.Background(), ref); err != nil || obs.Liveness != "live" {
		t.Fatalf("fixed terminal was not live: %#v, %v", obs, err)
	}
	t.Setenv("STEERLINE_FAKE_ORCA_INCARNATION", "inc-replacement")
	var mismatch *executionbackend.BackendError
	if _, err := backend.Observe(context.Background(), ref); !errors.As(err, &mismatch) || mismatch.Code != executionbackend.BackendIdentityMismatch {
		t.Fatalf("replacement incarnation was accepted: %v", err)
	}
	t.Setenv("STEERLINE_FAKE_ORCA_INCARNATION", "")
	cfg.Target.ID += "-different-connection"
	if _, err := (orcaworker.NewBackend(cfg)).Observe(context.Background(), ref); !errors.As(err, &mismatch) || mismatch.Code != executionbackend.BackendIdentityMismatch {
		t.Fatalf("changed connection was accepted: %v", err)
	}
}
