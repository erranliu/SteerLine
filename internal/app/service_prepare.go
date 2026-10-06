package app

import (
	"context"
	"errors"

	steerline "github.com/erranliu/SteerLine/internal/app/host"
)

// PrepareServiceConfig performs center-owned admission and recovery checks
// before application composition opens and injects the writer resource.
func PrepareServiceConfig(c Config) (Config, error) {
	if c.ExecutionDirectory == nil || c.ProcessHost == nil {
		return c, errors.New("execution directory and process host are required")
	}
	var err error
	c, err = admitServiceConfig(c)
	if err != nil {
		return c, err
	}
	hostConfig, err := hostRuntimeConfig(context.Background(), c)
	if err != nil {
		return c, err
	}
	if err := steerline.PreflightNativeStatePath(hostConfig); err != nil {
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

func admitServiceConfig(c Config) (Config, error) {
	if err := c.ValidateAdmission(); err != nil {
		return c, err
	}
	return c, validateNativeStateLocation(c)
}
