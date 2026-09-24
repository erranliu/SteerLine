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
	"testing"
	"time"
)

func TestConnectionClassifiesProcessAndSnapshotIndependently(t *testing.T) {
	f := newFixture(t)
	c := f.c
	id, err := currentProcessIdentity()
	require(t, err)
	rt := Runtime{
		SchemaVersion: 4,
		UpdatedAt:     now(),
		HeartbeatAt:   now(),
		Service: ServiceState{
			Mode: "scheduler", InstanceID: newID(), PID: id.PID, Status: "running", Claimant: c.Claimant,
			ProcessIdentity: id, ConfigDigest: c.configDigest, Ready: true,
			ControlProtocol: ControlProtocolVersion, Capabilities: []string{"stop_service"},
		},
		Repository: Repository{Path: c.Repository, Remote: c.Remote, Branch: c.Branch, Identity: c.Identity()},
		Runs:       []*Run{},
		Control:    ControlState{DispatchMode: "manual", Revision: 1},
	}
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
	legacyChanged := rt
	legacyChanged.Service.ConfigDigest = strings.Repeat("e", 64)
	write(legacyChanged)
	if got := ConnectService(context.Background(), c); got.State != ConnectionIdentityError || got.CanStop {
		t.Fatalf("legacy digest mismatch without live service locks must remain unbound: %+v", got)
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
	if got := ConnectService(context.Background(), c); got.State != ConnectionConfigChanged || !got.CanStop || got.CanControl {
		t.Fatalf("legacy digest mismatch with both live service locks=%+v", got)
	}
	stopRequest, err := SubmitControl(c, "stop_service", nil, "identity-test")
	require(t, err)
	requestPath := controlRequestPath(c.RunDir, stopRequest.RequestID)
	receipt := ControlReceipt{Version: ControlProtocolVersion, RequestID: stopRequest.RequestID, RequestDigest: rawRequestDigest(requestPath), InstanceID: rt.Service.InstanceID, Action: "stop_service", Status: "applied", RequestedBy: stopRequest.RequestedBy, SubmittedAt: stopRequest.CreatedAt, CompletedAt: ptr(now())}
	receiptBytes, err := json.Marshal(receipt)
	require(t, err)
	receiptPath := controlReceiptPath(c.RunDir, stopRequest.RequestID)
	require(t, os.MkdirAll(filepath.Dir(receiptPath), 0700))
	require(t, AtomicWrite(receiptPath, receiptBytes))
	repoLock.Close()
	runLock.Close()
	repoLock, runLock = nil, nil
	stoppedLegacy := rt
	stoppedLegacy.Service.ConfigDigest = strings.Repeat("e", 64)
	stoppedLegacy.Service.ProcessIdentity = &ProcessIdentity{Kind: "os", PID: 2_000_000_000, Created: "old-process", Executable: "old-executable"}
	stoppedLegacy.Service.PID = stoppedLegacy.Service.ProcessIdentity.PID
	require(t, writeStopObservation(c, &stoppedLegacy, stopRequest.RequestID))
	write(stoppedLegacy)
	if got := ConnectService(context.Background(), c); got.State != ConnectionOffline || !got.CanStart {
		t.Fatalf("stopped legacy service should allow explicit restart after the saved stop proof: %+v", got)
	}

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

type lifecycleParentReport struct {
	Status          string           `json:"status"`
	Error           string           `json:"error,omitempty"`
	TTY             bool             `json:"tty"`
	ParentPID       int              `json:"parent_pid"`
	ParentInJob     bool             `json:"parent_in_job"`
	ParentJobFlags  uint32           `json:"parent_job_limit_flags"`
	ParentIdentity  *ProcessIdentity `json:"parent_identity,omitempty"`
	ConsoleWindow   uint64           `json:"console_window,omitempty"`
	ServiceIdentity *ProcessIdentity `json:"service_identity,omitempty"`
	WorkerIdentity  *ProcessIdentity `json:"worker_identity,omitempty"`
	InstanceID      string           `json:"instance_id,omitempty"`
	StartupID       string           `json:"startup_id,omitempty"`
}

func TestServiceLifecycleParentHelper(t *testing.T) {
	if os.Getenv("STEERLINE_LIFECYCLE_PARENT") != "1" {
		return
	}
	reportPath := os.Getenv("STEERLINE_LIFECYCLE_REPORT")
	stdinInfo, stdinErr := os.Stdin.Stat()
	tty := stdinErr == nil && stdinInfo.Mode()&os.ModeCharDevice != 0
	parentIdentity, identityErr := currentProcessIdentity()
	parentInJob, parentJobFlags, jobErr := currentJobDiagnostics()
	report := lifecycleParentReport{TTY: tty, ParentPID: os.Getpid(), ParentInJob: parentInJob, ParentJobFlags: parentJobFlags, ParentIdentity: parentIdentity, ConsoleWindow: uint64(currentConsoleWindowForTests())}
	writeReport := func() error {
		if reportPath == "" {
			return nil
		}
		b, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return err
		}
		return AtomicWrite(reportPath, append(b, '\n'))
	}
	if os.Getenv("STEERLINE_LIFECYCLE_REQUIRE_TTY") == "1" && !tty {
		report.Status, report.Error = "not-tty", "parent process stdin is not a character device"
		if err := writeReport(); err != nil {
			t.Fatal(err)
		}
		t.Fatal(report.Error)
	}
	if os.Getenv("STEERLINE_LIFECYCLE_REQUIRE_TTY") == "1" && report.ConsoleWindow == 0 {
		report.Status, report.Error = "no-console-window", "parent process has no visible console window"
		if err := writeReport(); err != nil {
			t.Fatal(err)
		}
		t.Fatal(report.Error)
	}
	if identityErr != nil || jobErr != nil {
		report.Status = "parent-probe-failed"
		report.Error = fmt.Sprint(errors.Join(identityErr, jobErr))
		_ = writeReport()
		t.Fatal(report.Error)
	}
	configPath := os.Getenv("STEERLINE_LIFECYCLE_CONFIG")
	executable := os.Getenv("STEERLINE_LIFECYCLE_EXE")
	c, err := LoadConfig(configPath)
	if err != nil {
		report.Status, report.Error = "config-failed", err.Error()
		_ = writeReport()
		t.Fatal(err)
	}
	start, err := StartService(context.Background(), configPath, StartServiceOptions{Executable: executable, WaitTimeout: 20 * time.Second})
	if err != nil || start.Status != "connected" {
		report.Status = "start-failed"
		if err != nil {
			report.Error = err.Error()
		} else {
			report.Error = start.Error
		}
		_ = writeReport()
		t.Fatalf("StartService result=%+v err=%v", start, err)
	}
	rt := start.Connection.Snapshot.Runtime
	if rt.Control.DispatchMode != "manual" {
		t.Fatalf("new service dispatch mode=%q", rt.Control.DispatchMode)
	}
	dbPath := os.Getenv("STEERLINE_LIFECYCLE_DB")
	if dbPath == "" && len(c.BD.Args) != 0 {
		dbPath = c.BD.Args[len(c.BD.Args)-1]
	}
	if db := readFake(dbPath); len(db.Claims) != 0 || len(db.Starts) != 0 {
		t.Fatalf("manual service claimed work before explicit request: %+v", db)
	}
	request, err := SubmitControl(c, "start_task", map[string]string{"task_id": "t-1"}, "lifecycle-test")
	if err != nil {
		report.Status, report.Error = "control-failed", err.Error()
		_ = writeReport()
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	receipt, err := WaitControlReceipt(ctx, c, request.RequestID, 50*time.Millisecond)
	if err != nil || receipt.Status != "applied" {
		report.Status = "control-failed"
		if err != nil {
			report.Error = err.Error()
		} else {
			report.Error = "start_task was not applied"
		}
		_ = writeReport()
		t.Fatalf("start_task receipt=%+v err=%v", receipt, err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		current, e := LoadRuntime(filepath.Join(c.RunDir, c.RuntimeFile))
		if e == nil && current != nil && len(current.Runs) == 1 && current.Runs[0].ExecutionStatus == "running" && current.Runs[0].Process != nil {
			report.Status = "worker-running"
			report.ServiceIdentity = current.Service.ProcessIdentity
			report.WorkerIdentity = current.Runs[0].Process
			report.InstanceID = current.Service.InstanceID
			if current.Service.StartupID != nil {
				report.StartupID = *current.Service.StartupID
			}
			if e = writeReport(); e != nil {
				t.Fatal(e)
			}
			if os.Getenv("STEERLINE_LIFECYCLE_HOLD") == "1" {
				for {
					time.Sleep(time.Second)
				}
			}
			if readyPath := os.Getenv("STEERLINE_LIFECYCLE_READY"); readyPath != "" {
				if err := os.WriteFile(readyPath, []byte("ready"), 0600); err != nil {
					t.Fatal(err)
				}
				// The outer test forcefully terminates this starter with a live worker.
				select {}
			}
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	report.Status, report.Error = "worker-timeout", "explicit worker did not become running"
	_ = writeReport()
	t.Fatal("explicit worker did not become running")
}

func TestDetachedServiceLaunchReportsJobRestriction(t *testing.T) {
	if runtimeGOOSWindows() {
		t.Skip("Windows now uses a visible service window; detached launch remains the Linux path")
	}
	parentInJob, parentFlags, parentJobErr := currentJobDiagnostics()
	t.Logf("launcher parent job: in_job=%t limit_flags=0x%x query_error=%v", parentInJob, parentFlags, parentJobErr)
	if parentInJob {
		childOutput, childErr := jobDiagnosticChildOutput()
		if childErr != nil {
			t.Logf("isolated breakaway diagnostic child: %v\n%s", childErr, childOutput)
		}
		t.Logf("isolated breakaway diagnostic child: %s", strings.TrimSpace(string(childOutput)))
	}
	f := newFixture(t)
	config, configPath := lifecycleConfigFile(t, f, f.c.RunDir)
	result, err := StartService(context.Background(), configPath, StartServiceOptions{Executable: buildServiceLifecycleCLI(t), WaitTimeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status == "connected" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		stopped, stopErr := StopService(ctx, config, "test-cleanup", 8*time.Second)
		if stopErr != nil || stopped.Status != "stopped" {
			t.Fatalf("cleanup after verified detached launch: %+v %v", stopped, stopErr)
		}
		return
	}
	if result.Status != "failed" || !strings.Contains(strings.ToLower(result.Error), "detach") && !strings.Contains(strings.ToLower(result.Error), "job object") {
		t.Fatalf("detached launch did not report a concrete failure: %+v", result)
	}
	t.Logf("detached launch result: %s", result.Error)
	if result.StartupID == "" {
		t.Fatalf("detached launch failure omitted startup attempt: %+v", result)
	}
	record, readErr := readStartupRecord(config, result.StartupID)
	if readErr != nil || record.Status != "failed" || !strings.Contains(strings.ToLower(record.Error), "detach") && !strings.Contains(strings.ToLower(record.Error), "job object") {
		t.Fatalf("detachment failure record=%+v err=%v", record, readErr)
	}
}

func TestDetachedServiceSurvivesStarterExitAndStopsWorker(t *testing.T) {
	if runtimeGOOSWindows() {
		t.Skip("Windows service-window survival is covered by the foreground and visible-window tests")
	}
	if !detachedChildSupported(t) {
		t.Skip("this Windows host keeps child processes in a Job Object and rejects breakaway")
	}
	f := newFixture(t, task("t-1"))
	db := readFake(f.db)
	db.Behavior = "sleep"
	writeFake(f.db, db)
	// The web endpoint supplies PR responses, not Git smart HTTP. Keep remote
	// identity validation, but fetch the fixture's real local bare repository.
	f.c.Git = Command{Executable: f.c.BD.Executable, Args: []string{"--fake", "git-proxy", f.c.Git.Executable}}
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
	parent, err := os.Executable()
	require(t, err)
	readyPath := filepath.Join(t.TempDir(), "starter-ready")
	cmd := exec.Command(parent, "-test.run=^TestServiceLifecycleParentHelper$")
	cmd.Env = append(os.Environ(),
		"STEERLINE_LIFECYCLE_PARENT=1",
		"STEERLINE_LIFECYCLE_CONFIG="+configPath,
		"STEERLINE_LIFECYCLE_EXE="+serviceExe,
		"STEERLINE_LIFECYCLE_DB="+f.db,
		"STEERLINE_LIFECYCLE_READY="+readyPath,
	)
	log, err := os.Create(filepath.Join(t.TempDir(), "starter.log"))
	require(t, err)
	defer log.Close()
	cmd.Stdout, cmd.Stderr = log, log
	require(t, cmd.Start())
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	starterReaped := false
	defer func() {
		if !starterReaped {
			_ = cmd.Process.Kill()
			<-done
		}
	}()
	deadline := time.Now().Add(50 * time.Second)
	for {
		if _, err := os.Stat(readyPath); err == nil {
			break
		}
		select {
		case e := <-done:
			starterReaped = true
			output, _ := os.ReadFile(log.Name())
			t.Fatalf("starter exited before readiness: %v\n%s", e, output)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("starter did not report a running worker")
		}
		time.Sleep(25 * time.Millisecond)
	}
	beforeExit, err := LoadRuntime(filepath.Join(config.RunDir, config.RuntimeFile))
	require(t, err)
	require(t, cmd.Process.Kill())
	<-done
	starterReaped = true
	rt, err := LoadRuntime(filepath.Join(config.RunDir, config.RuntimeFile))
	require(t, err)
	if rt == nil || rt.Service.StartupID == nil || !rt.Service.Ready || rt.Control.DispatchMode != "manual" {
		t.Fatalf("service did not publish manual ready state after starter exit: %+v", rt)
	}
	connection := ConnectService(context.Background(), config)
	if connection.State != ConnectionConnected || !connection.CanControl {
		t.Fatalf("post-parent connection=%+v", connection)
	}
	if len(rt.Runs) != 1 || rt.Runs[0].ExecutionStatus != "running" || rt.Runs[0].Process == nil || probeProcess(rt.Runs[0].Process).State != ProcessMatches {
		t.Fatalf("worker did not remain alive with its original identity: %+v", rt.Runs)
	}
	if *beforeExit.Service.ProcessIdentity != *rt.Service.ProcessIdentity || *beforeExit.Runs[0].Process != *rt.Runs[0].Process {
		t.Fatal("service or worker identity changed after killing starter")
	}
	if got := readFake(f.db).Starts["t-1"]; got != 1 {
		t.Fatalf("worker starts=%d want one", got)
	}
	if logPath, e := startupLogPath(config.RunDir, *rt.Service.StartupID); e != nil {
		t.Fatal(e)
	} else if info, e := os.Stat(logPath); e != nil || info.IsDir() {
		t.Fatalf("service startup log unavailable: %s (%v)", logPath, e)
	}
	serviceIdentity := rt.Service.ProcessIdentity
	workerIdentity := rt.Runs[0].Process
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	stopped, err := StopService(ctx, config, "lifecycle-test", 12*time.Second)
	if err != nil || stopped.Status != "stopped" || !stopped.ProcessExited || !stopped.LocksReleased || stopped.Receipt == nil || stopped.Receipt.Status != "applied" {
		t.Fatalf("StopService result=%+v err=%v", stopped, err)
	}
	if probe := probeProcess(serviceIdentity); probe.State != ProcessAbsent && probe.State != ProcessReused {
		t.Fatalf("service process survived explicit stop: %+v", probe)
	}
	if probe := probeProcess(workerIdentity); probe.State != ProcessAbsent && probe.State != ProcessReused {
		t.Fatalf("worker process survived explicit stop: %+v", probe)
	}
	if rt, err = LoadRuntime(filepath.Join(config.RunDir, config.RuntimeFile)); err != nil || rt.Service.Status != "stopped" {
		t.Fatalf("stopped runtime=%+v err=%v", rt, err)
	}
}

func TestStartServiceSerializesSameConfigAndRepositoryLockRejectsOtherRunDir(t *testing.T) {
	if runtimeGOOSWindows() {
		t.Skip("Windows service-window uniqueness is verified through service locks")
	}
	if !detachedChildSupported(t) {
		t.Skip("this Windows host keeps child processes in a Job Object and rejects breakaway")
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
	otherRun := filepath.Join(filepath.Dir(config.RunDir), "competing-runtime")
	_, otherConfigPath := lifecycleConfigFile(t, f, otherRun)
	competing, err := StartService(context.Background(), otherConfigPath, opts)
	if err != nil || competing.Status != "failed" || !strings.Contains(competing.Error, "repository already locked") {
		t.Fatalf("different run_dir did not report repository lock conflict: result=%+v err=%v", competing, err)
	}
	if competing.StartupID == "" || competing.LogPath == "" {
		t.Fatalf("lock failure lacks startup identity/log: %+v", competing)
	}
	log, err := os.ReadFile(competing.LogPath)
	require(t, err)
	if !strings.Contains(string(log), "repository already locked") {
		t.Fatalf("lock conflict missing from startup log: %s", log)
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
		f.c.Remote = "web"
		f.c.StartSkill.Path = "missing-start.md"
		config, configPath := lifecycleConfigFile(t, f, f.c.RunDir)
		result, err := StartService(context.Background(), configPath, StartServiceOptions{Executable: buildServiceLifecycleCLI(t), WaitTimeout: 15 * time.Second})
		if err != nil || result.Status != "failed" || !strings.Contains(result.Error, "missing skill") {
			t.Fatalf("preflight result=%+v err=%v", result, err)
		}
		if result.StartupID == "" || result.LogPath == "" {
			t.Fatalf("preflight failure lacks startup identity/log: %+v", result)
		}
		record, readErr := readStartupRecord(config, result.StartupID)
		if readErr != nil || record.Status != "failed" || !strings.Contains(record.Error, "missing skill") {
			t.Fatalf("preflight startup record=%+v err=%v", record, readErr)
		}
		log, readErr := os.ReadFile(result.LogPath)
		require(t, readErr)
		if !strings.Contains(string(log), "missing skill") {
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
		db := readFake(f.db)
		db.Behavior = "slow-context"
		writeFake(f.db, db)
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

func TestRunCommandRecordsPreflightFailureAndLog(t *testing.T) {
	f := newFixture(t)
	config, configPath := lifecycleConfigFile(t, f, f.c.RunDir)
	config.StartSkill.Path = "missing-start.md"
	data, err := json.Marshal(config)
	require(t, err)
	require(t, os.WriteFile(configPath, data, 0600))
	config, err = LoadConfig(configPath)
	require(t, err)
	require(t, config.ResolveIdentity(context.Background()))
	config.Repository, err = resolveRepositoryPath(context.Background(), config)
	require(t, err)
	config.RunDir, err = expectedRunDir(config)
	require(t, err)
	id := newID()
	_, logPath, err := createStartupAttempt(config, id, true)
	require(t, err)
	require(t, writeStartupRecord(config, initialStartupRecord(config, id)))
	logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0600)
	require(t, err)
	stdin, err := os.Open(os.DevNull)
	require(t, err)
	cli := buildServiceLifecycleCLI(t)
	cmd := exec.Command(cli, "run", "-config", configPath, "-dispatch", "manual", "-console", "off", "-startup-id", id)
	cmd.Dir = config.Repository
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, logFile, logFile
	runErr := cmd.Run()
	require(t, logFile.Close())
	require(t, stdin.Close())
	if runErr == nil {
		t.Fatal("preflight failure returned success")
	}
	record, err := readStartupRecord(config, id)
	if err != nil || record.Status != "failed" || !strings.Contains(record.Error, "missing skill") {
		t.Fatalf("preflight startup record=%+v err=%v", record, err)
	}
	log, err := os.ReadFile(logPath)
	require(t, err)
	if !strings.Contains(string(log), "missing skill") {
		t.Fatalf("preflight error missing from startup log: %s", log)
	}
}

func lifecycleConfigFile(t *testing.T, f *fixture, runDir string) (Config, string) {
	t.Helper()
	c := f.c
	c.Remote = "web"
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
	name := "steerline-lifecycle"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(t.TempDir(), name)
	cmd := exec.Command("go", "build", "-o", path, "../../cmd/steerline")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build service lifecycle CLI: %v\n%s", err, output)
	}
	return path
}

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
