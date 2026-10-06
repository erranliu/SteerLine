package app

import (
	"context"
	"errors"
	"time"

	steerline "github.com/erranliu/SteerLine/internal/app/host"
	"github.com/erranliu/SteerLine/internal/execution"
	"github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	"github.com/erranliu/SteerLine/internal/execution/hostport"
	executionprocess "github.com/erranliu/SteerLine/internal/execution/process"
)

func NewProcessHost(timeout time.Duration) executionbackend.ManagedProcessHost {
	return executionprocess.New(timeout)
}

func StartService(ctx context.Context, configPath string, options steerline.StartServiceOptions) (steerline.ServiceStartResult, error) {
	config, err := LoadConfig(configPath)
	if err != nil {
		return steerline.ServiceStartResult{}, err
	}
	return StartConfiguredService(ctx, config, configPath, options)
}

func StartConfiguredService(ctx context.Context, config Config, configPath string, options steerline.StartServiceOptions) (steerline.ServiceStartResult, error) {
	if config.ExecutionDirectory == nil || config.ProcessHost == nil {
		return steerline.ServiceStartResult{}, errors.New("execution directory and process host are required")
	}
	if err := config.ValidateTaskCommandConfiguration(); err != nil {
		return steerline.ServiceStartResult{}, err
	}
	if err := config.RequireNativeTaskAuthority(); err != nil {
		return steerline.ServiceStartResult{}, err
	}
	hostConfig, err := hostRuntimeConfig(ctx, config)
	if err != nil {
		return steerline.ServiceStartResult{}, err
	}
	if options.ApplicationHome == "" {
		options.ApplicationHome = HomeDir()
	}
	if options.ApplicationBin == "" {
		options.ApplicationBin = BinDir()
	}
	return steerline.StartService(ctx, hostConfig, configPath, options)
}

func RunConfiguredService(ctx context.Context, config Config, mode, startupID, console string) error {
	return runConfiguredService(ctx, config, mode, startupID, console, nil)
}

func runConfiguredService(ctx context.Context, config Config, mode, startupID, console string, onConstruct func(*execution.Service)) error {
	if config.ExecutionDirectory == nil || config.ProcessHost == nil {
		return errors.New("execution directory and process host are required")
	}
	if err := config.ValidateTaskCommandConfiguration(); err != nil {
		return err
	}
	if err := config.RequireNativeTaskAuthority(); err != nil {
		return err
	}
	hostConfig, err := hostRuntimeConfig(ctx, config)
	if err != nil {
		return err
	}
	admission := serviceAdmission{config: &config}
	factory := func(input steerline.Config, hostPort hostport.HostPort, id string) (steerline.ServiceLifetime, error) {
		if err := admission.syncHostFacts(input); err != nil {
			return nil, err
		}
		service, err := newConfiguredServiceWithHost(config, hostPort)
		if err != nil {
			return nil, err
		}
		if onConstruct != nil {
			onConstruct(service)
		}
		service.SetStartupMetadata(id)
		return executionServiceLifetime{service: service, config: input, mode: mode}, nil
	}
	return steerline.RunConfiguredService(ctx, hostConfig, mode, startupID, console, factory, admission, consoleActivityFormatter{})
}

func RunSessionLauncher(ctx context.Context, requestPath string, processHost executionbackend.ManagedProcessHost) error {
	if processHost == nil {
		return errors.New("managed process host is required")
	}
	return codexworker.RunSessionLauncher(ctx, requestPath, processHost.StartManaged, processHost.WriteExecutionFile)
}

func InstallLaunchers() ([]string, error) { return steerline.InstallLaunchers(HomeDir()) }
