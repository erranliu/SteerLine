package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/runnerclient"
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
	state, err := reader.ReadRunnerSnapshot(ctx, c.Identity())
	if err != nil {
		return err
	}
	project := c.Identity()
	if state.Graph.Authority.ProjectIdentity != project {
		return errors.New("persisted task snapshot belongs to another project")
	}
	for _, candidate := range state.Candidates {
		if candidate.Project != project || canonicalPath(candidate.StateDir) != canonicalPath(filepath.Join(c.RunDir, "runner")) || candidate.Submit.Protocol != runnerclient.Protocol || candidate.Submit.RequestID == "" {
			return errors.New("persisted Runner candidate has a different caller or state directory binding")
		}
	}
	return nil
}
