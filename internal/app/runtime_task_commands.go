package app

import (
	"context"
	"github.com/erranliu/SteerLine/internal/execution"
	api "github.com/erranliu/SteerLine/internal/product/api"
)

type runtimeTaskCommands struct {
	clientTaskCommands
	service *execution.Service
}

func (r *runtimeTaskCommands) Close() error { return nil } // store belongs to ProjectRuntime
func (r *runtimeTaskCommands) Create(ctx context.Context, in api.CreateInput) (value api.TaskBatchReceipt, err error) {
	err = r.service.ExecuteTaskMutation(ctx, func(actor context.Context) error {
		var e error
		value, e = r.clientTaskCommands.Create(actor, in)
		return e
	})
	return
}
func (r *runtimeTaskCommands) AddPredecessors(ctx context.Context, in api.TaskPredecessorAddInput) (value api.TaskPredecessorAddReceipt, err error) {
	err = r.service.ExecuteTaskMutation(ctx, func(actor context.Context) error {
		var e error
		value, e = r.clientTaskCommands.AddPredecessors(actor, in)
		return e
	})
	return
}
func (r *runtimeTaskCommands) Discard(context.Context, api.DiscardInput) (api.TaskDiscardReceipt, error) {
	return api.TaskDiscardReceipt{}, &api.Error{Kind: api.Unsupported, Message: "discard uses the actor discard port"}
}
