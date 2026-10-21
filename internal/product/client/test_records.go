package client

import (
	"context"
	"encoding/json"
	api "github.com/erranliu/SteerLine/internal/product/api"
)

func (c *Client) recordsRead(ctx context.Context, fn func(api.TestRecordsPort) (json.RawMessage, error)) (json.RawMessage, error) {
	return testRead(ctx, c, func(p api.TestManagementPort) (json.RawMessage, error) {
		port, ok := p.(api.TestRecordsPort)
		if !ok {
			return nil, unsupported("test records")
		}
		return fn(port)
	})
}
func (c *Client) RegisterTestRecord(ctx context.Context, in api.TestRecordRegistration) (json.RawMessage, error) {
	return c.recordsRead(ctx, func(p api.TestRecordsPort) (json.RawMessage, error) { return p.RegisterTestRecord(ctx, in) })
}
func (c *Client) TestCatalogue(ctx context.Context) (json.RawMessage, error) {
	return c.recordsRead(ctx, func(p api.TestRecordsPort) (json.RawMessage, error) { return p.TestCatalogue(ctx) })
}
func (c *Client) TestRecords(ctx context.Context, f api.TestRecordFilter) (json.RawMessage, error) {
	return c.recordsRead(ctx, func(p api.TestRecordsPort) (json.RawMessage, error) { return p.TestRecords(ctx, f) })
}
func (c *Client) TestRecord(ctx context.Context, id string) (json.RawMessage, error) {
	return c.recordsRead(ctx, func(p api.TestRecordsPort) (json.RawMessage, error) { return p.TestRecord(ctx, id) })
}
func (c *Client) TestRecordEvidence(ctx context.Context, id string) (json.RawMessage, error) {
	return c.recordsRead(ctx, func(p api.TestRecordsPort) (json.RawMessage, error) { return p.TestRecordEvidence(ctx, id) })
}

func (c *Client) TestRecordReceipt(ctx context.Context, in api.TestRecordRegistration) (json.RawMessage, error) {
	return c.recordsRead(ctx, func(p api.TestRecordsPort) (json.RawMessage, error) { return p.TestRecordReceipt(ctx, in) })
}
