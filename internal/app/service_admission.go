package app

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/erranliu/SteerLine/internal/app/host"
)

// serviceAdmission binds the host's small startup facts back to this app's
// configuration. The host never receives the broad execution configuration.
type serviceAdmission struct{ config *Config }

func (a serviceAdmission) syncHostFacts(input host.Config) error {
	if a.config == nil {
		return errors.New("application service configuration is required")
	}
	return syncExecutionHostFacts(a.config, input)
}

func syncExecutionHostFacts(config *Config, input host.Config) error {
	if err := config.ResolveIdentity(context.Background()); err != nil {
		return err
	}
	if config.Identity() != input.Identity() {
		return errors.New("host repository identity differs from app configuration")
	}
	config.Repository = input.Repository
	config.Remote = input.Remote
	config.Branch = input.Branch
	config.RunDir = input.RunDir
	config.Claimant = input.Claimant
	return nil
}

func (a serviceAdmission) ValidateStateLocation(input host.Config) error {
	if err := a.syncHostFacts(input); err != nil {
		return err
	}
	if err := validateNativeStateLocation(*a.config); err != nil {
		return err
	}
	// This read-only gate rejects Beads and unsupported files/formats before the
	// host publishes startup.json or opens the service log.
	return PreflightNativeStatePath(*a.config)
}

func (a serviceAdmission) Preflight(ctx context.Context, input host.Config, checkSkills bool) (host.Config, error) {
	if err := a.syncHostFacts(input); err != nil {
		return input, err
	}
	if err := Preflight(ctx, a.config, checkSkills); err != nil {
		return input, err
	}
	return hostRuntimeConfig(ctx, *a.config)
}

func (a serviceAdmission) InitializeNativeProject(ctx context.Context, input host.Config) error {
	if err := a.syncHostFacts(input); err != nil {
		return err
	}
	store, err := OpenNativeTaskProject(ctx, a.config)
	if err != nil {
		return err
	}
	return store.Close()
}

func validateNativeStore(ctx context.Context, config Config) error {
	path := filepath.Join(config.RunDir, "steerline.db")
	reader, err := openBoundNativeReader(ctx, config, path)
	if err != nil {
		return err
	}
	return reader.Close()
}

var _ host.ApplicationAdmission = serviceAdmission{}
