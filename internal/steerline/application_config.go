package steerline

import (
 "github.com/erranliu/SteerLine/internal/execution")

func ValidateApplicationPaths(c execution.Config) error {
	if err := validateNativeStateLocation(c); err != nil {
		return err
	}
	return preflightNativeStatePath(c)
}
