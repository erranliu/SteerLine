package client

import (
	"context"
	"errors"
	api "github.com/erranliu/SteerLine/internal/product/api"
)

type RunUsageReader interface {
	RunUsage(context.Context, string, string) (api.RunUsageView, error)
}

func (c *Client) RunUsage(ctx context.Context, candidate, execution string) (api.RunUsageView, error) {
	return read(ctx, c, func(r Reader) (api.RunUsageView, error) {
		source, ok := r.(RunUsageReader)
		if !ok {
			return api.RunUsageView{}, errors.New("run usage source unavailable")
		}
		return source.RunUsage(ctx, candidate, execution)
	})
}
