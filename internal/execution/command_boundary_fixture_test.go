package execution

import (
	"github.com/erranliu/SteerLine/internal/steerline/persistence"
	taskapp "github.com/erranliu/SteerLine/internal/task"
)

func newTestCommandService(store persistence.CommandStore, config Config) CommandService {
	commands := NewTaskCommands(store, config)
	tasks := taskapp.Service{Reader: store, Input: commands.Input, Documents: commands.Documents}
	return NewCommandService(store, config.Identity(), tasks, tasks)
}
