package app

import (
	"context"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/taskrunner"
)

type runtimeTaskCommands struct {
	clientTaskCommands
	service *taskrunner.Actor
}

func (r *runtimeTaskCommands) Close() error { return nil } // store belongs to ProjectRuntime
func (r *runtimeTaskCommands) Create(ctx context.Context, in api.CreateInput) (api.TaskBatchReceipt, error) {
	result := make(chan api.TaskBatchReceipt, 1)
	err := r.service.ExecuteTaskMutation(ctx, func(actor context.Context) error {
		value, err := r.clientTaskCommands.Create(actor, in)
		result <- value
		return err
	})
	if err != nil {
		return api.TaskBatchReceipt{}, err
	}
	return <-result, nil
}
func (r *runtimeTaskCommands) AddPredecessors(ctx context.Context, in api.TaskPredecessorAddInput) (api.TaskPredecessorAddReceipt, error) {
	result := make(chan api.TaskPredecessorAddReceipt, 1)
	err := r.service.ExecuteTaskMutation(ctx, func(actor context.Context) error {
		value, err := r.clientTaskCommands.AddPredecessors(actor, in)
		result <- value
		return err
	})
	if err != nil {
		return api.TaskPredecessorAddReceipt{}, err
	}
	return <-result, nil
}
func (r *runtimeTaskCommands) Discard(context.Context, api.DiscardInput) (api.TaskDiscardReceipt, error) {
	return api.TaskDiscardReceipt{}, &api.Error{Kind: api.Unsupported, Message: "discard uses the actor discard port"}
}
