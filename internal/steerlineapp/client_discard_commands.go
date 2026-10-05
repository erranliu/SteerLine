package steerlineapp

import (
	"context"

	"github.com/erranliu/SteerLine/internal/execution"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

type clientDiscardCommands struct {
	service interface {
		DiscardNativeTask(context.Context, execution.Config, string, string, uint64, string, string) (taskmodel.TaskDiscardReceipt, error)
	}
	config execution.Config
}

func (r *clientDiscardCommands) Discard(ctx context.Context, in productapi.DiscardInput) (productapi.TaskDiscardReceipt, error) {
	value, err := r.service.DiscardNativeTask(ctx, r.config, in.TaskID, in.RequestID, in.ExpectedVersion, in.Reason, in.RequestedBy)
	if err != nil {
		return productapi.TaskDiscardReceipt{}, err
	}
	return productapi.TaskDiscardReceipt(value), nil
}
