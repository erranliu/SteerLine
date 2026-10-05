package app

import (
	"context"
	"errors"

	"github.com/erranliu/SteerLine/internal/app/host"
	"github.com/erranliu/SteerLine/internal/execution"
)

type serviceAdmission struct{}

func (serviceAdmission) ValidateStateLocation(config execution.Config) error {
	return validateNativeStateLocation(config)
}

func (serviceAdmission) Preflight(ctx context.Context, config *execution.Config, checkSkills bool) error {
	return Preflight(ctx, config, checkSkills)
}

func (serviceAdmission) InitializeNativeProject(ctx context.Context, config execution.Config) error {
	if config.EnsureNativeProject == nil {
		return errors.New("application native project initializer is required")
	}
	return config.EnsureNativeProject(ctx, config)
}

var _ host.ApplicationAdmission = serviceAdmission{}
