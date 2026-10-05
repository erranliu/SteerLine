package steerlineapp

import (
	"context"
	"errors"

	"github.com/erranliu/SteerLine/internal/execution"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	"github.com/erranliu/SteerLine/internal/steerline"
	"github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	"github.com/erranliu/SteerLine/internal/steerline/sqlite"
)

// NewService uses the directory and process adapter already bound to the
// loaded application configuration.
func NewService(config execution.Config) (*execution.Service, error) {
	return NewServiceWithLifecycleResolver(config, config.ExecutionDirectory)
}

// NewServiceWithLifecycleResolver permits an explicit decorator around the
// same catalog's lifecycle ports, while preserving all other app wiring.
func NewServiceWithLifecycleResolver(config execution.Config, resolver executionbackend.LifecycleResolver) (*execution.Service, error) {
	return NewServiceWithPorts(config, resolver, codexworker.NewConversationAdapter())
}

// NewServiceWithPorts composes explicitly supplied execution and history ports
// with the directory already accepted by this application instance.
func NewServiceWithPorts(config execution.Config, resolver executionbackend.LifecycleResolver, sources contracts.ConversationSourcePort) (*execution.Service, error) {
	return newServiceWithHost(config, resolver, sources, nil)
}

func newConfiguredServiceWithHost(config execution.Config, host execution.HostPort) (*execution.Service, error) {
	return newServiceWithHost(config, config.ExecutionDirectory, codexworker.NewConversationAdapter(), host)
}

func newServiceWithHost(config execution.Config, resolver executionbackend.LifecycleResolver, sources contracts.ConversationSourcePort, host execution.HostPort) (*execution.Service, error) {
	if config.ExecutionDirectory == nil || config.ProcessHost == nil {
		return nil, errors.New("application configuration is missing its execution directory or process host")
	}
	config.Persistence = sqlite.Factory{}
	config.Resources = steerline.ExecutionResources{}
	config.ReadRuntime = func(_ context.Context, c execution.Config) (*contracts.Runtime, error) {
		return steerline.LoadDatabaseRuntime(c)
	}
	var err error
	config, err = steerline.PrepareServiceConfig(config)
	if err != nil {
		return nil, err
	}
	connection, err := steerline.OpenNativeTaskCommandStore(context.Background(), &config)
	if err != nil {
		return nil, err
	}
	tasks := NewTaskService(connection, config)
	if host == nil {
		host = steerline.NewServiceHost(config, "", "", nil)
	}
	service, err := execution.NewService(config, config.ExecutionDirectory, resolver, sources, config.ProcessHost, connection, tasks, tasks, host)
	if err != nil {
		_ = connection.Close()
	}
	return service, err

}
