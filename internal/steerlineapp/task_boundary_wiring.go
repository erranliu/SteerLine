package steerlineapp

import (
	taskapp "github.com/erranliu/SteerLine/internal/task"
)

// Task construction owns only its reader. The executing consumer receives the
// result declaration capability separately from product command adapters.
func taskBoundary(reader taskapp.Reader) (taskapp.WorkPort, taskapp.ResultPort) {
	tasks := taskapp.Service{Reader: reader}
	return tasks, tasks
}
