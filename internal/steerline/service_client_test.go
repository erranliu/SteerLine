package steerline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestConnectionClassifiesProcessAndSnapshotIndependently(t *testing.T) {
	s := controlReviewService(t)
	c := s.Config
	id, err := currentProcessIdentity()
	require(t, err)
	rt := s.State
	rt.UpdatedAt, rt.HeartbeatAt = now(), now()
	rt.Service.Mode, rt.Service.Status, rt.Service.Ready = "scheduler", "running", true
	rt.Service.PID, rt.Service.ProcessIdentity = id.PID, id
	rt.Service.ConfigDigest = c.configDigest
	rt.Service.ControlProtocol, rt.Service.Capabilities = ControlProtocolVersion, []string{"stop_service"}
	write := func(value Runtime) {
		t.Helper()
		b, e := json.Marshal(value)
		require(t, e)
		require(t, AtomicWrite(filepath.Join(c.RunDir, c.RuntimeFile), b))
	}

	write(rt)
	connected := ConnectService(context.Background(), c)
	if connected.State != ConnectionConnected || !connected.CanControl || !connected.IdentityMatched {
		t.Fatalf("connected process classified as %+v", connected)
	}
	denied := connectServiceWithProbe(context.Background(), c, func(*ProcessIdentity) ProcessProbe {
		return ProcessProbe{State: ProcessUnknown, Reason: "access denied"}
	})
	if denied.State != ConnectionUnknown || denied.CanStart || denied.CanControl || denied.CanStop || !strings.Contains(denied.Diagnostic, "access denied") {
		t.Fatalf("inaccessible process was treated as a controllable or replaceable service: %+v", denied)
	}

	stale := rt
	stale.HeartbeatAt = now().Add(-time.Minute)
	write(stale)
	if got := ConnectService(context.Background(), c); got.State != ConnectionStale || got.CanStart || !got.CanStop {
		t.Fatalf("stale heartbeat classification=%+v", got)
	}

	initializing := rt
	initializing.Service.Ready = false
	write(initializing)
	if got := ConnectService(context.Background(), c); got.State != ConnectionInitializing || got.CanControl || !got.CanStop {
		t.Fatalf("ready=false classification=%+v", got)
	}

	incompatible := rt
	incompatible.Service.ControlProtocol = ControlProtocolVersion + 1
	write(incompatible)
	if got := ConnectService(context.Background(), c); got.State != ConnectionIncompatible || got.CanControl || got.CanStop {
		t.Fatalf("protocol classification=%+v", got)
	}

	changed := rt
	changed.Service.StartupID = ptr(newID())
	changed.Service.ConfigDigest = strings.Repeat("f", 64)
	oldConfig := c
	oldConfig.configDigest = changed.Service.ConfigDigest
	if e := os.MkdirAll(filepath.Join(c.RunDir, "service", "startups", *changed.Service.StartupID), 0700); e != nil {
		t.Fatal(e)
	}
	oldRunDir, e := expectedRunDir(c)
	require(t, e)
	record := initialStartupRecord(oldConfig, *changed.Service.StartupID)
	record.Status = "started"
	record.InstanceID = changed.Service.InstanceID
	record.ProcessIdentity = id
	require(t, writeStartupRecord(c, record))
	write(changed)
	configChanged := ConnectService(context.Background(), c)
	if configChanged.State != ConnectionConfigChanged || configChanged.CanControl || !configChanged.CanStop {
		t.Fatalf("config change classification=%+v; expected run_dir=%s", configChanged, oldRunDir)
	}
	unboundRuntime := rt
	unboundRuntime.Service.ConfigDigest = strings.Repeat("e", 64)
	write(unboundRuntime)
	if got := ConnectService(context.Background(), c); got.State != ConnectionIdentityError || got.CanStop {
		t.Fatalf("runtime without a startup binding must remain unbound: %+v", got)
	}
	repoLock, err := LockRepository(c.Identity())
	require(t, err)
	t.Cleanup(func() {
		if repoLock != nil {
			repoLock.Close()
		}
	})
	runLock, err := LockRepository(c.RunDir)
	require(t, err)
	t.Cleanup(func() {
		if runLock != nil {
			runLock.Close()
		}
	})
	if got := ConnectService(context.Background(), c); got.State != ConnectionIdentityError || got.CanStop || got.CanControl {
		t.Fatalf("unbound runtime must not gain identity from service locks=%+v", got)
	}
	repoLock.Close()
	runLock.Close()

	wrongRepo := rt
	wrongRepo.Repository.Identity = filepath.Join(filepath.Dir(c.Repository), "other.git")
	wrongRepo.Service.PID = 2_000_000_000
	wrongRepo.Service.ProcessIdentity = &ProcessIdentity{Kind: "os", PID: wrongRepo.Service.PID, Created: "unused", Executable: "unused"}
	write(wrongRepo)
	if got := ConnectService(context.Background(), c); got.State != ConnectionIdentityError || got.CanStart {
		t.Fatalf("mismatched repository must not be treated as offline: %+v", got)
	}
}

func TestProcessProbeSeparatesMatchReuseAndAbsent(t *testing.T) {
	id, err := currentProcessIdentity()
	require(t, err)
	if got := probeProcess(id); got.State != ProcessMatches {
		t.Fatalf("current process probe=%+v", got)
	}
	reused := *id
	reused.Created += "-different"
	if got := probeProcess(&reused); got.State != ProcessReused {
		t.Fatalf("reused PID probe=%+v", got)
	}
	absent := *id
	absent.PID = 2_000_000_000
	if got := probeProcess(&absent); got.State != ProcessAbsent {
		t.Fatalf("absent process probe=%+v", got)
	}
	if got := probeProcess(nil); got.State != ProcessUnknown {
		t.Fatalf("missing identity probe=%+v", got)
	}
}

func TestLaunchLockSerializesCompetingClients(t *testing.T) {
	path := filepath.Join(t.TempDir(), "service", "launch.lock")
	require(t, os.MkdirAll(filepath.Dir(path), 0700))
	first, err := acquireLaunchLock(context.Background(), path)
	require(t, err)
	gotLock := make(chan *RepoLock, 1)
	gotErr := make(chan error, 1)
	go func() {
		lock, err := acquireLaunchLock(context.Background(), path)
		gotLock <- lock
		gotErr <- err
	}()
	select {
	case lock := <-gotLock:
		if lock != nil {
			lock.Close()
		}
		t.Fatal("second launch client passed a held launch lock")
	case <-time.After(120 * time.Millisecond):
	}
	first.Close()
	select {
	case lock := <-gotLock:
		if err := <-gotErr; err != nil {
			t.Fatal(err)
		}
		if lock == nil {
			t.Fatal("second launch client received no lock")
		}
		lock.Close()
	case <-time.After(3 * time.Second):
		t.Fatal("second launch client did not acquire the released lock")
	}
}

func TestStartServiceReobservesExistingPendingAttempt(t *testing.T) {
	f := newFixture(t)
	config, configPath := lifecycleConfigFile(t, f, f.c.RunDir)
	id := newID()
	_, _, err := createStartupAttempt(config, id, true)
	require(t, err)
	processIdentity, err := currentProcessIdentity()
	require(t, err)
	record := initialStartupRecord(config, id)
	record.ProcessIdentity = processIdentity
	require(t, writeStartupRecord(config, record))
	exe, err := os.Executable()
	require(t, err)
	options := StartServiceOptions{Executable: exe, WaitTimeout: 150 * time.Millisecond, PollInterval: 15 * time.Millisecond}
	first, err := StartService(context.Background(), configPath, options)
	if err != nil || first.Status != "pending" || first.StartupID != id {
		t.Fatalf("first pending observation=%+v err=%v", first, err)
	}
	second, err := StartService(context.Background(), configPath, options)
	if err != nil || second.Status != "pending" || second.StartupID != id {
		t.Fatalf("repeated start launched or changed the pending attempt: second=%+v err=%v", second, err)
	}
	updated, err := readStartupRecord(config, id)
	require(t, err)
	if updated.Status != "starting" || updated.ProcessIdentity == nil || updated.ProcessIdentity.PID != os.Getpid() {
		t.Fatalf("pending attempt identity changed: %+v", updated)
	}
}

func TestWaitForStartupMarksConfirmedEarlyExit(t *testing.T) {
	f := newFixture(t)
	config, _ := lifecycleConfigFile(t, f, f.c.RunDir)
	id := newID()
	_, logPath, err := createStartupAttempt(config, id, true)
	require(t, err)
	record := initialStartupRecord(config, id)
	record.ProcessIdentity = &ProcessIdentity{Kind: "os", PID: 2_000_000_000, Created: "no-such-process", Executable: "no-such-executable"}
	require(t, writeStartupRecord(config, record))
	result, err := waitForStartup(context.Background(), config, id, logPath, StartServiceOptions{WaitTimeout: time.Second, PollInterval: 10 * time.Millisecond})
	if err != nil || result.Status != "failed" || !strings.Contains(result.Error, "exited before") {
		t.Fatalf("early process exit result=%+v err=%v", result, err)
	}
	updated, err := readStartupRecord(config, id)
	if err != nil || updated.Status != "failed" {
		t.Fatalf("early process exit record=%+v err=%v", updated, err)
	}
}

func TestStartupIDAndOutputPathValidation(t *testing.T) {
	f := newFixture(t)
	for _, id := range []string{"../outside", strings.Repeat("A", 32), strings.Repeat("0", 31), strings.Repeat("g", 32)} {
		if _, err := startupPath(f.c.RunDir, id); err == nil {
			t.Errorf("startup ID %q was accepted", id)
		}
	}
	id := newID()
	path, err := startupPath(f.c.RunDir, id)
	require(t, err)
	if !inside(filepath.Join(f.c.RunDir, "service", "startups"), filepath.Dir(path)) {
		t.Fatalf("validated startup path escaped managed root: %s", path)
	}
}

func TestStartServiceSerializesSameConfig(t *testing.T) {
	if runtimeGOOSWindows() {
		t.Skip("Windows service-window uniqueness is verified through service locks")
	}
	if !detachedChildSupported(t) {
		t.Skip("this host keeps child processes in a Job Object and rejects breakaway")
	}
	f := newFixture(t)
	config, configPath := lifecycleConfigFile(t, f, f.c.RunDir)
	serviceExe := buildServiceLifecycleCLI(t)
	t.Cleanup(func() {
		connection := ConnectService(context.Background(), config)
		if connection.CanStop {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			_, _ = StopService(ctx, config, "test-cleanup", 12*time.Second)
		}
	})
	opts := StartServiceOptions{Executable: serviceExe, WaitTimeout: 20 * time.Second, PollInterval: 30 * time.Millisecond}
	type outcome struct {
		result ServiceStartResult
		err    error
	}
	starts := make(chan outcome, 2)
	for range 2 {
		go func() {
			result, err := StartService(context.Background(), configPath, opts)
			starts <- outcome{result, err}
		}()
	}
	first, second := <-starts, <-starts
	if first.err != nil || second.err != nil || first.result.Status != "connected" || second.result.Status != "connected" {
		t.Fatalf("concurrent starts: first=%+v/%v second=%+v/%v", first.result, first.err, second.result, second.err)
	}
	firstID := first.result.Connection.Snapshot.Runtime.Service.InstanceID
	secondID := second.result.Connection.Snapshot.Runtime.Service.InstanceID
	if firstID == "" || firstID != secondID {
		t.Fatalf("same config started different instances: %q, %q", firstID, secondID)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	stopped, err := StopService(ctx, config, "lifecycle-test", 12*time.Second)
	if err != nil || stopped.Status != "stopped" {
		t.Fatalf("stop concurrent winner: %+v %v", stopped, err)
	}
}

func TestStartServiceReportsPreflightFailureAndPendingHandshake(t *testing.T) {
	if runtimeGOOSWindows() {
		t.Skip("Windows service-window diagnostics use foreground startup records")
	}
	if !detachedChildSupported(t) {
		t.Skip("this Windows host keeps child processes in a Job Object and rejects breakaway")
	}
	t.Run("preflight failure is logged", func(t *testing.T) {
		f := newFixture(t)
		f.c.PR.Repository = "wrong/repository"
		config, configPath := lifecycleConfigFile(t, f, f.c.RunDir)
		result, err := StartService(context.Background(), configPath, StartServiceOptions{Executable: buildServiceLifecycleCLI(t), WaitTimeout: 15 * time.Second})
		if err != nil || result.Status != "failed" || !strings.Contains(result.Error, "does not match Git remote") {
			t.Fatalf("preflight result=%+v err=%v", result, err)
		}
		if result.StartupID == "" || result.LogPath == "" {
			t.Fatalf("preflight failure lacks startup identity/log: %+v", result)
		}
		record, readErr := readStartupRecord(config, result.StartupID)
		if readErr != nil || record.Status != "failed" || !strings.Contains(record.Error, "does not match Git remote") {
			t.Fatalf("preflight startup record=%+v err=%v", record, readErr)
		}
		log, readErr := os.ReadFile(result.LogPath)
		require(t, readErr)
		if !strings.Contains(string(log), "does not match Git remote") {
			t.Fatalf("preflight diagnostic missing from log: %s", log)
		}
	})

	t.Run("timeout keeps startup identity and does not repeat launch", func(t *testing.T) {
		var pendingIdentity *ProcessIdentity
		t.Cleanup(func() {
			if pendingIdentity != nil {
				_ = terminatePendingTestProcess(pendingIdentity)
			}
		})
		f := newFixture(t)
		t.Setenv("STEERLINE_FAKE_PR_DELAY", "2s")
		config, configPath := lifecycleConfigFile(t, f, f.c.RunDir)
		serviceExe := buildServiceLifecycleCLI(t)
		result, err := StartService(context.Background(), configPath, StartServiceOptions{Executable: serviceExe, WaitTimeout: 350 * time.Millisecond, PollInterval: 25 * time.Millisecond})
		if err != nil || result.Status != "pending" || result.StartupID == "" {
			t.Fatalf("pending startup result=%+v err=%v", result, err)
		}
		again, err := StartService(context.Background(), configPath, StartServiceOptions{Executable: serviceExe, WaitTimeout: 350 * time.Millisecond, PollInterval: 25 * time.Millisecond})
		if err != nil || again.Status != "pending" || again.StartupID != result.StartupID {
			t.Fatalf("repeat observation changed startup attempt: first=%+v second=%+v err=%v", result, again, err)
		}
		record, err := readStartupRecord(config, result.StartupID)
		require(t, err)
		pendingIdentity = record.ProcessIdentity
		if record.Status != "starting" || record.ProcessIdentity == nil || probeProcess(record.ProcessIdentity).State != ProcessMatches {
			t.Fatalf("pending startup lost process identity: %+v", record)
		}
		if _, err := os.Stat(filepath.Join(config.RunDir, config.RuntimeFile)); err == nil {
			t.Fatal("preflight delay unexpectedly published runtime before acquiring service locks")
		}
		if err := terminatePendingTestProcess(record.ProcessIdentity); err != nil {
			t.Fatal(err)
		}
		pendingIdentity = nil
		observed, err := waitForStartup(context.Background(), config, result.StartupID, result.LogPath, StartServiceOptions{WaitTimeout: 3 * time.Second})
		if err != nil || observed.Status != "failed" {
			t.Fatalf("killed startup was not observed as failed: %+v %v", observed, err)
		}
	})

	t.Run("early process exit is not reported as online", func(t *testing.T) {
		t.Setenv("STEERLINE_LIFECYCLE_EXIT_EARLY", "1")
		f := newFixture(t)
		config, configPath := lifecycleConfigFile(t, f, f.c.RunDir)
		testExe, err := os.Executable()
		require(t, err)
		result, err := StartService(context.Background(), configPath, StartServiceOptions{Executable: testExe, WaitTimeout: 2 * time.Second, PollInterval: 30 * time.Millisecond})
		if err != nil || result.Status != "failed" || result.StartupID == "" {
			t.Fatalf("early-exit result=%+v err=%v", result, err)
		}
		if result.Connection.State == ConnectionConnected || result.Connection.CanControl {
			t.Fatalf("early exit was reported as connected: %+v", result)
		}
		if record, e := readStartupRecord(config, result.StartupID); e != nil || record.Status != "failed" {
			t.Fatalf("early exit startup record=%+v err=%v", record, e)
		}
	})
}

func TestRunCommandRejectsLegacyTaskAuthorityBeforeWritingState(t *testing.T) {
	f := newFixture(t)
	config := f.c
	config.TaskAuthority = "beads"
	data, err := json.Marshal(config)
	require(t, err)
	configPath := filepath.Join(t.TempDir(), "legacy-config.json")
	require(t, os.WriteFile(configPath, data, 0600))
	output, runErr := exec.Command(buildServiceLifecycleCLI(t), "run", "-config", configPath, "-dispatch", "manual", "-console", "off").CombinedOutput()
	if runErr == nil || !strings.Contains(string(output), "task_authority") {
		t.Fatalf("legacy task authority was accepted: err=%v output=%s", runErr, output)
	}
	if _, err := os.Stat(filepath.Join(f.c.RunDir, f.c.RuntimeFile)); !os.IsNotExist(err) {
		t.Fatalf("rejected legacy config wrote native runtime state: %v", err)
	}
	if _, err := os.Stat(filepath.Join(f.c.RunDir, "steerline.db")); !os.IsNotExist(err) {
		t.Fatalf("rejected legacy config initialized a native database: %v", err)
	}
}

func lifecycleConfigFile(t *testing.T, f *fixture, runDir string) (Config, string) {
	t.Helper()
	c := f.c
	c.RunDir = runDir
	c.WorktreeRoot = ""
	data, err := json.Marshal(c)
	require(t, err)
	if err = os.MkdirAll(runDir, 0700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(filepath.Dir(runDir), "service-config.json")
	require(t, os.WriteFile(configPath, data, 0600))
	loaded, err := LoadConfig(configPath)
	require(t, err)
	require(t, loaded.ResolveIdentity(context.Background()))
	return loaded, configPath
}

func buildServiceLifecycleCLI(t *testing.T) string {
	t.Helper()
	lifecycleCLIBuildOnce.Do(func() {
		root, err := sharedTestArtifactsDir()
		if err != nil {
			lifecycleCLIBuildErr = err
			return
		}
		name := "steerline-lifecycle"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		lifecycleCLIPath = filepath.Join(root, name)
		cmd := exec.Command("go", "build", "-o", lifecycleCLIPath, "../../cmd/steerline")
		if output, buildErr := cmd.CombinedOutput(); buildErr != nil {
			lifecycleCLIBuildErr = fmt.Errorf("%w: %s", buildErr, output)
		}
	})
	if lifecycleCLIBuildErr != nil {
		t.Fatalf("build shared service lifecycle CLI: %v", lifecycleCLIBuildErr)
	}
	return lifecycleCLIPath
}

var (
	lifecycleCLIBuildOnce sync.Once
	lifecycleCLIPath      string
	lifecycleCLIBuildErr  error
)

func terminatePendingTestProcess(identity *ProcessIdentity) error {
	if identity == nil || probeProcess(identity).State != ProcessMatches {
		return fmt.Errorf("refusing to terminate a process without its matching test identity")
	}
	process, err := os.FindProcess(identity.PID)
	if err != nil {
		return err
	}
	return process.Kill()
}

func detachedChildSupported(t *testing.T) bool {
	t.Helper()
	exe, err := os.Executable()
	require(t, err)
	cmd := exec.Command(exe, "--fake", "sleep")
	identity, err := startDetachedProcess(cmd)
	if err != nil {
		if errors.Is(err, errDetachUnsupported) && os.Getenv("STEERLINE_REQUIRE_DETACHED_TESTS") != "1" {
			t.Logf("detached test capability unavailable: %v", err)
			return false
		}
		t.Fatalf("required detached process capability failed: %v", err)
	}
	if err = cmd.Process.Kill(); err != nil {
		t.Fatalf("terminate detached capability probe %d: %v", identity.PID, err)
	}
	_ = cmd.Wait()
	return true
}
