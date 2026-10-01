//go:build windows

package testprocess

import (
	"fmt"
	"os/exec"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// runOwned places the suspended child in a kill-on-close Job before it runs.
// Closing the Job after Wait also removes any descendants it started.
func Start(cmd *exec.Cmd) (*Process, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	success := false
	defer func() {
		if !success {
			windows.CloseHandle(job)
		}
	}()
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	_, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)))
	if err != nil {
		return nil, err
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_SUSPENDED | windows.CREATE_NEW_PROCESS_GROUP, HideWindow: true}
	if err = cmd.Start(); err != nil {
		return nil, err
	}
	fail := func(err error) error { _ = cmd.Process.Kill(); _ = cmd.Wait(); return err }
	h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(cmd.Process.Pid))
	if err != nil {
		return nil, fail(err)
	}
	defer windows.CloseHandle(h)
	if err = windows.AssignProcessToJobObject(job, h); err != nil {
		return nil, fail(err)
	}
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return nil, fail(err)
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	resumed := false
	for err = windows.Thread32First(snapshot, &entry); err == nil; err = windows.Thread32Next(snapshot, &entry) {
		if entry.OwnerProcessID != uint32(cmd.Process.Pid) {
			continue
		}
		thread, openErr := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
		if openErr != nil {
			return nil, fail(openErr)
		}
		previous, resumeErr := windows.ResumeThread(thread)
		windows.CloseHandle(thread)
		if resumeErr != nil {
			return nil, fail(resumeErr)
		}
		resumed = resumed || previous > 0
	}
	if !resumed || err != windows.ERROR_NO_MORE_FILES {
		return nil, fail(fmt.Errorf("cannot resume owned test process: %w", err))
	}
	p := &Process{Cmd: cmd, Done: make(chan struct{})}
	var jobMu sync.Mutex
	p.stop = func() error {
		jobMu.Lock()
		defer jobMu.Unlock()
		select {
		case <-p.Done:
			return nil
		default:
		}
		return windows.TerminateJobObject(job, 1)
	}
	success = true
	go func() {
		err := cmd.Wait()
		p.mu.Lock()
		p.err = err
		p.mu.Unlock()
		jobMu.Lock()
		windows.CloseHandle(job)
		close(p.Done)
		jobMu.Unlock()
	}()
	return p, nil
}
