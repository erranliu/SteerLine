package steerlineapp

import (
	"context"

	productapi "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/steerline"
	"github.com/erranliu/SteerLine/internal/steerline/persistence"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

type clientTaskCommands struct {
	service interface {
		ValidateTaskBatch(context.Context, steerline.Config, []byte) (taskmodel.TaskBatchValidation, error)
		CreateTaskBatch(context.Context, steerline.Config, []byte, string, uint64, string) (taskmodel.TaskBatchReceipt, error)
		AddNativeTaskPredecessors(context.Context, steerline.Config, productapi.TaskPredecessorAddInput) (productapi.TaskPredecessorAddReceipt, error)
	}
	config steerline.Config
}

func newClientTaskCommands(store persistence.CommandStore, config steerline.Config) clientTaskCommands {
	return clientTaskCommands{service: steerline.NewCommandService(store, config.Identity(), nil, nil), config: config}
}

func (r *clientTaskCommands) Validate(ctx context.Context, payload []byte) (taskmodel.TaskBatchValidation, error) {
	return r.service.ValidateTaskBatch(ctx, r.config, payload)
}

func (r *clientTaskCommands) Create(ctx context.Context, in productapi.CreateInput) (taskmodel.TaskBatchReceipt, error) {
	return r.service.CreateTaskBatch(ctx, r.config, in.Payload, in.RequestID, in.ExpectedAuthorityVersion, in.RequestedBy)
}

func (r *clientTaskCommands) AddPredecessors(ctx context.Context, in productapi.TaskPredecessorAddInput) (productapi.TaskPredecessorAddReceipt, error) {
	return r.service.AddNativeTaskPredecessors(ctx, r.config, in)
}
