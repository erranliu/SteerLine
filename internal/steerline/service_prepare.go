package steerline

import (
	"context"
	"errors"

	"github.com/erranliu/SteerLine/internal/execution"
)

// PrepareServiceConfig performs center-owned admission and recovery checks
// before application composition opens and injects the writer resource.
func PrepareServiceConfig(c execution.Config) (execution.Config, error) {
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
	if e := c.PrepareWorktreeRoot(); e != nil {
		return c, e
	}

	return c, nil
}

func admitServiceConfig(c execution.Config) (execution.Config, error) {
	if err := c.ValidateAdmission(); err != nil {
		return c, err
	}
	return c, validateNativeStateLocation(c)
}
