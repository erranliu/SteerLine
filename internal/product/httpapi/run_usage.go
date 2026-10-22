package httpapi

import (
	"context"
	api "github.com/erranliu/SteerLine/internal/product/api"
	"net/url"
)

type runUsagePort interface {
	RunUsage(context.Context, string, string) (api.RunUsageView, error)
}

func (s *Server) readRunUsage(ctx context.Context, id string, values url.Values) (any, error) {
	if len(values) != 1 || len(values["execution_id"]) != 1 || values.Get("execution_id") == "" {
		return nil, invalidCommand("usage query requires only execution_id")
	}
	candidate, err := s.candidate(ctx, id)
	if err != nil {
		return nil, err
	}
	if candidate.ExecutionID != values.Get("execution_id") {
		return nil, &protocolError{status: 409, code: "stale_target", reason: "usage target changed"}
	}
	source, ok := s.query.(runUsagePort)
	if !ok {
		return nil, &api.Error{Kind: api.Unsupported, Message: "run usage unavailable"}
	}
	return source.RunUsage(ctx, id, candidate.ExecutionID)
}
