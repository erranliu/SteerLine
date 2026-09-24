//go:build windows

package steerline

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

func TestWindowsScheduledTTYParentExitAcceptance(t *testing.T) {
	if os.Getenv("STEERLINE_RUN_TTY_ACCEPTANCE") != "1" {
		return
	}
	f := newFixture(t, task("t-1"))
	db := readFake(f.db)
	db.Behavior = "sleep"
	writeFake(f.db, db)
	tempDir := t.TempDir()
	c := f.c
	c.Remote = "web"
	c.WorktreeRoot = ""
	configBytes, err := json.Marshal(c)
	require(t, err)
	configPath := filepath.Join(tempDir, "tty-service-config.json")
	require(t, os.WriteFile(configPath, configBytes, 0600))
	config, err := LoadConfig(configPath)
	require(t, err)
	serviceExe := buildServiceLifecycleCLI(t)
	testExe, err := os.Executable()
	require(t, err)
	reportPath := filepath.Join(tempDir, "lifecycle-report.json")
	logPath := filepath.Join(tempDir, "lifecycle-parent.log")
	scriptPath := filepath.Join(tempDir, "run-lifecycle-parent.cmd")
	cmdExe, err := exec.LookPath("cmd.exe")
	require(t, err)
	script := fmt.Sprintf("@echo off\r\necho SteerLine isolated TTY lifecycle acceptance\r\nset \"STEERLINE_LIFECYCLE_PARENT=1\"\r\nset \"STEERLINE_LIFECYCLE_REQUIRE_TTY=1\"\r\nset \"STEERLINE_LIFECYCLE_HOLD=1\"\r\nset \"STEERLINE_LIFECYCLE_CONFIG=%s\"\r\nset \"STEERLINE_LIFECYCLE_EXE=%s\"\r\nset \"STEERLINE_LIFECYCLE_REPORT=%s\"\r\n\"%s\" -test.run=TestServiceLifecycleParentHelper > \"%s\" 2>&1\r\nexit /b %%ERRORLEVEL%%\r\n", configPath, serviceExe, reportPath, testExe, logPath)
	require(t, os.WriteFile(scriptPath, []byte(script), 0600))
	taskName := "SteerLineTTYAcceptance-" + newID()
	t.Cleanup(func() {
		_ = exec.Command("schtasks.exe", "/End", "/TN", taskName).Run()
		_ = exec.Command("schtasks.exe", "/Delete", "/TN", taskName, "/F").Run()
		connection := ConnectService(context.Background(), config)
		if connection.CanStop {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			_, _ = StopService(ctx, config, "tty-test-cleanup", 12*time.Second)
		}
	})
	launchDate := time.Date(2099, time.December, 31, 23, 59, 0, 0, time.Local)
	action := fmt.Sprintf("\"%s\" /d /c \"%s\"", cmdExe, scriptPath)
	create := exec.Command("schtasks.exe", "/Create", "/TN", taskName, "/TR", action, "/SC", "ONCE", "/SD", launchDate.Format("2006/01/02"), "/ST", launchDate.Format("15:04"), "/IT", "/F")
	createOutput, err := create.CombinedOutput()
	if err != nil {
		t.Skipf("cannot create temporary interactive acceptance task: %v (%s)", err, createOutput)
	}
	runOutput, err := exec.Command("schtasks.exe", "/Run", "/TN", taskName).CombinedOutput()
	if err != nil {
		t.Fatalf("run temporary interactive acceptance task: %v (%s)", err, runOutput)
	}
	deadline := time.Now().Add(45 * time.Second)
	var report lifecycleParentReport
	for time.Now().Before(deadline) {
		if data, readErr := os.ReadFile(reportPath); readErr == nil {
			if jsonErr := json.Unmarshal(data, &report); jsonErr == nil {
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	if report.Status != "worker-running" {
		log, _ := os.ReadFile(logPath)
		if report.Status == "start-failed" && (strings.Contains(strings.ToLower(report.Error), "detach") || strings.Contains(strings.ToLower(report.Error), "access is denied")) && report.TTY && report.ConsoleWindow != 0 && report.ParentIdentity != nil {
			root := filepath.Join(config.RunDir, "service", "startups")
			entries, _ := os.ReadDir(root)
			var failedAttempt *StartupRecord
			for _, entry := range entries {
				if !entry.IsDir() || !controlIDPattern.MatchString(entry.Name()) {
					continue
				}
				if candidate, readErr := readStartupRecord(config, entry.Name()); readErr == nil && candidate.Status == "failed" {
					failedAttempt = candidate
				}
			}
			if failedAttempt == nil || !strings.Contains(strings.ToLower(failedAttempt.Error), "detach") && !strings.Contains(strings.ToLower(failedAttempt.Error), "access is denied") {
				t.Fatalf("real TTY detachment failure was not recorded: attempt=%+v error=%q tasklog=%s", failedAttempt, report.Error, log)
			}
			closeDeadline := time.Now().Add(15 * time.Second)
			for time.Now().Before(closeDeadline) && (probeProcess(report.ParentIdentity).State == ProcessMatches || windowExistsForTests(report.ConsoleWindow)) {
				time.Sleep(100 * time.Millisecond)
			}
			if probeProcess(report.ParentIdentity).State == ProcessMatches || windowExistsForTests(report.ConsoleWindow) {
				_ = exec.Command("schtasks.exe", "/End", "/TN", taskName).Run()
				t.Fatalf("failed start did not close its real TTY parent: console_window=%x", report.ConsoleWindow)
			}
			if _, statErr := os.Stat(filepath.Join(config.RunDir, config.RuntimeFile)); statErr == nil {
				t.Fatal("rejected TTY launch published a runtime snapshot")
			}
			t.Skipf("real TTY parent exited and its window closed; service correctly rejected the inherited non-breakaway Job Object: %s", failedAttempt.Error)
		}
		if report.Status == "not-tty" || report.Status == "no-console-window" {
			t.Skipf("interactive task did not provide a detachable TTY: status=%s error=%q tasklog=%s", report.Status, report.Error, log)
		}
		t.Fatalf("TTY lifecycle parent did not start service/worker: report=%+v tasklog=%s", report, log)
	}
	if !report.TTY || report.ConsoleWindow == 0 || report.ParentPID <= 0 || report.ParentIdentity == nil || report.ServiceIdentity == nil || report.WorkerIdentity == nil {
		t.Fatalf("TTY helper omitted console or process identities: %+v", report)
	}
	if !windowExistsForTests(report.ConsoleWindow) || !windowVisibleForTests(report.ConsoleWindow) {
		t.Fatalf("reported console window %x is not visible before close", report.ConsoleWindow)
	}
	runtimeBefore, err := LoadRuntime(filepath.Join(config.RunDir, config.RuntimeFile))
	require(t, err)
	if runtimeBefore == nil || len(runtimeBefore.Runs) != 1 || runtimeBefore.Service.InstanceID != report.InstanceID || probeProcess(runtimeBefore.Service.ProcessIdentity).State != ProcessMatches || probeProcess(runtimeBefore.Runs[0].Process).State != ProcessMatches {
		t.Fatalf("service and worker are not live before console close: %+v", runtimeBefore)
	}
	if started := readFake(f.db).Starts["t-1"]; started != 1 {
		t.Fatalf("manual service worker starts=%d, want one explicit start", started)
	}
	if err = closeConsoleWindowForTests(report.ConsoleWindow); err != nil {
		t.Fatalf("close isolated TTY window: %v", err)
	}
	closeDeadline := time.Now().Add(15 * time.Second)
	parentExited, consoleClosed := false, false
	for time.Now().Before(closeDeadline) {
		parentProbe := probeProcess(report.ParentIdentity)
		parentExited = parentProbe.State == ProcessAbsent || parentProbe.State == ProcessReused
		consoleClosed = !windowExistsForTests(report.ConsoleWindow) && !windowVisibleForTests(report.ConsoleWindow)
		if parentExited && consoleClosed {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !parentExited || !consoleClosed {
		_ = exec.Command("schtasks.exe", "/End", "/TN", taskName).Run()
		t.Fatalf("TTY close did not exit the parent and window: parent_exited=%t console_closed=%t", parentExited, consoleClosed)
	}
	connection := ConnectService(context.Background(), config)
	if connection.State != ConnectionConnected || connection.Snapshot.Runtime.Service.InstanceID != report.InstanceID || probeProcess(report.ServiceIdentity).State != ProcessMatches || probeProcess(report.WorkerIdentity).State != ProcessMatches {
		t.Fatalf("service or worker identity changed after real TTY close: %+v", connection)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	stopped, err := StopService(ctx, config, "tty-lifecycle-acceptance", 12*time.Second)
	if err != nil || stopped.Status != "stopped" || !stopped.ProcessExited || !stopped.LocksReleased || stopped.Receipt == nil || stopped.Receipt.Status != "applied" {
		t.Fatalf("explicit stop after TTY close: result=%+v err=%v", stopped, err)
	}
	if probeProcess(report.ServiceIdentity).State != ProcessAbsent && probeProcess(report.ServiceIdentity).State != ProcessReused {
		t.Fatal("service process survived explicit stop")
	}
	if probeProcess(report.WorkerIdentity).State != ProcessAbsent && probeProcess(report.WorkerIdentity).State != ProcessReused {
		t.Fatal("worker process survived explicit stop")
	}
}
