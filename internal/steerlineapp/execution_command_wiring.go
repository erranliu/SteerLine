package steerlineapp

import (
	"errors"

	"github.com/erranliu/SteerLine/internal/execution"
	"github.com/erranliu/SteerLine/internal/steerline/persistence"
)

func NewCommandService(store persistence.CommandStore, config execution.Config) (execution.CommandService, error) {
	if store == nil || config.Identity() == "" || config.ExecutionDirectory == nil {
		return execution.CommandService{}, errors.New("command construction requires a store, fixed project and execution directory")
	}
	tasks := NewTaskService(store, config)
	return execution.NewCommandService(store, config.Identity(), tasks, tasks), nil
}
