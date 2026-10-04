package steerline

import (
	"github.com/erranliu/SteerLine/internal/execution"
	"github.com/erranliu/SteerLine/internal/steerline/persistence"
	taskapp "github.com/erranliu/SteerLine/internal/task"
)

type taskTestCommands struct{ execution.CommandService }

func newTestCommandService(store persistence.CommandStore, project string) taskTestCommands {
	tasks := taskapp.Service{Reader: store}
	return taskTestCommands{execution.NewCommandService(store, project, tasks, tasks)}
}
