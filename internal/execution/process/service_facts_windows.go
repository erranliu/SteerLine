//go:build windows

package process

import (
	"errors"

	"github.com/erranliu/SteerLine/internal/hostcontract"
	"golang.org/x/sys/windows"
)

func (ServiceFacts) CurrentIdentity() (*hostcontract.ProcessIdentity, error) {
	return CurrentIdentity()
}

func (ServiceFacts) IdentityForPID(pid int) (hostcontract.ProcessIdentity, error) {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return hostcontract.ProcessIdentity{}, err
	}
	defer windows.CloseHandle(handle)
	return IdentityFromHandle(handle, pid)
}

func (ServiceFacts) IdentityFromHandle(handle uintptr, pid int) (hostcontract.ProcessIdentity, error) {
	return IdentityFromHandle(windows.Handle(handle), pid)
}

func (ServiceFacts) SessionID(int) (int, error) {
	return 0, errors.New("process sessions are not used on Windows")
}
