package app

import (
	"context"
	"github.com/erranliu/SteerLine/internal/app/host"
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
	return host.PreflightNativeStatePath(hostConfig)
}
