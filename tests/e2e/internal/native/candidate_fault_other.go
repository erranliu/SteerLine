//go:build !windows

package native

import (
	"errors"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

func DisconnectCandidate(executionmodel.ExecutionRecord, string) error {
	return errors.New("candidate native fault requires the Windows E2E environment")
}
