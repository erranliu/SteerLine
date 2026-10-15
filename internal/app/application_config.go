package app

import (
	"context"
	"fmt"
	"github.com/erranliu/SteerLine/internal/app/host"
	"github.com/erranliu/SteerLine/internal/product/httpapi"
)

func ValidateApplicationPaths(c Config) error {
	if err := validateNativeStateLocation(c); err != nil {
		return err
	}
	return PreflightNativeStatePath(c)
}

func PreflightNativeStatePath(c Config) error {
	hostConfig, err := hostRuntimeConfig(context.Background(), c)
	if err != nil {
		return err
	}
	if err := host.PreflightNativeStatePath(hostConfig); err != nil {
		return err
	}
	if err := httpapi.ValidateStateDirectory(c.RunDir, c.Identity()); err != nil {
		return fmt.Errorf("local API state: %w", err)
	}
	return httpapi.ValidateProjectCommandDirectory(c.RunDir, c.Identity())
}
