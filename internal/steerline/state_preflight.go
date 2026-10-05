package steerline

import (
	"context"
	"os"
	"path/filepath"

	"github.com/erranliu/SteerLine/internal/execution"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

func preflightWorkerFlowState(_ context.Context, c execution.Config) error {
	if c.WorkflowProtocol != executionmodel.Version {
		return nil
	}
	path := filepath.Join(c.RunDir, "steerline.db")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	return preflightNativeStatePath(c)
}
