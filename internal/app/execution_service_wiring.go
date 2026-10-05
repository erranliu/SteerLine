package app

import (
	"context"
	"errors"
	reconstructiontypes "github.com/erranliu/SteerLine/internal/execution/reconstruction"

	steerline "github.com/erranliu/SteerLine/internal/app/host"
	"github.com/erranliu/SteerLine/internal/execution"
	"github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
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
func NewServiceWithPorts(config execution.Config, resolver executionbackend.LifecycleResolver, sources execution.ConversationSourcePort) (*execution.Service, error) {
	return newServiceWithHost(config, resolver, sources, nil)
}

func newConfiguredServiceWithHost(config execution.Config, host execution.HostPort) (*execution.Service, error) {
	return newServiceWithHost(config, config.ExecutionDirectory, codexworker.NewConversationAdapter(), host)
}

func newServiceWithHost(config execution.Config, resolver executionbackend.LifecycleResolver, sources execution.ConversationSourcePort, host execution.HostPort) (*execution.Service, error) {
	if config.ExecutionDirectory == nil || config.ProcessHost == nil {
		return nil, errors.New("application configuration is missing its execution directory or process host")
	}
	config.Resources = executionResources{}
	config.ReadRuntime = func(_ context.Context, c execution.Config) (*reconstructiontypes.Runtime, error) {
		return LoadDatabaseRuntime(c)
	}
	configureNativeStorage(&config)
	var err error
	config, err = PrepareServiceConfig(config)
	if err != nil {
		return nil, err
	}
	connection, err := OpenNativeTaskCommandStore(context.Background(), &config)
	if err != nil {
		return nil, err
	}
	tasks := NewTaskService(connection, config)
	if host == nil {
		host = steerline.NewServiceHost(config, "", "", nil, serviceAdmission{}, consoleActivityFormatter{})
	}
	stores := execution.ServiceStorage{State: connection, Commands: connection, Worker: connection, Conversations: connection, Records: connection, Close: connection.Close}
	service, err := execution.NewService(config, config.ExecutionDirectory, resolver, sources, config.ProcessHost, stores, tasks, tasks, host)
	if err != nil {
		_ = connection.Close()
	}
	return service, err

}
