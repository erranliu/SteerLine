//go:build !windows

package native

import (
	"errors"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
)

func DisconnectCandidate(contracts.ExecutionRecord, string) error {
	return errors.New("candidate native fault requires the Windows E2E environment")
}
