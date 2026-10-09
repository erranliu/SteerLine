//go:build windows

package process

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"unsafe"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	"golang.org/x/sys/windows"
)

type childProcess struct {
	mu            sync.Mutex
	job           windows.Handle
	identity      executionbackend.ProcessIdentity
	writersAbsent bool
}

func (p *childProcess) WritersAbsent() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.job == 0 {
		return p.writersAbsent
	}
	var info struct {
		TotalUserTime, TotalKernelTime, ThisPeriodTotalUserTime, ThisPeriodTotalKernelTime int64
		TotalPageFaultCount, TotalProcesses, ActiveProcesses, TotalTerminatedProcesses     uint32
	}
	err := windows.QueryInformationJobObject(p.job, windows.JobObjectBasicAccountingInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)), nil)
	if err == nil && info.ActiveProcesses == 0 {
		p.writersAbsent = true
	}
	return p.writersAbsent
}

func (p *childProcess) Identity() executionbackend.ProcessIdentity { return p.identity }

func (*childProcess) OwnedProcess() bool { return true }

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

func StartManaged(cmd *exec.Cmd) (*childProcess, error) {
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
	id, e := IdentityFromHandle(h, cmd.Process.Pid)
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

// A managed worker's descendants remain in its non-breakaway, kill-on-close
// Job. Closing the owning handle (including on service exit) ends that tree.
func WorkerTreeAbsent(identity executionbackend.ProcessIdentity) bool {
	return InspectProcess(&identity) == "process absent"
}

func IdentityFromHandle(h windows.Handle, pid int) (executionbackend.ProcessIdentity, error) {
	var created, exit, kernel, user windows.Filetime
	if e := windows.GetProcessTimes(h, &created, &exit, &kernel, &user); e != nil {
		return executionbackend.ProcessIdentity{}, e
	}
	b := make([]uint16, 32768)
	n := uint32(len(b))
	if e := windows.QueryFullProcessImageName(h, 0, &b[0], &n); e != nil {
		return executionbackend.ProcessIdentity{}, e
	}
	return executionbackend.ProcessIdentity{Kind: "os", PID: pid, Created: fmt.Sprintf("%08x%08x", created.HighDateTime, created.LowDateTime), Executable: canonical(windows.UTF16ToString(b[:n]))}, nil
}

func CurrentIdentity() (*executionbackend.ProcessIdentity, error) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(os.Getpid()))
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(h)
	id, err := IdentityFromHandle(h, os.Getpid())
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func InspectProcess(old *executionbackend.ProcessIdentity) string {
	probe := Probe(old)
	switch probe.State {
	case "absent":
		return "process absent"
	case "matches":
		return "matching process still exists"
	case "pid-reused":
		return "PID belongs to a different process"
	default:
		if probe.Reason == "process identity is missing or unsupported" {
			return probe.Reason
		}
		return "process identity unavailable: " + probe.Reason
	}
}

func Probe(old *executionbackend.ProcessIdentity) ProbeResult {
	if old == nil || old.PID <= 0 || old.Kind != "os" || old.Created == "" || old.Executable == "" {
		return ProbeResult{State: "unknown", Reason: "process identity is missing or unsupported"}
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, uint32(old.PID))
	if err != nil {
		if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
			return ProbeResult{State: "absent", Reason: "process does not exist"}
		}
		return ProbeResult{State: "unknown", Reason: err.Error()}
	}
	defer windows.CloseHandle(h)
	// An exited process remains queryable while another process holds a handle.
	// Creation identity alone does not establish that it is still running.
	status, err := windows.WaitForSingleObject(h, 0)
	if err != nil {
		return ProbeResult{State: "unknown", Reason: err.Error()}
	}
	if status == windows.WAIT_OBJECT_0 {
		return ProbeResult{State: "absent", Reason: "process exited"}
	}
	id, err := IdentityFromHandle(h, old.PID)
	if err != nil {
		return ProbeResult{State: "unknown", Reason: err.Error()}
	}
	if id.Created == old.Created && id.Executable == old.Executable {
		return ProbeResult{State: "matches", Reason: "process creation identity and executable match"}
	}
	return ProbeResult{State: "pid-reused", Reason: "PID belongs to a different process identity"}
}

func (p *childProcess) ContainsProcess(pid uint32) (bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.job == 0 {
		return false, errors.New("worker job is closed")
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return false, err
	}
	defer windows.CloseHandle(h)
	var member uint32
	ok, _, callErr := windows.NewLazySystemDLL("kernel32.dll").NewProc("IsProcessInJob").Call(uintptr(h), uintptr(p.job), uintptr(unsafe.Pointer(&member)))
	if ok == 0 {
		if callErr != windows.ERROR_SUCCESS {
			return false, callErr
		}
		return false, errors.New("IsProcessInJob failed without a system error")
	}
	return member != 0, nil
}
