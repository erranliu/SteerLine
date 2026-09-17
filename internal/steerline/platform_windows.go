//go:build windows

package steerline

import (
	"fmt"
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
	f, e := os.OpenFile(filepath.Join(identity, "steerline.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	l := &RepoLock{f: f}
	e = windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &l.o)
	if e != nil {
		f.Close()
		return nil, fmt.Errorf("repository already locked: %w", e)
	}
	return l, nil
}
func (l *RepoLock) Close() {
	windows.UnlockFileEx(windows.Handle(l.f.Fd()), 0, 1, 0, &l.o)
	l.f.Close()
}

type childProcess struct {
	mu       sync.Mutex
	job      windows.Handle
	identity ProcessIdentity
}

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
	return ProcessIdentity{PID: pid, Created: fmt.Sprintf("%08x%08x", created.HighDateTime, created.LowDateTime), Executable: canonical(windows.UTF16ToString(b[:n]))}, nil
}
func inspectProcess(old *ProcessIdentity) string {
	if old == nil {
		return "identity unavailable"
	}
	h, e := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(old.PID))
	if e != nil {
		return "process absent or inaccessible"
	}
	defer windows.CloseHandle(h)
	id, e := identityFromHandle(h, old.PID)
	if e != nil {
		return "process identity unavailable"
	}
	if id.Created == old.Created && id.Executable == old.Executable {
		return "matching process still exists"
	}
	return "PID belongs to a different process"
}
