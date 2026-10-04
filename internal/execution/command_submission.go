package execution

import (
	"context"
	"errors"

	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
)

func (c CommandService) Submit(ctx context.Context, command storageports.CommandRecord) (storageports.CommandRecord, bool, error) {
	if c.Store == nil || c.Project == "" {
		return command, false, errors.New("command service store and project identity are required")
	}
	command.ProjectIdentity = c.Project
	return c.Store.EnqueueCommand(ctx, command)
}

func (c CommandService) Pending(ctx context.Context, limit int) ([]storageports.CommandRecord, error) {
	if c.Store == nil || c.Project == "" {
		return nil, errors.New("command service store and project identity are required")
	}
	return c.Store.Commands(ctx, c.Project, "pending", limit)
}

func (c CommandService) Complete(ctx context.Context, command storageports.CommandRecord, result []byte) error {
	if c.Store == nil || c.Project == "" {
		return errors.New("command service store and project identity are required")
	}
	command.ProjectIdentity = c.Project
	command.Result = append([]byte(nil), result...)
	command.Status = "completed"
	return c.Store.UpdateCommand(ctx, command)
}

func (c CommandService) SetStatus(ctx context.Context, command storageports.CommandRecord, status string, result []byte) error {
	if c.Store == nil || c.Project == "" {
		return errors.New("command service store and project identity are required")
	}
	command.ProjectIdentity = c.Project
	command.Status = status
	command.Result = append([]byte(nil), result...)
	return c.Store.UpdateCommand(ctx, command)
}
