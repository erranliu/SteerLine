//go:build !windows

package native

import (
	"errors"
)

func DisconnectCandidate(ProcessFaultTarget, string) error {
	return errors.New("candidate native fault requires the Windows E2E environment")
}
