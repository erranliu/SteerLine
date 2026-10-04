package steerlineapp

import (
	"time"

	"github.com/erranliu/SteerLine/internal/execution"
	"github.com/erranliu/SteerLine/internal/steerline"
	taskapp "github.com/erranliu/SteerLine/internal/task"
)

// NewTaskService constructs every task capability from minimal input and narrow
// readers. Full application configuration remains in this app adapter.
func NewTaskService(reader taskapp.Reader, config execution.Config) taskapp.Service {
	timeout, _ := time.ParseDuration(config.CommandTimeout)
	return taskapp.Service{
		Reader: reader,
		Input: taskapp.Input{Project: config.Identity(), Authority: config.TaskAuthority,
			Remote: config.Remote, Branch: config.Branch,
			ExecutableTypes:    append([]string(nil), config.ExecutableTypes...),
			DefaultRequestedBy: steerline.DefaultRequestedBy()},
		Documents: taskDocuments{target: config.TaskRepositoryTarget,
			repository: config.Repository, git: config.Git, timeout: timeout, processHost: config.ProcessHost},
	}
}
