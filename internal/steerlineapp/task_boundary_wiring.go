package steerlineapp

import (
	"time"

	"github.com/erranliu/SteerLine/internal/steerline"
	taskapp "github.com/erranliu/SteerLine/internal/task"
)

// Returns actual values so the execution consumer owns its interface definition.
// Existing main consumers use only work/result declarations. #76 switches them
// to NewTaskService when it adds the fixed-document capability to its work port.
func taskBoundary(reader taskapp.Reader) (taskapp.Service, taskapp.Service) {
	tasks := taskapp.Service{Reader: reader}
	return tasks, tasks
}

// NewTaskService constructs every task capability from minimal input and narrow
// readers. Full application configuration remains in this app adapter.
func NewTaskService(reader taskapp.Reader, config steerline.Config) taskapp.Service {
	timeout, _ := time.ParseDuration(config.CommandTimeout)
	return taskapp.Service{
		Reader: reader,
		Input: taskapp.Input{Project: config.Identity(), Authority: config.TaskAuthority,
			Remote: config.Remote, Branch: config.Branch,
			ExecutableTypes:    append([]string(nil), config.ExecutableTypes...),
			DefaultRequestedBy: steerline.DefaultRequestedBy()},
		Documents: taskDocuments{target: config.TaskRepositoryTarget,
			repository: config.Repository, git: config.Git, timeout: timeout},
	}
}
