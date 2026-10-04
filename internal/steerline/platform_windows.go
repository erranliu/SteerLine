//go:build windows

package steerline

import (
	"context"
	"errors"
	"fmt"
	"github.com/erranliu/SteerLine/internal/steerline/host"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	resourceprocess "github.com/erranliu/SteerLine/internal/execution/process"
	"golang.org/x/sys/windows"
)

func serviceConsoleWindow() uint64 {
	hwnd, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetConsoleWindow").Call()
	return uint64(hwnd)
}

// ServiceWindowContext ends a foreground service when its required console
// window is destroyed. A Windows host may close the window without terminating
// every process that used it, especially when a Job Object owns the process.
func ServiceWindowContext(parent context.Context, projectName string) (context.Context, context.CancelFunc, error) {
	hwnd := serviceConsoleWindow()
	if hwnd == 0 {
		return nil, nil, errors.New("service-window requires a visible Windows console")
	}
	user32 := windows.NewLazySystemDLL("user32.dll")
	isWindow := user32.NewProc("IsWindow")
	isVisible := user32.NewProc("IsWindowVisible")
	exists, _, _ := isWindow.Call(uintptr(hwnd))
	visible, _, _ := isVisible.Call(uintptr(hwnd))
	if exists == 0 || visible == 0 {
		return nil, nil, errors.New("service-window console is not visible")
	}
	title, err := windows.UTF16PtrFromString(projectName + " - SteerLine service")
	if err != nil {
		return nil, nil, fmt.Errorf("service-window title: %w", err)
	}
	if ok, _, callErr := windows.NewLazySystemDLL("kernel32.dll").NewProc("SetConsoleTitleW").Call(uintptr(unsafe.Pointer(title))); ok == 0 {
		return nil, nil, fmt.Errorf("set service-window title: %w", callErr)
	}
	ctx, cancel := context.WithCancel(parent)
	go func() {
		tick := time.NewTicker(500 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				exists, _, _ := isWindow.Call(uintptr(hwnd))
				if exists == 0 || serviceConsoleWindow() != hwnd {
					cancel()
					return
				}
			}
		}
	}()
	return ctx, cancel, nil
}

func canonical(s string) string { return strings.ToLower(filepath.Clean(s)) }

// Snapshot readers cooperate with atomic replacement, including the stop CLI.
func readSnapshot(path string) ([]byte, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	for attempt := 0; attempt < 100; attempt++ {
		h, e := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
		if e == nil {
			file := os.NewFile(uintptr(h), path)
			defer file.Close()
			return io.ReadAll(file)
		}
		err = e
		if e != windows.ERROR_SHARING_VIOLATION && e != windows.ERROR_ACCESS_DENIED {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	return nil, &os.PathError{Op: "read snapshot", Path: path, Err: err}
}

func replaceFile(from, to string) error {
	a, e := windows.UTF16PtrFromString(from)
	if e != nil {
		return e
	}
	b, e := windows.UTF16PtrFromString(to)
	if e != nil {
		return e
	}
	// Some Windows readers omit FILE_SHARE_DELETE. Retry transient sharing violations
	// while keeping the previous complete snapshot; never remove the destination.
	for attempt := 0; attempt < 100; attempt++ {
		e = windows.MoveFileEx(a, b, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
		if e != windows.ERROR_SHARING_VIOLATION && e != windows.ERROR_ACCESS_DENIED {
			return e
		}
		time.Sleep(5 * time.Millisecond)
	}
	return e
}

type RepoLock struct {
	f *os.File
	o windows.Overlapped
}

func LockRepository(identity string) (*RepoLock, error) {
	// Inside Git's common directory: shared by linked worktrees and all configurations,
	// outside the tracked checkout. Lock lifetime belongs to the OS, not a stale PID file.
	l, e := lockPath(filepath.Join(identity, "steerline.lock"))
	if e != nil {
		return nil, fmt.Errorf("repository already locked: %w", e)
	}
	return l, nil
}

func (l *RepoLock) Close() {
	windows.UnlockFileEx(windows.Handle(l.f.Fd()), 0, 1, 0, &l.o)
	l.f.Close()
}

func lockPath(path string) (*RepoLock, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	l := &RepoLock{f: f}
	err = windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &l.o)
	if err != nil {
		f.Close()
		if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			return nil, errLockBusy
		}
		return nil, err
	}
	return l, nil
}

func tryExistingLock(path string) (*RepoLock, error) { return lockPath(path) }

func probeLockFile(path string) (bool, error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer f.Close()
	lock := &RepoLock{f: f}
	err = windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &lock.o)
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	err = windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, &lock.o)
	return false, err
}

func startDetachedProcess(cmd *exec.Cmd) (executionbackend.ProcessIdentity, error) {
	// CREATE_BREAKAWAY_FROM_JOB is required because the caller may be a UI in a
	// kill-on-close job. IsProcessInJob below verifies the result before we claim
	// the service is detached. DETACHED_PROCESS also prevents console inheritance.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_SUSPENDED | windows.CREATE_BREAKAWAY_FROM_JOB | windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP,
		HideWindow:    true,
	}
	if err := cmd.Start(); err != nil {
		if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			if inJob, _, e := currentJobDiagnostics(); e == nil && inJob {
				return executionbackend.ProcessIdentity{}, fmt.Errorf("%w: create detached process: %v", host.ErrDetachUnsupported, err)
			}
		}
		return executionbackend.ProcessIdentity{}, fmt.Errorf("cannot detach service from caller job or console: %w", err)
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, uint32(cmd.Process.Pid))
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return executionbackend.ProcessIdentity{}, fmt.Errorf("cannot verify detached service process: %w", err)
	}
	defer windows.CloseHandle(h)
	var inJob uint32
	ok, _, callErr := windows.NewLazySystemDLL("kernel32.dll").NewProc("IsProcessInJob").Call(uintptr(h), 0, uintptr(unsafe.Pointer(&inJob)))
	if ok == 0 {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return executionbackend.ProcessIdentity{}, fmt.Errorf("cannot verify service Job Object isolation: %w", callErr)
	}
	if inJob != 0 {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		parentInJob, parentFlags, parentErr := currentJobDiagnostics()
		detail := fmt.Sprintf("service remains inside a Job Object (parent_in_job=%t", parentInJob)
		if parentInJob {
			detail += fmt.Sprintf(", parent_job_limit_flags=0x%x", parentFlags)
		}
		if parentErr != nil {
			detail += ", parent_job_query_error=" + parentErr.Error()
		}
		return executionbackend.ProcessIdentity{}, fmt.Errorf("%w: %s); cannot guarantee survival after the UI exits", host.ErrDetachUnsupported, detail)
	}
	id, err := resourceprocess.IdentityFromHandle(h, cmd.Process.Pid)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return executionbackend.ProcessIdentity{}, fmt.Errorf("cannot identify detached service process: %w", err)
	}
	// No service instruction (including recovery of existing workers) may run
	// until we know this process can survive the launcher. A failed isolation
	// check above terminates an unstarted process without service side effects.
	if err = resumeDetachedProcess(cmd.Process.Pid); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return executionbackend.ProcessIdentity{}, fmt.Errorf("cannot resume detached service: %w", err)
	}
	return id, nil
}

func resumeDetachedProcess(pid int) error {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	resumed := false
	for err = windows.Thread32First(snapshot, &entry); err == nil; err = windows.Thread32Next(snapshot, &entry) {
		if entry.OwnerProcessID != uint32(pid) {
			continue
		}
		thread, e := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
		if e != nil {
			return e
		}
		previous, e := windows.ResumeThread(thread)
		windows.CloseHandle(thread)
		if e != nil {
			return e
		}
		resumed = resumed || previous > 0
	}
	if resumed && err == windows.ERROR_NO_MORE_FILES {
		return nil
	}
	return fmt.Errorf("cannot locate suspended initial thread: %w", err)
}

func currentJobDiagnostics() (bool, uint32, error) {
	process, err := windows.GetCurrentProcess()
	if err != nil {
		return false, 0, err
	}
	inJob, err := processInAnyJob(process)
	if err != nil {
		return false, 0, err
	}
	if !inJob {
		return false, 0, nil
	}
	var limits windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	err = windows.QueryInformationJobObject(0, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits)), nil)
	if err != nil {
		return true, 0, err
	}
	return true, limits.BasicLimitInformation.LimitFlags, nil
}

func processInAnyJob(process windows.Handle) (bool, error) {
	var inJob uint32
	ok, _, callErr := windows.NewLazySystemDLL("kernel32.dll").NewProc("IsProcessInJob").Call(uintptr(process), 0, uintptr(unsafe.Pointer(&inJob)))
	if ok == 0 {
		return false, callErr
	}
	return inJob != 0, nil
}
