//go:build windows

package steerline

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

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

type childProcess struct {
	mu       sync.Mutex
	job      windows.Handle
	identity ProcessIdentity
}

func (p *childProcess) Identity() ProcessIdentity { return p.identity }
func (p *childProcess) Kill() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.job != 0 {
		windows.TerminateJobObject(p.job, 1)
	}
}
func (p *childProcess) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.job != 0 {
		windows.CloseHandle(p.job)
		p.job = 0
	}
}
func startProcess(cmd *exec.Cmd) (*childProcess, error) {
	job, e := windows.CreateJobObject(nil, nil)
	if e != nil {
		return nil, e
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	_, e = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)))
	if e != nil {
		windows.CloseHandle(job)
		return nil, e
	}
	// Suspend before the first instruction: no descendant can escape job assignment.
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_SUSPENDED | windows.CREATE_NEW_PROCESS_GROUP, HideWindow: true}
	if e = cmd.Start(); e != nil {
		windows.CloseHandle(job)
		return nil, e
	}
	fail := func(err error) (*childProcess, error) {
		cmd.Process.Kill()
		cmd.Wait()
		windows.CloseHandle(job)
		return nil, err
	}
	h, e := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(cmd.Process.Pid))
	if e != nil {
		return fail(e)
	}
	defer windows.CloseHandle(h)
	if e = windows.AssignProcessToJobObject(job, h); e != nil {
		return fail(e)
	}
	id, e := identityFromHandle(h, cmd.Process.Pid)
	if e != nil {
		return fail(e)
	}
	snap, e := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if e != nil {
		return fail(e)
	}
	defer windows.CloseHandle(snap)
	te := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	resumed := false
	for e = windows.Thread32First(snap, &te); e == nil; e = windows.Thread32Next(snap, &te) {
		if te.OwnerProcessID != uint32(cmd.Process.Pid) {
			continue
		}
		th, err := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, te.ThreadID)
		if err != nil {
			return fail(err)
		}
		previous, err := windows.ResumeThread(th)
		windows.CloseHandle(th)
		if err != nil {
			return fail(err)
		}
		// Toolhelp does not promise primary-thread ordering. Instrumentation may
		// have created another thread before enumeration; resume every suspended
		// thread in this pre-start snapshot, all already protected by the job.
		resumed = resumed || previous > 0
	}
	if resumed && e == windows.ERROR_NO_MORE_FILES {
		return &childProcess{job: job, identity: id}, nil
	}
	return fail(fmt.Errorf("cannot locate suspended initial thread: %w", e))
}

func startDetachedProcess(cmd *exec.Cmd) (ProcessIdentity, error) {
	// CREATE_BREAKAWAY_FROM_JOB is required because the caller may be a UI in a
	// kill-on-close job. IsProcessInJob below verifies the result before we claim
	// the service is detached. DETACHED_PROCESS also prevents console inheritance.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_BREAKAWAY_FROM_JOB | windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP,
		HideWindow:    true,
	}
	if err := cmd.Start(); err != nil {
		return ProcessIdentity{}, fmt.Errorf("cannot detach service from caller job or console: %w", err)
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, uint32(cmd.Process.Pid))
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return ProcessIdentity{}, fmt.Errorf("cannot verify detached service process: %w", err)
	}
	defer windows.CloseHandle(h)
	var inJob uint32
	ok, _, callErr := windows.NewLazySystemDLL("kernel32.dll").NewProc("IsProcessInJob").Call(uintptr(h), 0, uintptr(unsafe.Pointer(&inJob)))
	if ok == 0 {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return ProcessIdentity{}, fmt.Errorf("cannot verify service Job Object isolation: %w", callErr)
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
		return ProcessIdentity{}, errors.New(detail + "); cannot guarantee survival after the UI exits")
	}
	id, err := identityFromHandle(h, cmd.Process.Pid)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return ProcessIdentity{}, fmt.Errorf("cannot identify detached service process: %w", err)
	}
	return id, nil
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

func processInJobByPID(pid int) (bool, error) {
	if pid <= 0 {
		return false, errors.New("invalid process ID")
	}
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false, err
	}
	defer windows.CloseHandle(process)
	return processInAnyJob(process)
}
func identityFromHandle(h windows.Handle, pid int) (ProcessIdentity, error) {
	var created, exit, kernel, user windows.Filetime
	if e := windows.GetProcessTimes(h, &created, &exit, &kernel, &user); e != nil {
		return ProcessIdentity{}, e
	}
	b := make([]uint16, 32768)
	n := uint32(len(b))
	if e := windows.QueryFullProcessImageName(h, 0, &b[0], &n); e != nil {
		return ProcessIdentity{}, e
	}
	return ProcessIdentity{Kind: "os", PID: pid, Created: fmt.Sprintf("%08x%08x", created.HighDateTime, created.LowDateTime), Executable: canonical(windows.UTF16ToString(b[:n]))}, nil
}
func currentProcessIdentity() (*ProcessIdentity, error) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(os.Getpid()))
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(h)
	id, err := identityFromHandle(h, os.Getpid())
	if err != nil {
		return nil, err
	}
	return &id, nil
}
func inspectProcess(old *ProcessIdentity) string {
	probe := probeProcess(old)
	switch probe.State {
	case ProcessAbsent:
		return "process absent"
	case ProcessMatches:
		return "matching process still exists"
	case ProcessReused:
		return "PID belongs to a different process"
	default:
		return "process identity unavailable: " + probe.Reason
	}
}

func probeProcess(old *ProcessIdentity) ProcessProbe {
	if old == nil || old.PID <= 0 || old.Kind != "os" {
		return ProcessProbe{State: ProcessUnknown, Reason: "process identity is missing or unsupported"}
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(old.PID))
	if err != nil {
		if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
			return ProcessProbe{State: ProcessAbsent, Reason: "process does not exist"}
		}
		return ProcessProbe{State: ProcessUnknown, Reason: err.Error()}
	}
	defer windows.CloseHandle(h)
	id, err := identityFromHandle(h, old.PID)
	if err != nil {
		return ProcessProbe{State: ProcessUnknown, Reason: err.Error()}
	}
	if id.Created == old.Created && id.Executable == old.Executable {
		return ProcessProbe{State: ProcessMatches, Reason: "process creation identity and executable match"}
	}
	return ProcessProbe{State: ProcessReused, Reason: "PID belongs to a different process identity"}
}
