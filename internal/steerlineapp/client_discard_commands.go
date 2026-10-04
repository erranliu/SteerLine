package steerlineapp

import (
	"context"

	productapi "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/steerline"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

type clientDiscardCommands struct {
	service interface {
		DiscardNativeTask(context.Context, steerline.Config, string, string, uint64, string, string) (taskmodel.TaskDiscardReceipt, error)
	}
	config steerline.Config
}

func (r *clientDiscardCommands) Discard(ctx context.Context, in productapi.DiscardInput) (taskmodel.TaskDiscardReceipt, error) {
	return r.service.DiscardNativeTask(ctx, r.config, in.TaskID, in.RequestID, in.ExpectedVersion, in.Reason, in.RequestedBy)
}
