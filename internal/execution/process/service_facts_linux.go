//go:build linux

package process

import (
	"errors"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func (ServiceFacts) CurrentIdentity() (*executionbackend.ProcessIdentity, error) {
	return CurrentIdentity()
}

func (ServiceFacts) IdentityForPID(pid int) (executionbackend.ProcessIdentity, error) {
	return IdentityForPID(pid)
}

func (ServiceFacts) IdentityFromHandle(uintptr, int) (executionbackend.ProcessIdentity, error) {
	return executionbackend.ProcessIdentity{}, errors.New("process handles are unavailable on Linux")
}

func (ServiceFacts) SessionID(pid int) (int, error) { return SessionID(pid) }
