package steerlineapp

import (
	"context"
	"errors"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline"
	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"github.com/erranliu/SteerLine/internal/steerline/processhost"
)

func NewProcessHost(timeout time.Duration) executionapi.ManagedProcessHost {
	return processhost.New(timeout)
}

// NewService uses the directory and process adapter already bound to the
// loaded application configuration.
func NewService(config steerline.Config) (*steerline.Service, error) {
	return NewServiceWithLifecycleResolver(config, config.ExecutionDirectory)
}

// NewServiceWithLifecycleResolver permits an explicit decorator around the
// same catalog's lifecycle ports, while preserving all other app wiring.
func NewServiceWithLifecycleResolver(config steerline.Config, resolver executionapi.LifecycleResolver) (*steerline.Service, error) {
	if config.ExecutionDirectory == nil || config.ProcessHost == nil {
		return nil, errors.New("application configuration is missing its execution directory or process host")
	}
	sources := codexworker.NewConversationAdapter()
	return steerline.NewService(config, config.ExecutionDirectory, resolver, sources, config.ProcessHost)
}

func StartService(ctx context.Context, configPath string, options steerline.StartServiceOptions) (steerline.ServiceStartResult, error) {
	config, err := LoadConfig(configPath)
	if err != nil {
		return steerline.ServiceStartResult{}, err
	}
	return StartConfiguredService(ctx, config, configPath, options)
}

func StartConfiguredService(ctx context.Context, config steerline.Config, configPath string, options steerline.StartServiceOptions) (steerline.ServiceStartResult, error) {
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

func NewCommandService(store *steerline.SQLiteStore, config steerline.Config) (steerline.CommandService, error) {
	if store == nil || config.Identity() == "" || config.ExecutionDirectory == nil {
		return steerline.CommandService{}, errors.New("command construction requires a store, fixed project and execution directory")
	}
	return steerline.CommandService{Store: store, Project: config.Identity()}, nil
}

func RunConfiguredService(ctx context.Context, config steerline.Config, mode, startupID, console string) error {
	return steerline.RunConfiguredService(ctx, config, mode, startupID, console, NewService)
}

func RunSessionLauncher(ctx context.Context, requestPath string, processHost executionapi.ManagedProcessHost) error {
	if processHost == nil {
		return errors.New("managed process host is required")
	}
	return codexworker.RunSessionLauncher(ctx, requestPath, processHost.StartManaged, processHost.WriteExecutionFile)
}

func InstallLaunchers() ([]string, error) { return steerline.InstallLaunchers(HomeDir()) }
