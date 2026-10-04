//go:build windows

package processhost

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

type processHandle struct {
	mu       sync.Mutex
	job      windows.Handle
	identity executionbackend.ProcessIdentity
}

func (*processHandle) OwnedProcess() bool { return true }

func (handle *processHandle) Identity() executionbackend.ProcessIdentity { return handle.identity }

func (handle *processHandle) Kill() {
	handle.mu.Lock()
	defer handle.mu.Unlock()
	if handle.job != 0 {
		_ = windows.TerminateJobObject(handle.job, 1)
	}
}

func (handle *processHandle) Close() {
	handle.mu.Lock()
	defer handle.mu.Unlock()
	if handle.job != 0 {
		_ = windows.CloseHandle(handle.job)
		handle.job = 0
	}
}

func startManaged(command *exec.Cmd) (executionbackend.StopHandle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		_ = windows.CloseHandle(job)
		return nil, err
	}
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_SUSPENDED | windows.CREATE_NEW_PROCESS_GROUP, HideWindow: true}
	if err = command.Start(); err != nil {
		_ = windows.CloseHandle(job)
		return nil, err
	}
	fail := func(cause error) (executionbackend.StopHandle, error) {
		_ = command.Process.Kill()
		_ = command.Wait()
		_ = windows.CloseHandle(job)
		return nil, cause
	}
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(command.Process.Pid))
	if err != nil {
		return fail(err)
	}
	defer windows.CloseHandle(process)
	if err = windows.AssignProcessToJobObject(job, process); err != nil {
		return fail(err)
	}
	identity, err := identityFromHandle(process, command.Process.Pid)
	if err != nil {
		return fail(err)
	}
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return fail(err)
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	resumed := false
	for err = windows.Thread32First(snapshot, &entry); err == nil; err = windows.Thread32Next(snapshot, &entry) {
		if entry.OwnerProcessID != uint32(command.Process.Pid) {
			continue
		}
		thread, openErr := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
		if openErr != nil {
			return fail(openErr)
		}
		previous, resumeErr := windows.ResumeThread(thread)
		_ = windows.CloseHandle(thread)
		if resumeErr != nil {
			return fail(resumeErr)
		}
		resumed = resumed || previous > 0
	}
	if resumed && err == windows.ERROR_NO_MORE_FILES {

		return &processHandle{job: job, identity: identity}, nil
	}
	return fail(fmt.Errorf("cannot locate suspended initial thread: %w", err))
}

func inspectProcess(old *executionbackend.ProcessIdentity) string {
	if old == nil || old.PID <= 0 || old.Kind != "os" || old.Created == "" || old.Executable == "" {
		return "process identity is missing or unsupported"
	}
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, uint32(old.PID))
	if err != nil {
		if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
			return "process absent"
		}
		return "process identity unavailable: " + err.Error()
	}
	defer windows.CloseHandle(process)
	status, err := windows.WaitForSingleObject(process, 0)
	if err != nil {
		return "process identity unavailable: " + err.Error()
	}
	if status == windows.WAIT_OBJECT_0 {
		return "process absent"
	}
	identity, err := identityFromHandle(process, old.PID)
	if err != nil {
		return "process identity unavailable: " + err.Error()
	}
	if identity.Created == old.Created && identity.Executable == old.Executable {
		return "matching process still exists"
	}
	return "PID belongs to a different process"
}

func identityFromHandle(process windows.Handle, pid int) (executionbackend.ProcessIdentity, error) {
	var created, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(process, &created, &exit, &kernel, &user); err != nil {
		return executionbackend.ProcessIdentity{}, err
	}
	image := make([]uint16, 32768)
	length := uint32(len(image))
	if err := windows.QueryFullProcessImageName(process, 0, &image[0], &length); err != nil {
		return executionbackend.ProcessIdentity{}, err
	}
	executable := strings.ToLower(filepath.Clean(windows.UTF16ToString(image[:length])))
	return executionbackend.ProcessIdentity{Kind: "os", PID: pid, Created: fmt.Sprintf("%08x%08x", created.HighDateTime, created.LowDateTime), Executable: executable}, nil
}

func replaceFile(from, to string) error {
	fromPtr, err := windows.UTF16PtrFromString(from)
	if err != nil {
		return err
	}
	toPtr, err := windows.UTF16PtrFromString(to)
	if err != nil {
		return err
	}
	for attempt := 0; attempt < 100; attempt++ {
		err = windows.MoveFileEx(fromPtr, toPtr, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
		if err != windows.ERROR_SHARING_VIOLATION && err != windows.ERROR_ACCESS_DENIED {
			return err
		}
		time.Sleep(5 * time.Millisecond)
	}
	return err
}
