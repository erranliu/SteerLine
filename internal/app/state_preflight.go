package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/erranliu/SteerLine/internal/execution"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/execution/reconstruction"
	steerline "github.com/erranliu/SteerLine/internal/app/host"
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
	if err := steerline.PreflightNativeStatePath(c); err != nil {
		return err
	}
	if c.ReadPersistedExecutionState == nil {
		return errors.New("application persisted execution reader is required for worker-flow preflight")
	}
	state, err := c.ReadPersistedExecutionState(ctx, c)
	if err != nil {
		return err
	}
	project := c.Identity()
	if state.Records.ProjectIdentity != project || state.Graph.Authority.ProjectIdentity != project {
		return errors.New("persisted execution snapshot belongs to another project")
	}
	return c.ValidateStoredExecutions(reconstruction.SnapshotFromStorage(state))
}
