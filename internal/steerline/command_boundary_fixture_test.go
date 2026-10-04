package steerline

import (
	"github.com/erranliu/SteerLine/internal/steerline/persistence"
	taskapp "github.com/erranliu/SteerLine/internal/task"
)

func newTestCommandService(store persistence.CommandStore, project string) CommandService {
	tasks := taskapp.Service{Reader: store}
	return NewCommandService(store, project, tasks, tasks)
}
