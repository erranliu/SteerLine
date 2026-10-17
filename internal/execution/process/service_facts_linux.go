//go:build linux

package process

import (
	"errors"

	"github.com/erranliu/SteerLine/internal/hostcontract"
)

func (ServiceFacts) CurrentIdentity() (*hostcontract.ProcessIdentity, error) {
	return CurrentIdentity()
}

func (ServiceFacts) IdentityForPID(pid int) (hostcontract.ProcessIdentity, error) {
	return IdentityForPID(pid)
}

func (ServiceFacts) IdentityFromHandle(uintptr, int) (hostcontract.ProcessIdentity, error) {
	return hostcontract.ProcessIdentity{}, errors.New("process handles are unavailable on Linux")
}

func (ServiceFacts) SessionID(pid int) (int, error) { return SessionID(pid) }
