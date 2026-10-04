package steerlineapp

import (
	"context"

	productapi "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/steerline"
	"github.com/erranliu/SteerLine/internal/steerline/persistence"
	taskapp "github.com/erranliu/SteerLine/internal/task"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

// NewTaskCommands extracts only fixed task input and injects narrow I/O ports.
func NewTaskCommands(store persistence.CommandStore, config steerline.Config) taskapp.Commands {
	tasks := NewTaskService(nil, config)
	return taskapp.Commands{Store: taskCommandStore{store}, Documents: tasks.Documents, Input: tasks.Input}
}

type clientTaskCommands struct {
	store  persistence.CommandStore
	config steerline.Config
}

func newClientTaskCommands(store persistence.CommandStore, config steerline.Config) clientTaskCommands {
	return clientTaskCommands{store: store, config: config}
}
func (r *clientTaskCommands) Validate(ctx context.Context, payload []byte) (taskmodel.TaskBatchValidation, error) {
	commands := NewTaskCommands(r.store, r.config)
	return commands.ValidateTaskBatch(ctx, payload)
}
func (r *clientTaskCommands) Create(ctx context.Context, in productapi.CreateInput) (taskmodel.TaskBatchReceipt, error) {
	if err := r.config.ValidateTaskCommandConfiguration(); err != nil {
		return taskmodel.TaskBatchReceipt{}, err
	}
	commands := NewTaskCommands(r.store, r.config)
	return commands.CreateTaskBatch(ctx, in.Payload, in.RequestID, in.ExpectedAuthorityVersion, in.RequestedBy)
}
func (r *clientTaskCommands) AddPredecessors(ctx context.Context, in productapi.TaskPredecessorAddInput) (productapi.TaskPredecessorAddReceipt, error) {
	if err := r.config.ValidateTaskCommandConfiguration(); err != nil {
		return productapi.TaskPredecessorAddReceipt{}, err
	}
	commands := NewTaskCommands(r.store, r.config)
	receipt, err := commands.AddNativeTaskPredecessors(ctx, taskmodel.PredecessorAddInput(in))
	return productapi.TaskPredecessorAddReceipt(receipt), err
}
