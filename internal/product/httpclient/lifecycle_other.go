//go:build !windows

package httpclient

import (
	"errors"
	wire "github.com/erranliu/SteerLine/internal/product/httpcontract"
)

func CurrentProcessIdentity() (wire.ProcessIdentity, error) {
	return wire.ProcessIdentity{}, errors.New("local process identity unsupported on this platform")
}
func ProcessExited(wire.ProcessIdentity) (bool, error) {
	return false, errors.New("local process observation unsupported on this platform")
}
