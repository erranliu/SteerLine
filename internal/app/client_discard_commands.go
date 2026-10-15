package app

import (
	"context"

	productapi "github.com/erranliu/SteerLine/internal/product/api"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

type clientDiscardCommands struct {
	service interface {
		Discard(context.Context, string, taskmodel.DiscardIntent) (taskmodel.TaskDiscardReceipt, error)
	}
}

func (r *clientDiscardCommands) Discard(ctx context.Context, in productapi.DiscardInput) (productapi.TaskDiscardReceipt, error) {
	value, err := r.service.Discard(ctx, in.RequestID, taskmodel.DiscardIntent{TaskID: in.TaskID, ExpectedVersion: in.ExpectedVersion, Reason: in.Reason, RequestedBy: in.RequestedBy})
	if err != nil {
		return productapi.TaskDiscardReceipt{}, err
	}
	return productapi.TaskDiscardReceipt(value), nil
}
