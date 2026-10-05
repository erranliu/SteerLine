package integration

import (
	"context"

	steerlineapp "github.com/erranliu/SteerLine/internal/app"
	"github.com/erranliu/SteerLine/internal/execution"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func createTaskBatchFixture(ctx context.Context, store *sqlite.SQLiteStore, config execution.Config, data []byte, request string, version uint64, by string) (taskmodel.TaskBatchReceipt, error) {
	if err := config.ValidateTaskCommandConfiguration(); err != nil {
		return taskmodel.TaskBatchReceipt{}, err
	}
	commands := steerlineapp.NewTaskCommands(store, config)
	return commands.CreateTaskBatch(ctx, data, request, version, by)
}

func sqliteCommandStore(store execution.CommandStore) *sqlite.SQLiteStore {
	value, ok := store.(*sqlite.SQLiteStore)
	if !ok {
		panic("integration command service is not wired to the app-owned SQLite implementation")
	}
	return value
}
func validateTaskBatchFixture(ctx context.Context, store *sqlite.SQLiteStore, config execution.Config, data []byte) (taskmodel.TaskBatchValidation, error) {
	commands := steerlineapp.NewTaskCommands(store, config)
	return commands.ValidateTaskBatch(ctx, data)
}
