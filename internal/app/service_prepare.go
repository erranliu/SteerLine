package app

import (
	"context"
	"errors"
)

// PrepareServiceConfig performs center-owned admission and recovery checks
// before application composition opens and injects the writer resource.
func PrepareServiceConfig(c Config) (Config, error) {
	if c.ProcessHost == nil {
		return c, errors.New("service process host is required")
	}
	var err error
	c, err = admitServiceConfig(c)
	if err != nil {
		return c, err
	}
	if err := PreflightNativeStatePath(c); err != nil {
		return c, err
	}
	if err := preflightWorkerFlowState(context.Background(), c); err != nil {
		return c, err
	}

	return c, nil
}

func admitServiceConfig(c Config) (Config, error) {
	if err := c.ValidateAdmission(); err != nil {
		return c, err
	}
	return c, validateNativeStateLocation(c)
}
