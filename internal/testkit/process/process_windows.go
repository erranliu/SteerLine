//go:build windows

package process

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var isProcessInJob = windows.NewLazySystemDLL("kernel32.dll").NewProc("IsProcessInJob")

// runOwned places the suspended child in a kill-on-close Job before it runs.
// Done is published only after the Job has no remaining descendants.
func Start(cmd *exec.Cmd) (*Process, error) {
	return startOwned(cmd, false)
}

// StartVisibleConsole gives an owned Windows child its own visible console.
// It is reserved for tests of product entry points that require a real
// service-window console.
func StartVisibleConsole(cmd *exec.Cmd) (*Process, error) {
	return startOwned(cmd, true)
}

func startOwned(cmd *exec.Cmd, visibleConsole bool) (*Process, error) {
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
	creationFlags := uint32(windows.CREATE_SUSPENDED | windows.CREATE_NEW_PROCESS_GROUP)
	if visibleConsole {
		creationFlags |= windows.CREATE_NEW_CONSOLE
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: creationFlags, HideWindow: !visibleConsole}
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
	var cleanupErr error
	terminate := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := terminateOwnedJob(ctx, job)
		cleanupErr = errors.Join(cleanupErr, err)
		return err
	}
	p.stop = func() error {
		jobMu.Lock()
		defer jobMu.Unlock()
		select {
		case <-p.Done:
			return nil
		default:
		}
		return terminate()
	}
	success = true
	go func() {
		err := cmd.Wait()
		jobMu.Lock()
		// Job termination is asynchronous. Retain the handle so the owning
		// scenario cannot delete its files while descendants are still exiting.
		_ = terminate()
		cleanupErr = errors.Join(cleanupErr, windows.CloseHandle(job))
		p.mu.Lock()
		p.err, p.cleanupErr = err, cleanupErr
		p.mu.Unlock()
		close(p.Done)
		jobMu.Unlock()
	}()
	return p, nil
}

func terminateOwnedJob(ctx context.Context, job windows.Handle) error {
	for {
		ids, err := jobProcessIDs(ctx, job)
		if err != nil {
			// Closing the Job still provides kill-on-close after this error.
			return errors.Join(err, windows.TerminateJobObject(job, 1))
		}
		if len(ids) == 0 {
			return nil
		}
		if err := terminateJobProcesses(ctx, job, ids); err != nil {
			return err
		}
		// Check again for descendants created between the snapshot and kill.
	}
}

func terminateJobProcesses(ctx context.Context, job windows.Handle, ids []uintptr) (result error) {
	defer func() {
		if result != nil {
			// An observation failure must still stop the owned tree, and must
			// remain a cleanup failure rather than silently reporting success.
			result = errors.Join(result, windows.TerminateJobObject(job, 1))
		}
	}()
	var handles []windows.Handle
	defer func() {
		for _, h := range handles {
			windows.CloseHandle(h)
		}
	}()
	for _, id := range ids {
		h, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(id))
		if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
			continue // The process exited before we could retain its handle.
		}
		if err != nil {
			return fmt.Errorf("open owned test descendant: %w", err)
		}
		var inJob uint32
		if ok, _, err := isProcessInJob.Call(uintptr(h), uintptr(job), uintptr(unsafe.Pointer(&inJob))); ok == 0 {
			windows.CloseHandle(h)
			return err
		}
		if inJob == 0 {
			windows.CloseHandle(h)
			continue // A reused PID is not evidence for this Job.
		}
		handles = append(handles, h)
	}
	if err := windows.TerminateJobObject(job, 1); err != nil {
		return err
	}
	for _, h := range handles {
		for {
			status, err := windows.WaitForSingleObject(h, 5)
			if err != nil {
				return err
			}
			if status == windows.WAIT_OBJECT_0 {
				break
			}
			if status != uint32(windows.WAIT_TIMEOUT) {
				return fmt.Errorf("unexpected owned descendant wait status: %d", status)
			}
			if err := ctx.Err(); err != nil {
				return fmt.Errorf("wait for owned test descendant exit: %w", err)
			}
		}
	}
	return nil
}

func jobProcessIDs(ctx context.Context, job windows.Handle) ([]uintptr, error) {
	// JOBOBJECT_BASIC_PROCESS_ID_LIST has two DWORDs followed by ULONG_PTRs.
	headerWords := 8 / int(unsafe.Sizeof(uintptr(0)))
	for capacity := 16; ; capacity *= 2 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		buffer := make([]uintptr, headerWords+capacity)
		header := (*struct{ Assigned, Count uint32 })(unsafe.Pointer(&buffer[0]))
		err := windows.QueryInformationJobObject(job, windows.JobObjectBasicProcessIdList, uintptr(unsafe.Pointer(&buffer[0])), uint32(len(buffer))*uint32(unsafe.Sizeof(buffer[0])), nil)
		if errors.Is(err, windows.ERROR_MORE_DATA) || err == nil && header.Assigned > header.Count {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("query owned test process Job: %w", err)
		}
		if int(header.Count) > capacity {
			return nil, errors.New("owned test process Job returned an invalid PID count")
		}
		return buffer[headerWords : headerWords+int(header.Count)], nil
	}
}
