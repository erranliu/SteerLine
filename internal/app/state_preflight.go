package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/execution/reconstruction"
)

func preflightWorkerFlowState(ctx context.Context, c Config) error {
	if c.WorkflowProtocol != executionmodel.Version {
		return nil
	}
	path := filepath.Join(c.RunDir, "steerline.db")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	if err := PreflightNativeStatePath(c); err != nil {
		return err
	}
	reader, err := OpenNativeTaskReader(ctx, &c)
	if err != nil {
		return err
	}
	defer reader.Close()
	state, err := reader.ReadExecutionStateSnapshot(ctx, c.Identity())
	if err != nil {
		return err
	}
	project := c.Identity()
	if state.Records.ProjectIdentity != project || state.Graph.Authority.ProjectIdentity != project {
		return errors.New("persisted execution snapshot belongs to another project")
	}
	return c.ValidateStoredExecutions(reconstruction.SnapshotFromStorage(state))
}
