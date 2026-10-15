package client

import (
	"context"
	"errors"
	api "github.com/erranliu/SteerLine/internal/product/api"
)

// Activity telemetry is a separate capability from canonical history readers.
type LastMessageReader interface {
	LastMessage(context.Context, string, string, string) (api.LastMessageView, error)
}

func (c *Client) LastMessage(ctx context.Context, run, execution, attempt string) (api.LastMessageView, error) {
	return read(ctx, c, func(r Reader) (api.LastMessageView, error) {
		source, ok := r.(LastMessageReader)
		if !ok {
			return api.LastMessageView{}, errors.New("last message source unavailable")
		}
		return source.LastMessage(ctx, run, execution, attempt)
	})
}
