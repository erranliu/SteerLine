package client

import (
	"context"
	"encoding/json"
	api "github.com/erranliu/SteerLine/internal/product/api"
)

func testRead[T any](ctx context.Context, c *Client, fn func(api.TestManagementPort) (T, error)) (zero T, err error) {
	if err = c.begin(ctx); err != nil {
		return zero, err
	}
	defer c.mu.RUnlock()
	if c.resources.Tests == nil {
		return zero, unsupported("test management")
	}
	return fn(c.resources.Tests)
}
func (c *Client) PlanTests(ctx context.Context, id string) (json.RawMessage, error) {
	return testRead(ctx, c, func(p api.TestManagementPort) (json.RawMessage, error) { return p.PlanTests(ctx, id) })
}
func (c *Client) GenerateTests(ctx context.Context, in api.TestGenerateRequest) (api.TestGeneration, error) {
	return testRead(ctx, c, func(p api.TestManagementPort) (api.TestGeneration, error) { return p.GenerateTests(ctx, in) })
}
func (c *Client) TestGeneration(ctx context.Context, id string) (api.TestGeneration, error) {
	return testRead(ctx, c, func(p api.TestManagementPort) (api.TestGeneration, error) { return p.TestGeneration(ctx, id) })
}
func (c *Client) StopTestGeneration(ctx context.Context, in api.TestStopGenerationRequest) (api.TestGeneration, error) {
	return testRead(ctx, c, func(p api.TestManagementPort) (api.TestGeneration, error) { return p.StopTestGeneration(ctx, in) })
}
