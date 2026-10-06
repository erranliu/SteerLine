package app

import (
	"context"
	"errors"
	steerline "github.com/erranliu/SteerLine/internal/app/host"
	"github.com/erranliu/SteerLine/internal/execution"
	"github.com/erranliu/SteerLine/internal/execution/adapters/codex"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	conversationtypes "github.com/erranliu/SteerLine/internal/execution/conversation"
	"github.com/erranliu/SteerLine/internal/execution/hostport"
)

type executionServiceLifetime struct {
	service *execution.Service
	config  steerline.Config
	mode    string
}

func (r executionServiceLifetime) Run(ctx context.Context) error {
	return steerline.RunService(ctx, r.service, r.config, r.mode)
}

func (r executionServiceLifetime) Close() error { return r.service.Close() }

// NewService uses the directory and process adapter already bound to the
// loaded application configuration.
func NewService(config Config) (*execution.Service, error) {
	return NewServiceWithLifecycleResolver(config, config.ExecutionDirectory)
}

// NewServiceWithLifecycleResolver permits an explicit decorator around the
// same catalog's lifecycle ports, while preserving all other app wiring.
func NewServiceWithLifecycleResolver(config Config, resolver executionbackend.LifecycleResolver) (*execution.Service, error) {
	return NewServiceWithPorts(config, resolver, codexworker.NewConversationAdapter())
}

// NewServiceWithPorts composes explicitly supplied execution and history ports
// with the directory already accepted by this application instance.
func NewServiceWithPorts(config Config, resolver executionbackend.LifecycleResolver, sources conversationtypes.ConversationSourcePort) (*execution.Service, error) {
	return newServiceWithHost(config, resolver, sources, nil)
}

func newConfiguredServiceWithHost(config Config, host hostport.HostPort) (*execution.Service, error) {
	return newServiceWithHost(config, config.ExecutionDirectory, codexworker.NewConversationAdapter(), host)
}

func newServiceWithHost(config Config, resolver executionbackend.LifecycleResolver, sources conversationtypes.ConversationSourcePort, host hostport.HostPort) (*execution.Service, error) {
	if config.ExecutionDirectory == nil || config.ProcessHost == nil {
		return nil, errors.New("application configuration is missing its execution directory or process host")
	}
	var err error
	config, err = PrepareServiceConfig(config)
	if err != nil {
		return nil, err
	}
	hostConfig, err := hostRuntimeConfig(context.Background(), config)
	if err != nil {
		return nil, err
	}
	connection, err := OpenNativeTaskCommandStore(context.Background(), &config)
	if err != nil {
		return nil, err
	}
	tasks := NewTaskService(connection, config)
	if host == nil {
		host = steerline.NewServiceHost(hostConfig, "", "", nil, serviceAdmission{config: &config}, consoleActivityFormatter{})
	}
	stores := execution.ServiceStorage{State: connection, Commands: connection, Worker: connection, Conversations: connection, Records: connection, Resources: executionResources{}, Close: connection.Close}
	service, err := execution.NewService(config.executionConfig(), config.runtimeDependencies(), config.runtimeSettings(), resolver, sources, stores, tasks, tasks, host)
	return service, err

}
