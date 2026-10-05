//go:build windows

package process

import (
	"errors"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	"golang.org/x/sys/windows"
)

func (ServiceFacts) CurrentIdentity() (*executionbackend.ProcessIdentity, error) {
	return CurrentIdentity()
}

func (ServiceFacts) IdentityForPID(pid int) (executionbackend.ProcessIdentity, error) {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return executionbackend.ProcessIdentity{}, err
	}
	defer windows.CloseHandle(handle)
	return IdentityFromHandle(handle, pid)
}

func (ServiceFacts) IdentityFromHandle(handle uintptr, pid int) (executionbackend.ProcessIdentity, error) {
	return IdentityFromHandle(windows.Handle(handle), pid)
}

func (ServiceFacts) SessionID(int) (int, error) {
	return 0, errors.New("process sessions are not used on Windows")
}
