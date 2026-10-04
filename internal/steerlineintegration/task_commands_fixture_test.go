package steerlineintegration

import (
	"context"
	"github.com/erranliu/SteerLine/internal/steerline"
	"github.com/erranliu/SteerLine/internal/steerline/persistence"
	steerlineapp "github.com/erranliu/SteerLine/internal/steerlineapp"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func createTaskBatchFixture(ctx context.Context, store persistence.CommandStore, config steerline.Config, data []byte, request string, version uint64, by string) (taskmodel.TaskBatchReceipt, error) {
	if err := config.ValidateTaskCommandConfiguration(); err != nil {
		return taskmodel.TaskBatchReceipt{}, err
	}
	commands := steerlineapp.NewTaskCommands(store, config)
	return commands.CreateTaskBatch(ctx, data, request, version, by)
}
func validateTaskBatchFixture(ctx context.Context, store persistence.CommandStore, config steerline.Config, data []byte) (taskmodel.TaskBatchValidation, error) {
	commands := steerlineapp.NewTaskCommands(store, config)
	return commands.ValidateTaskBatch(ctx, data)
}
