//go:build windows

package steerline

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestWindowsReplaceFailurePreservesSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.json")
	require(t, AtomicWrite(path, []byte(`{"version":1}`)))
	p, e := windows.UTF16PtrFromString(path)
	require(t, e)
	h, e := windows.CreateFile(p, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	require(t, e)
	e = AtomicWrite(path, []byte(`{"version":2}`))
	windows.CloseHandle(h)
	if e == nil {
		t.Fatal("expected sharing violation")
	}
	b, e := os.ReadFile(path)
	require(t, e)
	if string(b) != `{"version":1}` {
		t.Fatal("failed replacement destroyed prior snapshot")
	}
	require(t, AtomicWrite(path, []byte(`{"version":2}`)))
}

func TestJobDiagnosticChildHelper(t *testing.T) {
	if os.Getenv("STEERLINE_JOB_DIAGNOSTIC_CHILD") != "1" {
		return
	}
	inJob, flags, err := currentJobDiagnostics()
	if err != nil {
		fmt.Printf("child_job_query_error=%v\n", err)
		return
	}
	fmt.Printf("child_in_job=%t child_job_limit_flags=0x%x\n", inJob, flags)
}

func jobDiagnosticChildOutput() ([]byte, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(exe, "-test.run=^TestJobDiagnosticChildHelper$")
	cmd.Env = append(os.Environ(), "STEERLINE_JOB_DIAGNOSTIC_CHILD=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_BREAKAWAY_FROM_JOB | windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP,
		HideWindow:    true,
	}
	return cmd.CombinedOutput()
}

func TestLifecycleReviewExitedProcessWithRetainedHandle(t *testing.T) {
	exe, err := os.Executable()
	require(t, err)
	cmd := exec.Command(exe, "--fake", "sleep")
	require(t, cmd.Start())
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, uint32(cmd.Process.Pid))
	require(t, err)
	defer windows.CloseHandle(handle)
	identity, err := identityFromHandle(handle, cmd.Process.Pid)
	require(t, err)
	require(t, cmd.Process.Kill())
	_, err = windows.WaitForSingleObject(handle, 5000)
	require(t, err)
	if probe := probeProcess(&identity); probe.State != ProcessAbsent {
		t.Fatalf("retained exited process handle reported alive: %+v", probe)
	}
}

func currentConsoleWindowForTests() uintptr {
	hwnd, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetConsoleWindow").Call()
	return hwnd
}

func windowExistsForTests(hwnd uint64) bool {
	if hwnd == 0 {
		return false
	}
	result, _, _ := windows.NewLazySystemDLL("user32.dll").NewProc("IsWindow").Call(uintptr(hwnd))
	return result != 0
}

func windowVisibleForTests(hwnd uint64) bool {
	if hwnd == 0 {
		return false
	}
	result, _, _ := windows.NewLazySystemDLL("user32.dll").NewProc("IsWindowVisible").Call(uintptr(hwnd))
	return result != 0
}

func closeConsoleWindowForTests(hwnd uint64) error {
	if hwnd == 0 {
		return fmt.Errorf("no console window handle")
	}
	result, _, callErr := windows.NewLazySystemDLL("user32.dll").NewProc("PostMessageW").Call(uintptr(hwnd), 0x0010, 0, 0)
	if result == 0 {
		return callErr
	}
	return nil
}

func TestWindowsForegroundViewerHelper(t *testing.T) {
	configPath := os.Getenv("STEERLINE_FOREGROUND_VIEWER_CONFIG")
	if configPath == "" {
		return
	}
	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	connection := ConnectService(context.Background(), config)
	if connection.State != ConnectionConnected {
		t.Fatalf("viewer could not connect: %+v", connection)
	}
	if err := os.WriteFile(os.Getenv("STEERLINE_FOREGROUND_VIEWER_READY"), []byte("connected"), 0600); err != nil {
		t.Fatal(err)
	}
	for {
		time.Sleep(time.Second)
	}
}

func TestWindowsStartServiceRequiresVisibleWindow(t *testing.T) {
	f := newFixture(t)
	_, configPath := lifecycleConfigFile(t, f, f.c.RunDir)
	home := t.TempDir()
	t.Setenv("STEERLINE_HOME", home)
	_, err := InstallLaunchers()
	require(t, err)
	result, err := StartService(context.Background(), configPath, StartServiceOptions{Executable: f.c.BD.Executable})
	require(t, err)
	if result.Status != "failed" || !strings.Contains(result.Error, "steerline install") {
		t.Fatalf("missing installed binary was not explained: %+v", result)
	}
	require(t, os.MkdirAll(filepath.Join(home, "bin"), 0700))
	require(t, os.WriteFile(filepath.Join(home, "bin", "steerline.exe"), []byte("test-only installed path"), 0600))
	result, err = StartService(context.Background(), configPath, StartServiceOptions{Executable: f.c.BD.Executable})
	require(t, err)
	configAbsolute, err := filepath.Abs(configPath)
	require(t, err)
	if result.Status != "window-required" || result.LauncherPath != filepath.Join(home, "service-window.bat") ||
		result.ConfigPath != configAbsolute || result.StartupID != "" || result.Connection.State != ConnectionNoRecord {
		t.Fatalf("Windows StartService did not return the fixed visible-window entry: %+v", result)
	}
	if _, err := os.Stat(filepath.Join(f.c.RunDir, "service")); !os.IsNotExist(err) {
		t.Fatalf("Windows StartService wrote service state without opening a window: %v", err)
	}
}

func TestWindowsServiceWindowMissingConfigIsLiteral(t *testing.T) {
	installDir := filepath.Join(t.TempDir(), "home with spaces")
	t.Setenv("STEERLINE_HOME", installDir)
	_, err := InstallLaunchers()
	require(t, err)
	require(t, os.MkdirAll(filepath.Join(installDir, "bin"), 0700))
	require(t, os.WriteFile(filepath.Join(installDir, "bin", "steerline.exe"), []byte("not executed"), 0600))
	launcher := filepath.Join(installDir, "service-window.bat")
	missingConfig := filepath.Join(t.TempDir(), "missing & echo unexpected-launcher-command")
	cmd := exec.Command("cmd.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CmdLine: fmt.Sprintf("cmd.exe /d /c \"\"%s\" \"%s\"\"", launcher, missingConfig),
	}
	cmd.Stdin = strings.NewReader("\n")
	output, runErr := cmd.CombinedOutput()
	if runErr == nil {
		t.Fatal("missing configuration returned success")
	}
	for _, line := range strings.Split(string(output), "\n") {
		if strings.TrimSpace(line) == "unexpected-launcher-command" {
			t.Fatalf("configuration path was executed as a command:\n%s", output)
		}
	}
	if !strings.Contains(string(output), missingConfig) {
		t.Fatalf("missing configuration path was not printed literally:\n%s", output)
	}
}

func TestWindowsServiceWindowRejectsMissingConsole(t *testing.T) {
	f := newFixture(t)
	config, configPath := lifecycleConfigFile(t, f, f.c.RunDir)
	serviceExe := buildServiceLifecycleCLI(t)
	cmd := exec.Command(serviceExe, "run", "-config", configPath, "-dispatch", "manual", "-console", "summary", "-service-window")
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW, HideWindow: true}
	output, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "requires a visible Windows console") {
		t.Fatalf("service-window accepted a hidden process: %v\n%s", err, output)
	}
	if _, err := os.Stat(filepath.Join(config.RunDir, "service")); !os.IsNotExist(err) {
		t.Fatalf("hidden service-window wrote startup state: %v", err)
	}
}

func TestWindowsPendingStartupPreservesMismatchClassification(t *testing.T) {
	for _, kind := range []string{"config", "repository-path"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixture(t)
			c, configPath := lifecycleConfigFile(t, f, f.c.RunDir)
			identity, err := currentProcessIdentity()
			require(t, err)
			record := reviewStartup(t, c, "starting", identity)
			expected := ConnectionConfigChanged
			if kind == "config" {
				record.ConfigDigest = strings.Repeat("f", 64)
			} else {
				record.RepositoryPath = filepath.Join(t.TempDir(), "another-worktree")
				expected = ConnectionIdentityError
			}
			require(t, writeStartupRecord(c, record))
			result, err := StartService(context.Background(), configPath, StartServiceOptions{})
			require(t, err)
			if result.Status != "pending" || result.Connection.State != expected ||
				result.Connection.Process.State != ProcessMatches || result.Connection.CanStart ||
				result.Connection.StartupID != record.StartupID {
				t.Fatalf("pending %s mismatch lost known identity or classification: %+v", kind, result)
			}
		})
	}
}

func windowsForegroundLifecycleConfig(t *testing.T, f *fixture) (Config, string) {
	t.Helper()
	config, configPath := lifecycleConfigFile(t, f, f.c.RunDir)
	config.Git = Command{Executable: f.c.BD.Executable, Args: []string{"--fake", "git-proxy", f.c.Git.Executable}}
	configData, err := json.Marshal(config)
	require(t, err)
	require(t, os.WriteFile(configPath, configData, 0600))
	config, err = LoadConfig(configPath)
	require(t, err)
	require(t, config.ResolveIdentity(context.Background()))
	return config, configPath
}

func TestWindowsForegroundServiceWindowLifecycle(t *testing.T) {
	f := newFixture(t, task("t-1"))
	db := readFake(f.db)
	db.Behavior = "sleep"
	writeFake(f.db, db)
	config, configPath := windowsForegroundLifecycleConfig(t, f)
	serviceExe := buildServiceLifecycleCLI(t)
	serviceLogPath := filepath.Join(t.TempDir(), "foreground-service.log")
	serviceLog, err := os.Create(serviceLogPath)
	require(t, err)
	defer serviceLog.Close()
	service := exec.Command(serviceExe, "run", "-config", configPath, "-dispatch", "manual", "-console", "summary")
	service.Stdout, service.Stderr = serviceLog, serviceLog
	require(t, service.Start())
	reaped := false
	defer func() {
		if !reaped {
			_ = service.Process.Kill()
			_ = service.Wait()
		}
	}()
	deadline := time.Now().Add(25 * time.Second)
	var connection ServiceConnection
	for time.Now().Before(deadline) {
		connection = ConnectService(context.Background(), config)
		if connection.State == ConnectionConnected {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if connection.State != ConnectionConnected {
		output, _ := os.ReadFile(serviceLogPath)
		t.Fatalf("foreground service did not connect: %+v\n%s", connection, output)
	}
	connected, err := StartService(context.Background(), configPath, StartServiceOptions{})
	require(t, err)
	if connected.Status != "connected" || connected.Connection.Snapshot.Runtime.Service.InstanceID != connection.Snapshot.Runtime.Service.InstanceID {
		t.Fatalf("StartService did not connect the existing foreground service: %+v", connected)
	}
	otherConfig := config
	otherConfig.RunDir = filepath.Join(t.TempDir(), "competing-runtime")
	otherConfigPath := filepath.Join(t.TempDir(), "other-service-config.json")
	otherConfigData, err := json.Marshal(otherConfig)
	require(t, err)
	require(t, os.WriteFile(otherConfigPath, otherConfigData, 0600))
	for _, competingConfig := range []string{configPath, otherConfigPath} {
		competing := exec.Command(serviceExe, "run", "-config", competingConfig, "-dispatch", "manual", "-console", "summary")
		output, startErr := competing.CombinedOutput()
		if startErr == nil || !strings.Contains(strings.ToLower(string(output)), "lock") {
			t.Fatalf("second foreground service did not report the repository lock for %s: %v\n%s", competingConfig, startErr, output)
		}
	}
	if again := ConnectService(context.Background(), config); again.State != ConnectionConnected ||
		again.Snapshot.Runtime.Service.InstanceID != connection.Snapshot.Runtime.Service.InstanceID {
		t.Fatalf("competing service changed the foreground winner: %+v", again)
	}
	runtimeBefore := connection.Snapshot.Runtime
	if runtimeBefore.Service.StartupID == nil || runtimeBefore.Control.DispatchMode != "manual" || !runtimeBefore.Service.Ready {
		t.Fatalf("foreground startup missed identity/manual readiness: %+v", runtimeBefore.Service)
	}
	if db := readFake(f.db); len(db.Claims) != 0 || len(db.Starts) != 0 {
		t.Fatalf("manual foreground service claimed work before an explicit request: %+v", db)
	}
	request, err := SubmitControl(config, "start_task", map[string]string{"task_id": "t-1"}, "foreground-window-test")
	require(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	receipt, err := WaitControlReceipt(ctx, config, request.RequestID, 50*time.Millisecond)
	if err != nil || receipt.Status != "applied" {
		t.Fatalf("start_task receipt=%+v err=%v", receipt, err)
	}
	var running *Runtime
	for time.Now().Before(deadline.Add(10 * time.Second)) {
		running, err = LoadRuntime(filepath.Join(config.RunDir, config.RuntimeFile))
		if err == nil && running != nil && len(running.Runs) == 1 && running.Runs[0].ExecutionStatus == "running" && running.Runs[0].Process != nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil || running == nil || len(running.Runs) != 1 || running.Runs[0].Process == nil {
		output, _ := os.ReadFile(serviceLogPath)
		t.Fatalf("foreground worker did not run: runtime=%+v err=%v\n%s", running, err, output)
	}
	serviceIdentity := *running.Service.ProcessIdentity
	workerIdentity := *running.Runs[0].Process
	if probeProcess(&serviceIdentity).State != ProcessMatches || probeProcess(&workerIdentity).State != ProcessMatches {
		t.Fatalf("foreground identities are not live: service=%+v worker=%+v", serviceIdentity, workerIdentity)
	}
	if starts := readFake(f.db).Starts["t-1"]; starts != 1 {
		t.Fatalf("explicit worker starts=%d, want 1", starts)
	}
	startupLog, err := startupLogPath(config.RunDir, *running.Service.StartupID)
	require(t, err)
	startupOutput, err := os.ReadFile(startupLog)
	require(t, err)
	if !strings.Contains(string(startupOutput), "service ready") || !strings.Contains(string(startupOutput), "worker started") {
		t.Fatalf("foreground summary missing from startup log:\n%s", startupOutput)
	}
	testExe, err := os.Executable()
	require(t, err)
	viewerReady := filepath.Join(t.TempDir(), "viewer-ready")
	viewerLog, err := os.Create(filepath.Join(t.TempDir(), "viewer.log"))
	require(t, err)
	defer viewerLog.Close()
	viewer := exec.Command(testExe, "-test.run=^TestWindowsForegroundViewerHelper$")
	viewer.Env = append(os.Environ(), "STEERLINE_FOREGROUND_VIEWER_CONFIG="+configPath, "STEERLINE_FOREGROUND_VIEWER_READY="+viewerReady)
	viewer.Stdout, viewer.Stderr = viewerLog, viewerLog
	require(t, viewer.Start())
	viewerReaped := false
	defer func() {
		if !viewerReaped {
			_ = viewer.Process.Kill()
			_ = viewer.Wait()
		}
	}()
	viewerDeadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(viewerDeadline) {
		if _, err := os.Stat(viewerReady); err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if _, err := os.Stat(viewerReady); err != nil {
		output, _ := os.ReadFile(viewerLog.Name())
		t.Fatalf("independent viewer did not connect: %v\n%s", err, output)
	}
	require(t, viewer.Process.Kill())
	_ = viewer.Wait()
	viewerReaped = true
	reconnected := ConnectService(context.Background(), config)
	if reconnected.State != ConnectionConnected || reconnected.Snapshot.Runtime.Service.InstanceID != running.Service.InstanceID ||
		probeProcess(&serviceIdentity).State != ProcessMatches || probeProcess(&workerIdentity).State != ProcessMatches {
		t.Fatalf("viewer exit changed foreground service or worker: %+v", reconnected)
	}
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer stopCancel()
	stopped, err := StopService(stopCtx, config, "foreground-window-test", 15*time.Second)
	if err != nil || stopped.Status != "stopped" || !stopped.ProcessExited || !stopped.LocksReleased || stopped.Receipt == nil || stopped.Receipt.Status != "applied" {
		t.Fatalf("foreground stop result=%+v err=%v", stopped, err)
	}
	_ = service.Wait()
	reaped = true
	if probeProcess(&serviceIdentity).State == ProcessMatches || probeProcess(&workerIdentity).State == ProcessMatches {
		t.Fatalf("foreground service or worker survived explicit stop: service=%+v worker=%+v", probeProcess(&serviceIdentity), probeProcess(&workerIdentity))
	}
}

type windowsServiceWindowProbe struct {
	TTY           bool
	ConsoleWindow uint64
}

func TestWindowsServiceWindowProbeHelper(t *testing.T) {
	reportPath := os.Getenv("STEERLINE_WINDOW_PROBE_PATH")
	if reportPath == "" {
		return
	}
	info, err := os.Stdin.Stat()
	require(t, err)
	report := windowsServiceWindowProbe{
		TTY:           info.Mode()&os.ModeCharDevice != 0,
		ConsoleWindow: uint64(currentConsoleWindowForTests()),
	}
	data, err := json.Marshal(report)
	require(t, err)
	require(t, AtomicWrite(reportPath, append(data, '\n')))
}

func TestWindowsConsoleCtrlCHelper(t *testing.T) {
	target := os.Getenv("STEERLINE_CTRL_C_TARGET")
	if target == "" {
		return
	}
	pid, err := strconv.Atoi(target)
	require(t, err)
	kernel32 := windows.NewLazySystemDLL("kernel32.dll")
	_, _, _ = kernel32.NewProc("FreeConsole").Call()
	if ok, _, callErr := kernel32.NewProc("AttachConsole").Call(uintptr(pid)); ok == 0 {
		t.Fatalf("attach to service console %d: %v", pid, callErr)
	}
	defer kernel32.NewProc("FreeConsole").Call()
	setHandler := kernel32.NewProc("SetConsoleCtrlHandler")
	if ok, _, callErr := setHandler.Call(0, 1); ok == 0 {
		t.Fatalf("ignore Ctrl+C in signal helper: %v", callErr)
	}
	defer setHandler.Call(0, 0)
	if ok, _, callErr := kernel32.NewProc("GenerateConsoleCtrlEvent").Call(0, 0); ok == 0 {
		t.Fatalf("send Ctrl+C to service console: %v", callErr)
	}
	time.Sleep(300 * time.Millisecond)
}

func launchWindowsVisibleServiceWindow(t *testing.T, config Config, configPath string) (uint64, string, ServiceConnection) {
	t.Helper()
	serviceExe := buildServiceLifecycleCLI(t)
	home := t.TempDir()
	require(t, os.MkdirAll(filepath.Join(home, "bin"), 0700))
	data, err := os.ReadFile(serviceExe)
	require(t, err)
	require(t, os.WriteFile(filepath.Join(home, "bin", "steerline.exe"), data, 0700))
	t.Setenv("STEERLINE_HOME", home)
	_, err = InstallLaunchers()
	require(t, err)
	testExe, err := os.Executable()
	require(t, err)
	probePath := filepath.Join(t.TempDir(), "console-probe.json")
	scriptPath := filepath.Join(t.TempDir(), "open-service-window.cmd")
	script := fmt.Sprintf("@echo off\r\nchcp 65001 >nul 2>&1\r\nset \"STEERLINE_WINDOW_PROBE_PATH=%s\"\r\n\"%s\" -test.run=TestWindowsServiceWindowProbeHelper >nul 2>&1\r\ncall \"%s\" \"%s\"\r\n", probePath, testExe, filepath.Join(home, "service-window.bat"), configPath)
	require(t, os.WriteFile(scriptPath, []byte(script), 0600))
	taskName := "SteerLineWindowAcceptance-" + newID()
	var window uint64
	t.Cleanup(func() {
		connection := ConnectService(context.Background(), config)
		if connection.CanStop {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			_, _ = StopService(ctx, config, "window-test-cleanup", 12*time.Second)
			cancel()
		}
		if window != 0 && windowExistsForTests(window) {
			_ = closeConsoleWindowForTests(window)
		}
		_ = exec.Command("schtasks.exe", "/End", "/TN", taskName).Run()
		_ = exec.Command("schtasks.exe", "/Delete", "/TN", taskName, "/F").Run()
	})
	cmdExe, err := exec.LookPath("cmd.exe")
	require(t, err)
	launchDate := time.Date(2099, time.December, 31, 23, 59, 0, 0, time.Local)
	action := fmt.Sprintf("\"%s\" /d /c \"%s\"", cmdExe, scriptPath)
	createOutput, err := exec.Command("schtasks.exe", "/Create", "/TN", taskName, "/TR", action, "/SC", "ONCE", "/SD", launchDate.Format("2006/01/02"), "/ST", launchDate.Format("15:04"), "/IT", "/F").CombinedOutput()
	if err != nil {
		t.Skipf("cannot create temporary interactive service-window task: %v (%s)", err, createOutput)
	}
	runOutput, err := exec.Command("schtasks.exe", "/Run", "/TN", taskName).CombinedOutput()
	if err != nil {
		t.Fatalf("run temporary service-window task: %v (%s)", err, runOutput)
	}
	deadline := time.Now().Add(35 * time.Second)
	var probe windowsServiceWindowProbe
	for time.Now().Before(deadline) {
		if b, e := os.ReadFile(probePath); e == nil && json.Unmarshal(b, &probe) == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	window = probe.ConsoleWindow
	if !probe.TTY || window == 0 || !windowExistsForTests(window) || !windowVisibleForTests(window) {
		t.Fatalf("launcher did not open a real visible TTY: %+v", probe)
	}
	var connection ServiceConnection
	for time.Now().Before(deadline) {
		connection = ConnectService(context.Background(), config)
		if connection.State == ConnectionConnected {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if connection.State != ConnectionConnected {
		taskOutput, _ := exec.Command("schtasks.exe", "/Query", "/TN", taskName, "/FO", "LIST", "/V").CombinedOutput()
		t.Fatalf("visible service window did not connect: %+v\ntask:\n%s", connection, taskOutput)
	}
	return window, testExe, connection
}

func TestWindowsVisibleServiceWindowAcceptance(t *testing.T) {
	if os.Getenv("STEERLINE_RUN_WINDOW_ACCEPTANCE") != "1" {
		return
	}
	f := newFixture(t, task("t-1"))
	db := readFake(f.db)
	db.Behavior = "sleep"
	writeFake(f.db, db)
	config, configPath := windowsForegroundLifecycleConfig(t, f)
	window, testExe, connection := launchWindowsVisibleServiceWindow(t, config, configPath)
	var err error
	rt := connection.Snapshot.Runtime
	if rt.Service.StartupID == nil || !rt.Service.Ready || rt.Control.DispatchMode != "manual" {
		t.Fatalf("visible service did not start manual and ready: %+v", rt.Service)
	}
	if db := readFake(f.db); len(db.Claims) != 0 || len(db.Starts) != 0 {
		t.Fatalf("visible manual service claimed work before explicit action: %+v", db)
	}
	request, err := SubmitControl(config, "start_task", map[string]string{"task_id": "t-1"}, "visible-window-test")
	require(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	receipt, err := WaitControlReceipt(ctx, config, request.RequestID, 50*time.Millisecond)
	if err != nil || receipt.Status != "applied" {
		t.Fatalf("visible start_task receipt=%+v err=%v", receipt, err)
	}
	var running *Runtime
	workerDeadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(workerDeadline) {
		running, err = LoadRuntime(filepath.Join(config.RunDir, config.RuntimeFile))
		if err == nil && running != nil && len(running.Runs) == 1 && running.Runs[0].ExecutionStatus == "running" && running.Runs[0].Process != nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil || running == nil || len(running.Runs) != 1 || running.Runs[0].Process == nil {
		t.Fatalf("visible window did not start worker: runtime=%+v err=%v", running, err)
	}
	serviceIdentity := *running.Service.ProcessIdentity
	workerIdentity := *running.Runs[0].Process
	startupLog, err := startupLogPath(config.RunDir, *running.Service.StartupID)
	require(t, err)
	startupOutput, err := os.ReadFile(startupLog)
	require(t, err)
	if !strings.Contains(string(startupOutput), "service ready") || !strings.Contains(string(startupOutput), "worker started") {
		t.Fatalf("visible window summary missing from startup log:\n%s", startupOutput)
	}
	viewerReady := filepath.Join(t.TempDir(), "viewer-ready")
	viewerLog, err := os.Create(filepath.Join(t.TempDir(), "viewer.log"))
	require(t, err)
	defer viewerLog.Close()
	viewer := exec.Command(testExe, "-test.run=^TestWindowsForegroundViewerHelper$")
	viewer.Env = append(os.Environ(), "STEERLINE_FOREGROUND_VIEWER_CONFIG="+configPath, "STEERLINE_FOREGROUND_VIEWER_READY="+viewerReady)
	viewer.Stdout, viewer.Stderr = viewerLog, viewerLog
	require(t, viewer.Start())
	viewerReaped := false
	defer func() {
		if !viewerReaped {
			_ = viewer.Process.Kill()
			_ = viewer.Wait()
		}
	}()
	viewerDeadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(viewerDeadline) {
		if _, e := os.Stat(viewerReady); e == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if _, err := os.Stat(viewerReady); err != nil {
		output, _ := os.ReadFile(viewerLog.Name())
		t.Fatalf("independent viewer did not connect to visible window: %v\n%s", err, output)
	}
	require(t, viewer.Process.Kill())
	_ = viewer.Wait()
	viewerReaped = true
	reconnected := ConnectService(context.Background(), config)
	if reconnected.State != ConnectionConnected || reconnected.Snapshot.Runtime.Service.InstanceID != running.Service.InstanceID ||
		probeProcess(&serviceIdentity).State != ProcessMatches || probeProcess(&workerIdentity).State != ProcessMatches {
		t.Fatalf("viewer exit changed visible service or worker: %+v", reconnected)
	}
	signal := exec.Command(testExe, "-test.run=^TestWindowsConsoleCtrlCHelper$")
	signal.Env = append(os.Environ(), "STEERLINE_CTRL_C_TARGET="+strconv.Itoa(serviceIdentity.PID))
	if output, err := signal.CombinedOutput(); err != nil {
		t.Fatalf("send Ctrl+C to visible service window: %v\n%s", err, output)
	}
	stopDeadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(stopDeadline) &&
		(probeProcess(&serviceIdentity).State == ProcessMatches || probeProcess(&workerIdentity).State == ProcessMatches) {
		time.Sleep(100 * time.Millisecond)
	}
	if probeProcess(&serviceIdentity).State == ProcessMatches || probeProcess(&workerIdentity).State == ProcessMatches {
		t.Fatalf("visible service or worker survived Ctrl+C: service=%+v worker=%+v", probeProcess(&serviceIdentity), probeProcess(&workerIdentity))
	}
	repoLock, err := LockRepository(config.Identity())
	require(t, err)
	repoLock.Close()
	runLock, err := LockRepository(config.RunDir)
	require(t, err)
	runLock.Close()
	if observed := ConnectService(context.Background(), config); observed.State != ConnectionOffline {
		t.Fatalf("Ctrl+C did not leave the service offline: %+v", observed)
	}
	if windowExistsForTests(window) {
		require(t, closeConsoleWindowForTests(window))
	}
}

func TestWindowsVisibleServiceWindowCloseAndRestart(t *testing.T) {
	if os.Getenv("STEERLINE_RUN_WINDOW_ACCEPTANCE") != "1" {
		return
	}
	f := newFixture(t, task("t-1"))
	db := readFake(f.db)
	db.Behavior = "sleep"
	writeFake(f.db, db)
	config, configPath := windowsForegroundLifecycleConfig(t, f)
	window, _, connected := launchWindowsVisibleServiceWindow(t, config, configPath)
	request, err := SubmitControl(config, "start_task", map[string]string{"task_id": "t-1"}, "visible-close-test")
	require(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	receipt, err := WaitControlReceipt(ctx, config, request.RequestID, 50*time.Millisecond)
	if err != nil || receipt.Status != "applied" {
		t.Fatalf("worker did not start before window close: receipt=%+v err=%v", receipt, err)
	}
	var running *Runtime
	workerDeadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(workerDeadline) {
		running, err = LoadRuntime(filepath.Join(config.RunDir, config.RuntimeFile))
		if err == nil && running != nil && len(running.Runs) == 1 && running.Runs[0].Process != nil &&
			running.Runs[0].ExecutionStatus == "running" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil || running == nil || len(running.Runs) != 1 || running.Runs[0].Process == nil {
		t.Fatalf("worker did not become running before window close: runtime=%+v err=%v", running, err)
	}
	serviceID := *running.Service.ProcessIdentity
	workerID := *running.Runs[0].Process
	require(t, closeConsoleWindowForTests(window))
	exitDeadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(exitDeadline) {
		if !windowExistsForTests(window) && probeProcess(&serviceID).State != ProcessMatches &&
			probeProcess(&workerID).State != ProcessMatches {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if windowExistsForTests(window) || probeProcess(&serviceID).State == ProcessMatches ||
		probeProcess(&workerID).State == ProcessMatches {
		t.Fatalf("closing service window left process or worker alive: window=%t service=%+v worker=%+v",
			windowExistsForTests(window), probeProcess(&serviceID), probeProcess(&workerID))
	}
	repoLock, err := LockRepository(config.Identity())
	require(t, err)
	repoLock.Close()
	runLock, err := LockRepository(config.RunDir)
	require(t, err)
	runLock.Close()
	if observed := ConnectService(context.Background(), config); observed.State != ConnectionOffline {
		t.Fatalf("closed service window was not classified offline: %+v", observed)
	}
	newWindow, _, restarted := launchWindowsVisibleServiceWindow(t, config, configPath)
	if restarted.Snapshot.Runtime.Service.InstanceID == connected.Snapshot.Runtime.Service.InstanceID ||
		restarted.Snapshot.Runtime.Control.DispatchMode != "manual" {
		t.Fatalf("reopened service window did not create a manual new instance: %+v", restarted)
	}
	if starts := readFake(f.db).Starts["t-1"]; starts != 1 {
		t.Fatalf("reopened window duplicated old worker: starts=%d", starts)
	}
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer stopCancel()
	stopped, err := StopService(stopCtx, config, "visible-close-restart-test", 15*time.Second)
	if err != nil || stopped.Status != "stopped" || !stopped.ProcessExited || !stopped.LocksReleased ||
		stopped.Receipt == nil || stopped.Receipt.Status != "applied" {
		t.Fatalf("reopened service window did not stop: %+v err=%v", stopped, err)
	}
	require(t, closeConsoleWindowForTests(newWindow))
}

func TestWindowsConcurrentVisibleServiceWindows(t *testing.T) {
	if os.Getenv("STEERLINE_RUN_WINDOW_ACCEPTANCE") != "1" {
		return
	}
	for _, differentRunDir := range []bool{false, true} {
		name := "same-config"
		if differentRunDir {
			name = "same-repository-different-run-dir"
		}
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			config, configPath := windowsForegroundLifecycleConfig(t, f)
			otherConfig, otherPath := config, configPath
			if differentRunDir {
				otherConfig.RunDir = filepath.Join(t.TempDir(), "other-runtime")
				otherPath = filepath.Join(t.TempDir(), "other-config.json")
				data, err := json.Marshal(otherConfig)
				require(t, err)
				require(t, os.WriteFile(otherPath, data, 0600))
				otherConfig, err = LoadConfig(otherPath)
				require(t, err)
				require(t, otherConfig.ResolveIdentity(context.Background()))
			}
			serviceExe := buildServiceLifecycleCLI(t)
			paths := []string{configPath, otherPath}
			commands := make([]*exec.Cmd, 2)
			logs := make([]string, 2)
			reaped := make([]bool, 2)
			for i := range commands {
				logs[i] = filepath.Join(t.TempDir(), fmt.Sprintf("service-%d.log", i))
				logFile, err := os.Create(logs[i])
				require(t, err)
				defer logFile.Close()
				commands[i] = exec.Command(serviceExe, "run", "-config", paths[i], "-dispatch", "manual", "-console", "summary", "-service-window")
				commands[i].SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_CONSOLE}
				commands[i].Stdout, commands[i].Stderr = logFile, logFile
			}
			t.Cleanup(func() {
				for i, command := range commands {
					if command.Process != nil && !reaped[i] {
						_ = command.Process.Kill()
						_ = command.Wait()
					}
				}
			})
			start := make(chan struct{})
			type startResult struct {
				index int
				err   error
			}
			started := make(chan startResult, 2)
			for i, command := range commands {
				go func(index int, cmd *exec.Cmd) {
					<-start
					started <- startResult{index: index, err: cmd.Start()}
				}(i, command)
			}
			close(start)
			for range 2 {
				result := <-started
				if result.err != nil {
					t.Fatalf("start visible service window %d: %v", result.index, result.err)
				}
			}
			deadline := time.Now().Add(30 * time.Second)
			var winner ServiceConnection
			var winnerConfig Config
			for time.Now().Before(deadline) {
				winner = ConnectService(context.Background(), config)
				winnerConfig = config
				if winner.State != ConnectionConnected && differentRunDir {
					winner = ConnectService(context.Background(), otherConfig)
					winnerConfig = otherConfig
				}
				if winner.State == ConnectionConnected {
					break
				}
				time.Sleep(50 * time.Millisecond)
			}
			if winner.State != ConnectionConnected {
				firstLog, _ := os.ReadFile(logs[0])
				secondLog, _ := os.ReadFile(logs[1])
				t.Fatalf("no concurrent service window became ready: first=%s\nsecond=%s", firstLog, secondLog)
			}
			winnerIndex := -1
			for i, command := range commands {
				if command.Process.Pid == winner.Snapshot.Runtime.Service.PID {
					winnerIndex = i
				}
			}
			if winnerIndex < 0 {
				t.Fatalf("ready service PID %d belongs to neither window", winner.Snapshot.Runtime.Service.PID)
			}
			loserIndex := 1 - winnerIndex
			loserDone := make(chan error, 1)
			go func() { loserDone <- commands[loserIndex].Wait() }()
			select {
			case exitErr := <-loserDone:
				reaped[loserIndex] = true
				loserLog, _ := os.ReadFile(logs[loserIndex])
				if exitErr == nil || !strings.Contains(strings.ToLower(string(loserLog)), "lock") {
					t.Fatalf("competing window did not fail on the service lock: %v\n%s", exitErr, loserLog)
				}
			case <-time.After(15 * time.Second):
				_ = commands[loserIndex].Process.Kill()
				<-loserDone
				reaped[loserIndex] = true
				t.Fatal("competing service window did not exit")
			}
			if differentRunDir {
				other := ConnectService(context.Background(), otherConfig)
				first := ConnectService(context.Background(), config)
				if (first.State == ConnectionConnected) == (other.State == ConnectionConnected) {
					t.Fatalf("same repository has zero or two connected services: first=%s second=%s", first.State, other.State)
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			stopped, err := StopService(ctx, winnerConfig, "concurrent-window-test", 15*time.Second)
			cancel()
			if err != nil || stopped.Status != "stopped" || !stopped.ProcessExited || !stopped.LocksReleased {
				t.Fatalf("winner did not stop cleanly: %+v err=%v", stopped, err)
			}
			_ = commands[winnerIndex].Wait()
			reaped[winnerIndex] = true
		})
	}
}
