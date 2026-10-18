//go:build !windows && !linux

package process

import (
	"errors"

	"github.com/erranliu/SteerLine/internal/hostcontract"
)

func (ServiceFacts) CurrentIdentity() (*hostcontract.ProcessIdentity, error) {
	return nil, errors.New("process identity is unsupported on this operating system")
}

func (ServiceFacts) IdentityForPID(int) (hostcontract.ProcessIdentity, error) {
	return hostcontract.ProcessIdentity{}, errors.New("process identity is unsupported on this operating system")
}

func (ServiceFacts) IdentityFromHandle(uintptr, int) (hostcontract.ProcessIdentity, error) {
	return hostcontract.ProcessIdentity{}, errors.New("process handles are unsupported on this operating system")
}

func (ServiceFacts) SessionID(int) (int, error) {
	return 0, errors.New("process sessions are unsupported on this operating system")
}
