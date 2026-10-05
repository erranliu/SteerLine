package app

import (
	"context"

	"github.com/erranliu/SteerLine/internal/execution"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
	taskapp "github.com/erranliu/SteerLine/internal/task"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

// NewTaskCommands extracts only fixed task input and injects narrow I/O ports.
func NewTaskCommands(store taskStorage, config execution.Config) taskapp.Commands {
	tasks := NewTaskService(nil, config)
	return taskapp.Commands{Store: taskCommandStore{store}, Documents: tasks.Documents, Input: tasks.Input}
}

type clientTaskCommands struct {
	store  taskStorage
	config execution.Config
}

func newClientTaskCommands(store taskStorage, config execution.Config) clientTaskCommands {
	return clientTaskCommands{store: store, config: config}
}
func (r *clientTaskCommands) Validate(ctx context.Context, payload []byte) (productapi.TaskBatchValidation, error) {
	commands := NewTaskCommands(r.store, r.config)
	value, err := commands.ValidateTaskBatch(ctx, payload)
	if err != nil {
		return productapi.TaskBatchValidation{}, err
	}
	result := productapi.TaskBatchValidation{Valid: value.Valid, ProjectID: value.ProjectID, TaskCount: value.TaskCount, DependencyCount: value.DependencyCount, AuthorityVersion: value.AuthorityVersion, GraphRevision: value.GraphRevision, DatabaseRevision: value.DatabaseRevision, DatabaseID: value.DatabaseID, SampledAt: value.SampledAt, Cycle: append([]string(nil), value.Cycle...)}
	for _, issue := range value.Issues {
		result.Issues = append(result.Issues, productapi.TaskBatchIssue{Code: issue.Code, Node: issue.Node, Message: issue.Message})
	}
	return result, nil
}
func (r *clientTaskCommands) Create(ctx context.Context, in productapi.CreateInput) (productapi.TaskBatchReceipt, error) {
	if err := r.config.ValidateTaskCommandConfiguration(); err != nil {
		return productapi.TaskBatchReceipt{}, err
	}
	commands := NewTaskCommands(r.store, r.config)
	value, err := commands.CreateTaskBatch(ctx, in.Payload, in.RequestID, in.ExpectedAuthorityVersion, in.RequestedBy)
	if err != nil {
		return productapi.TaskBatchReceipt{}, err
	}
	return productapi.TaskBatchReceipt(value), nil
}
func (r *clientTaskCommands) AddPredecessors(ctx context.Context, in productapi.TaskPredecessorAddInput) (productapi.TaskPredecessorAddReceipt, error) {
	if err := r.config.ValidateTaskCommandConfiguration(); err != nil {
		return productapi.TaskPredecessorAddReceipt{}, err
	}
	commands := NewTaskCommands(r.store, r.config)
	receipt, err := commands.AddNativeTaskPredecessors(ctx, taskmodel.PredecessorAddInput(in))
	return productapi.TaskPredecessorAddReceipt(receipt), err
}
