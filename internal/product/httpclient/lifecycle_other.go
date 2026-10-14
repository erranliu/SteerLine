//go:build !windows

package httpclient

import (
	"errors"
	wire "github.com/erranliu/SteerLine/internal/product/httpcontract"
)

func observeExit(wire.Identity) (bool, bool, error) {
	return false, false, errors.New("local process/lock exit observation unsupported on this platform")
}
