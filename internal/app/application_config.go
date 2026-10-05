package app

import (
	"context"
	"github.com/erranliu/SteerLine/internal/app/host"
	"github.com/erranliu/SteerLine/internal/execution"
)

func ValidateApplicationPaths(c execution.Config) error {
	if err := validateNativeStateLocation(c); err != nil {
		return err
	}
	return PreflightNativeStatePath(c)
}

func PreflightNativeStatePath(c execution.Config) error {
	hostConfig, err := hostRuntimeConfig(context.Background(), c)
	if err != nil {
		return err
	}
	return host.PreflightNativeStatePath(hostConfig)
}
