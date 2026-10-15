//go:build !windows

package httpclient

import (
	"errors"
	wire "github.com/erranliu/SteerLine/internal/product/httpcontract"
)

func observeExit(wire.Identity) (bool, bool, error) {
	return false, false, errors.New("local process/lock exit observation unsupported on this platform")
}

func CurrentProcessIdentity() (wire.ProcessIdentity, error) {
	return wire.ProcessIdentity{}, errors.New("local process identity unsupported on this platform")
}
func ProcessExited(wire.ProcessIdentity) (bool, error) {
	return false, errors.New("local process observation unsupported on this platform")
}
