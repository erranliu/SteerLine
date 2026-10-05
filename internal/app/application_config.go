package app

import (
	"github.com/erranliu/SteerLine/internal/execution"
	steerline "github.com/erranliu/SteerLine/internal/app/host"
)

func ValidateApplicationPaths(c execution.Config) error {
	if err := validateNativeStateLocation(c); err != nil {
		return err
	}
	return steerline.PreflightNativeStatePath(c)
}
