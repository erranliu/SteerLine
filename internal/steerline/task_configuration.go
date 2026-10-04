package steerline

import (
	"context"
	"errors"
)

// ValidateTaskCommandConfiguration validates the existing application admission
// settings before app extracts a task's small input. It grants no execution.
func (c Config) ValidateTaskCommandConfiguration() error {
	return validateWorkflowConfig(&c, nil)
}

// TaskRepositoryTarget returns the validated publication identity for task
// input construction; task never receives the Git command or provider config.
func (c Config) TaskRepositoryTarget(ctx context.Context) (string, error) {
	return configuredRepositoryTarget(ctx, c)
}

func requireNativeTaskAuthority(c Config) error {
	if c.TaskAuthority != "native" {
		return errors.New("task_authority=native is required; Beads is not an active product task source")
	}
	return nil
}
