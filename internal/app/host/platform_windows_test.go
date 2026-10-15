//go:build windows

package host

import (
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
	if probe := publicProcessProbe(resourceprocess.NewServiceFacts(), &identity); probe.State != ProcessAbsent {
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

type windowsServiceWindowProbe struct {
	TTY           bool
	ConsoleWindow uint64
}
