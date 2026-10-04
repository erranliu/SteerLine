package steerline

import (
	"context"
	"errors"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

// PrepareServiceConfig performs center-owned admission and recovery checks
// before application composition opens and injects the writer resource.
func PrepareServiceConfig(c Config) (Config, error) {
	if c.ExecutionDirectory == nil || c.ProcessHost == nil || c.Persistence == nil {
		return c, errors.New("execution directory, process host and persistence ports are required")
	}
	var err error
	c, err = admitServiceConfig(c)
	if err != nil {
		return c, err
	}
	if err := preflightNativeStatePath(c); err != nil {
		return c, err
	}
	if err := preflightWorkerFlowState(context.Background(), c); err != nil {
		return c, err
	}
	if e := c.prepareWorktreeRoot(); e != nil {
		return c, e
	}

	return c, nil
}

func admitServiceConfig(c Config) (Config, error) {
	if err := requireNativeTaskAuthority(c); err != nil {
		return c, err
	}
	if c.WorkflowProtocol != executionmodel.Version {
		return c, nativeErr("unsupported_schema", "only worker-flow-v1 may start a service; preserve older execution state")
	}
	if err := validateWorkflowConfig(&c, nil); err != nil {
		return c, err
	}
	if err := validateExecutionProfiles(c); err != nil {
		return c, err
	}
	if err := c.ResolveIdentity(context.Background()); err != nil {
		return c, err
	}
	if err := validateNativeStateLocation(c); err != nil {
		return c, err
	}

	return c, nil
}
