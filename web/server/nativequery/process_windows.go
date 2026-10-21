//go:build windows

package nativequery

import (
	"errors"
	"golang.org/x/sys/windows"
	"os/exec"
	"sync"
	"unsafe"
)

func prepare(cmd *exec.Cmd) {}
func own(cmd *exec.Cmd) (func() error, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, errors.New("cannot own metadata query process")
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(job)
		return nil, errors.New("cannot constrain metadata query process")
	}
	h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err == nil {
		err = windows.AssignProcessToJobObject(job, h)
		windows.CloseHandle(h)
	}
	if err != nil {
		windows.CloseHandle(job)
		return nil, errors.New("cannot bind metadata query process owner")
	}
	var once sync.Once
	return func() error { var err error; once.Do(func() { err = windows.CloseHandle(job) }); return err }, nil
}
