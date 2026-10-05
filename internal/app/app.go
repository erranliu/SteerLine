package app

import (
	"context"
	"errors"
	"time"

	steerline "github.com/erranliu/SteerLine/internal/app/host"
	"github.com/erranliu/SteerLine/internal/execution"
	"github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
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

func StartConfiguredService(ctx context.Context, config execution.Config, configPath string, options steerline.StartServiceOptions) (steerline.ServiceStartResult, error) {
	if config.ExecutionDirectory == nil || config.ProcessHost == nil {
		return steerline.ServiceStartResult{}, errors.New("execution directory and process host are required")
	}
	if options.ApplicationHome == "" {
		options.ApplicationHome = HomeDir()
	}
	if options.ApplicationBin == "" {
		options.ApplicationBin = BinDir()
	}
	return steerline.StartService(ctx, config, configPath, options)
}

func RunConfiguredService(ctx context.Context, config execution.Config, mode, startupID, console string) error {
	return steerline.RunConfiguredService(ctx, config, mode, startupID, console, newConfiguredServiceWithHost, serviceAdmission{}, consoleActivityFormatter{})
}

func RunSessionLauncher(ctx context.Context, requestPath string, processHost executionbackend.ManagedProcessHost) error {
	if processHost == nil {
		return errors.New("managed process host is required")
	}
	return codexworker.RunSessionLauncher(ctx, requestPath, processHost.StartManaged, processHost.WriteExecutionFile)
}

func InstallLaunchers() ([]string, error) { return steerline.InstallLaunchers(HomeDir()) }
