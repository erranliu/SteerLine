//go:build !windows && !linux

package process

import (
	"errors"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

func (ServiceFacts) CurrentIdentity() (*executionbackend.ProcessIdentity, error) {
	return nil, errors.New("process identity is unsupported on this operating system")
}

func (ServiceFacts) IdentityForPID(int) (executionbackend.ProcessIdentity, error) {
	return executionbackend.ProcessIdentity{}, errors.New("process identity is unsupported on this operating system")
}

func (ServiceFacts) IdentityFromHandle(uintptr, int) (executionbackend.ProcessIdentity, error) {
	return executionbackend.ProcessIdentity{}, errors.New("process handles are unsupported on this operating system")
}

func (ServiceFacts) SessionID(int) (int, error) {
	return 0, errors.New("process sessions are unsupported on this operating system")
}
