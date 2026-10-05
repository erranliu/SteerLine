//go:build windows

package host

import (
	"bytes"
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

	resourceprocess "github.com/erranliu/SteerLine/internal/execution/process"
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
	entries, e := os.ReadDir(filepath.Dir(path))
	require(t, e)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".steerline-") && strings.HasSuffix(entry.Name(), ".tmp") {
			t.Fatalf("failed replacement left a temporary snapshot: %s", entry.Name())
		}
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
	identity, err := resourceprocess.IdentityFromHandle(handle, cmd.Process.Pid)
	require(t, err)
	require(t, cmd.Process.Kill())
	_, err = windows.WaitForSingleObject(handle, 5000)
	require(t, err)
	if probe := testProcessProbe(&identity); probe.State != ProcessAbsent {
		t.Fatalf("retained exited process handle reported alive: %+v", probe)
	}
}

func TestWindowsOwnedWorkerRunsInJobAndStopsWithOwner(t *testing.T) {
	exe, err := os.Executable()
	require(t, err)
	childPath := filepath.Join(t.TempDir(), "child.pid")
	cmd := exec.Command(exe, "--fake", "spawn-owned-child", childPath)
	owned, err := resourceprocess.StartManaged(cmd)
	require(t, err)
	defer func() { owned.Close(); _ = cmd.Wait() }()
	identity := owned.Identity()
	if resourceprocess.InspectProcess(&identity) != "matching process still exists" {
		t.Fatal("owned worker identity is not live")
	}
	var childPID int
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
		data, readErr := os.ReadFile(childPath)
		if readErr == nil {
			childPID, err = strconv.Atoi(string(data))
			require(t, err)
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if childPID == 0 {
		t.Fatal("owned worker did not start its child")
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(childPID))
	require(t, err)
	childIdentity, err := resourceprocess.IdentityFromHandle(handle, childPID)
	require(t, err)
	defer func() {
		if resourceprocess.InspectProcess(&childIdentity) == "matching process still exists" {
			if child, findErr := os.FindProcess(childPID); findErr == nil {
				_ = child.Kill()
			}
		}
	}()
	windows.CloseHandle(handle)
	inJob, err := owned.ContainsProcess(uint32(childPID))
	require(t, err)
	if !inJob {
		t.Fatal("owned child did not inherit the worker Job Object")
	}
	owned.Close()
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
		if resourceprocess.WorkerTreeAbsent(identity) && resourceprocess.InspectProcess(&childIdentity) == "process absent" {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("closing the owner did not terminate the worker and child")
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

func TestWindowsStartServiceRequiresVisibleWindow(t *testing.T) {
	f := newFixture(t)
	_, configPath := lifecycleConfigFile(t, f, f.c.RunDir)
	testExe, err := os.Executable()
	require(t, err)
	home := fixtureHome()
	_, err = InstallLaunchers(fixtureHome())
	require(t, err)
	result, err := testStartService(context.Background(), configPath, StartServiceOptions{Executable: testExe})
	require(t, err)
	if result.Status != "failed" || !strings.Contains(result.Error, "steerline install") {
		t.Fatalf("missing installed binary was not explained: %+v", result)
	}
	require(t, os.MkdirAll(filepath.Join(home, "bin"), 0700))
	require(t, os.WriteFile(filepath.Join(home, "bin", "steerline.exe"), []byte("test-only installed path"), 0600))
	result, err = testStartService(context.Background(), configPath, StartServiceOptions{Executable: testExe})
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
	_, err := InstallLaunchers(fixtureHome())
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

func TestWindowsPendingStartupPreservesMismatchClassification(t *testing.T) {
	for _, kind := range []string{"config", "repository-path"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixture(t)
			c, configPath := lifecycleConfigFile(t, f, f.c.RunDir)
			identity, err := resourceprocess.CurrentIdentity()
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
			if kind == "repository-path" {
				path, err := startupPath(c.RunDir, record.StartupID)
				require(t, err)
				before, err := os.ReadFile(path)
				require(t, err)
				if _, err := loadFixtureConfig(configPath); err == nil {
					t.Fatal("startup evidence from another repository was accepted")
				}
				after, err := os.ReadFile(path)
				require(t, err)
				if !bytes.Equal(before, after) {
					t.Fatal("rejected startup evidence was modified")
				}
				return
			}
			result, err := testStartService(context.Background(), configPath, StartServiceOptions{})
			require(t, err)
			if result.Status != "pending" || result.Connection.State != expected ||
				result.Connection.Process.State != ProcessMatches || result.Connection.CanStart ||
				result.Connection.StartupID != record.StartupID {
				t.Fatalf("pending %s mismatch lost known identity or classification: %+v", kind, result)
			}
		})
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
