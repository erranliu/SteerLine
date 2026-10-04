package steerlineapp

import (
	"errors"

	"github.com/erranliu/SteerLine/internal/steerline"
	"github.com/erranliu/SteerLine/internal/steerline/persistence"
)

func NewCommandService(store persistence.CommandStore, config steerline.Config) (steerline.CommandService, error) {
	if store == nil || config.Identity() == "" || config.ExecutionDirectory == nil {
		return steerline.CommandService{}, errors.New("command construction requires a store, fixed project and execution directory")
	}
	work, results := taskBoundary(store)
	return steerline.NewCommandService(store, config.Identity(), work, results), nil
}
