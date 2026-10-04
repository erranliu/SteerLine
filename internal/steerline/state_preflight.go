package steerline

import (
	"context"
	"os"
	"path/filepath"

	"github.com/erranliu/SteerLine/internal/execution"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

func preflightWorkerFlowState(ctx context.Context, c execution.Config) error {
	if c.WorkflowProtocol != executionmodel.Version {
		return nil
	}
	path := filepath.Join(c.RunDir, "steerline.db")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	reader, err := OpenNativeTaskReader(ctx, &c)
	if err != nil {
		return err
	}
	defer reader.Close()
	payload, _, err := reader.LoadProjection(ctx, c.Identity())
	if err != nil {
		return err
	}
	if len(payload) != 0 {
		var runtime ServiceCheckpoint
		if err := decode(payload, &runtime); err != nil || runtime.SchemaVersion != WorkerFlowRuntimeSchema {
			return nativeErr("unsupported_schema", "worker-flow-v1 refuses the old runtime projection; preserve the original state and external session")
		}
	}
	snapshot, err := reader.Query(ctx, c.Identity())
	if err != nil {
		return err
	}
	return c.ValidateStoredExecutions(snapshot)
}
