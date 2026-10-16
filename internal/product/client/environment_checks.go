package client

import (
	"context"
	"encoding/json"
	api "github.com/erranliu/SteerLine/internal/product/api"
)

func environmentRead[T any](ctx context.Context, c *Client, fn func(api.EnvironmentCheckPort) (T, error)) (zero T, err error) {
	if err = c.begin(ctx); err != nil {
		return zero, err
	}
	defer c.mu.RUnlock()
	if c.resources.EnvironmentChecks == nil {
		return zero, unsupported("environment checks")
	}
	return fn(c.resources.EnvironmentChecks)
}
func (c *Client) EnvironmentCheckCatalog(ctx context.Context) (json.RawMessage, error) {
	return environmentRead(ctx, c, func(p api.EnvironmentCheckPort) (json.RawMessage, error) {
		return p.EnvironmentCheckCatalog(ctx)
	})
}
func (c *Client) EnvironmentChecks(ctx context.Context) ([]api.EnvironmentCheckRecord, error) {
	return environmentRead(ctx, c, func(p api.EnvironmentCheckPort) ([]api.EnvironmentCheckRecord, error) {
		return p.EnvironmentChecks(ctx)
	})
}
func (c *Client) EnvironmentCheck(ctx context.Context, id string) (api.EnvironmentCheckRecord, error) {
	return environmentRead(ctx, c, func(p api.EnvironmentCheckPort) (api.EnvironmentCheckRecord, error) {
		return p.EnvironmentCheck(ctx, id)
	})
}
func (c *Client) EnvironmentCheckRequest(ctx context.Context, id string) (api.EnvironmentCheckRecord, error) {
	return environmentRead(ctx, c, func(p api.EnvironmentCheckPort) (api.EnvironmentCheckRecord, error) {
		return p.EnvironmentCheckRequest(ctx, id)
	})
}
func (c *Client) SubmitEnvironmentCheck(ctx context.Context, in api.EnvironmentCheckCommand) (api.EnvironmentCheckRecord, error) {
	return environmentRead(ctx, c, func(p api.EnvironmentCheckPort) (api.EnvironmentCheckRecord, error) {
		return p.SubmitEnvironmentCheck(ctx, in)
	})
}
