package app

import (
	"errors"

	"github.com/erranliu/SteerLine/internal/execution"
)

type commandWiringStore interface {
	execution.CommandStore
	taskStorage
}

func NewCommandService(store commandWiringStore, config Config) (execution.CommandService, error) {
	if store == nil || config.Identity() == "" || config.ExecutionDirectory == nil {
		return execution.CommandService{}, errors.New("command construction requires a store, fixed project and execution directory")
	}
	tasks := NewTaskService(taskCommandStore{store: store}, config)
	return execution.NewCommandServiceWithRuntime(store, config.Identity(), tasks, tasks, config.runtimeDependencies()), nil
}
