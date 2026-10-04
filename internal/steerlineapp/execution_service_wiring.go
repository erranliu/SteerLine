package steerlineapp

import (
	"context"
	"errors"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	"github.com/erranliu/SteerLine/internal/steerline"
	"github.com/erranliu/SteerLine/internal/steerline/codexworker"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	"github.com/erranliu/SteerLine/internal/steerline/sqlite"
)

// NewService uses the directory and process adapter already bound to the
// loaded application configuration.
func NewService(config steerline.Config) (*steerline.Service, error) {
	return NewServiceWithLifecycleResolver(config, config.ExecutionDirectory)
}

// NewServiceWithLifecycleResolver permits an explicit decorator around the
// same catalog's lifecycle ports, while preserving all other app wiring.
func NewServiceWithLifecycleResolver(config steerline.Config, resolver executionbackend.LifecycleResolver) (*steerline.Service, error) {
	return NewServiceWithPorts(config, resolver, codexworker.NewConversationAdapter())
}

// NewServiceWithPorts composes explicitly supplied execution and history ports
// with the directory already accepted by this application instance.
func NewServiceWithPorts(config steerline.Config, resolver executionbackend.LifecycleResolver, sources contracts.ConversationSourcePort) (*steerline.Service, error) {
	if config.ExecutionDirectory == nil || config.ProcessHost == nil {
		return nil, errors.New("application configuration is missing its execution directory or process host")
	}
	config.Persistence = sqlite.Factory{}
	var err error
	config, err = steerline.PrepareServiceConfig(config)
	if err != nil {
		return nil, err
	}
	connection, err := steerline.OpenNativeTaskCommandStore(context.Background(), &config)
	if err != nil {
		return nil, err
	}
	work, results := taskBoundary(connection)
	service, err := steerline.NewService(config, config.ExecutionDirectory, resolver, sources, config.ProcessHost, connection, work, results)
	if err != nil {
		_ = connection.Close()
	}
	return service, err

}
